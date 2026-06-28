// Package payments adapts shared/go/walletclient to the booking usecase
// PaymentClient interface. It is a separate package from booking/clients to
// avoid an import cycle: bookingusecases imports booking/clients (the notifier),
// while this adapter imports bookingusecases (for its interface types).
package payments

import (
	"context"
	"fmt"

	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	"cosmicforge/logistics/shared/go/walletclient"
)

const (
	sourceService = "driver-hauling-service"
	providerType  = "truck_provider"
	currencyNGN   = "NGN"
)

// WalletPaymentClient binds booking fares to payment-wallet-service payment
// intents keyed by booking id (source_service + source_reference), so wallet/card
// holds, settlements, and refunds are all traceable to the trip.
type WalletPaymentClient struct {
	wallet walletclient.Client
}

func NewWalletPaymentClient(baseURL, serviceName string, secret []byte) *WalletPaymentClient {
	return &WalletPaymentClient{
		wallet: walletclient.Client{
			BaseURL:     baseURL,
			ServiceName: serviceName,
			Secret:      secret,
		},
	}
}

// HoldFromWallet creates (idempotently) a wallet payment intent for the booking
// fare and holds the funds from the customer wallet. Returns the payment
// reference (refunds/lookups resolve by reference, not intent id).
func (c *WalletPaymentClient) HoldFromWallet(ctx context.Context, in bookingusecases.PaymentHoldInput) (string, error) {
	intent, err := c.wallet.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{
		SourceService:   sourceService,
		SourceReference: in.BookingID,
		CustomerID:      in.CustomerID,
		ProviderID:      in.ProviderID,
		ProviderType:    providerType,
		AmountKobo:      in.AmountKobo,
		Currency:        currencyNGN,
		PaymentMethod:   walletclient.MethodWallet,
		IdempotencyKey:  "hauling-hold-" + in.BookingID,
	})
	if err != nil {
		return "", err
	}
	if _, err := c.wallet.PayFromWallet(ctx, intent.ID, "hauling-pay-"+in.BookingID); err != nil {
		return intent.Reference, err
	}
	return intent.Reference, nil
}

// CreateCardIntent creates a Paystack payment intent for the booking fare and
// returns the payment reference plus the authorization URL for the checkout WebView.
func (c *WalletPaymentClient) CreateCardIntent(ctx context.Context, in bookingusecases.PaymentHoldInput, customerEmail string) (string, string, error) {
	intent, err := c.wallet.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{
		SourceService:   sourceService,
		SourceReference: in.BookingID,
		CustomerID:      in.CustomerID,
		CustomerEmail:   customerEmail,
		ProviderID:      in.ProviderID,
		ProviderType:    providerType,
		AmountKobo:      in.AmountKobo,
		Currency:        currencyNGN,
		PaymentMethod:   walletclient.MethodPaystack,
		IdempotencyKey:  "hauling-card-" + in.BookingID,
	})
	if err != nil {
		return "", "", err
	}
	if intent.AuthorizationURL == "" {
		return intent.Reference, "", fmt.Errorf("payment-wallet returned no authorization url")
	}
	return intent.Reference, intent.AuthorizationURL, nil
}

// Settle releases the booking's held funds to the provider on completion.
func (c *WalletPaymentClient) Settle(ctx context.Context, bookingID string) error {
	_, err := c.wallet.CompleteJob(ctx, sourceService, bookingID)
	return err
}

// Refund reverses a held charge when a booking is cancelled or unmatched.
func (c *WalletPaymentClient) Refund(ctx context.Context, paymentReference string, amountKobo int64, reason string) error {
	_, err := c.wallet.RequestRefund(ctx, walletclient.RefundRequest{
		PaymentReference: paymentReference,
		AmountKobo:       amountKobo,
		Currency:         currencyNGN,
		Reason:           reason,
		IdempotencyKey:   "hauling-refund-" + paymentReference,
	})
	return err
}
