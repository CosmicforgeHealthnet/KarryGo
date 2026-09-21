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
	wallet        walletclient.Client
	recoveryCases RecoveryCaseLookup
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
		return "", fmt.Errorf("%w: create wallet payment intent: %v", bookingusecases.ErrPaymentOutcomeUnknown, err)
	}
	if _, err := c.wallet.PayFromWallet(ctx, intent.ID, "hauling-pay-"+in.BookingID); err != nil {
		readBack, readErr := c.wallet.GetPayment(ctx, intent.Reference)
		if readErr == nil && walletSecured(readBack.Status) {
			return intent.Reference, nil
		}
		if readErr != nil || !walletDefinitivelyFailed(readBack.Status) {
			return intent.Reference, fmt.Errorf("%w: wallet hold requires reconciliation", bookingusecases.ErrPaymentOutcomeUnknown)
		}
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
func (c *WalletPaymentClient) Settle(ctx context.Context, bookingID string, paymentReference string) error {
	_, err := c.wallet.CompleteJob(ctx, sourceService, bookingID)
	if err == nil {
		return nil
	}
	if paymentReference != "" {
		readBack, readErr := c.wallet.GetPayment(ctx, paymentReference)
		if readErr == nil && walletSettled(readBack.Status) {
			return nil
		}
		if readErr != nil || !walletDefinitivelyFailed(readBack.Status) {
			return fmt.Errorf("%w: wallet settlement requires reconciliation", bookingusecases.ErrPaymentOutcomeUnknown)
		}
	}
	return fmt.Errorf("%w: wallet settlement has no readable payment reference", bookingusecases.ErrPaymentOutcomeUnknown)
}

func (c *WalletPaymentClient) ReleaseCommerceDelivery(ctx context.Context, _ string, vendorOrderID, providerID, completionReference string) error {
	result, err := c.wallet.ReleaseCommerceDelivery(ctx, vendorOrderID, walletclient.DeliveryReleaseRequest{ProviderType: providerType, ProviderID: providerID, CompletionReference: completionReference, IdempotencyKey: "hauling-commerce-delivery-release-" + completionReference + "-" + vendorOrderID})
	if err != nil {
		return err
	}
	if result.Status != "APPLIED" {
		return fmt.Errorf("commerce delivery settlement was not applied")
	}
	return nil
}

var _ bookingusecases.CommerceDeliverySettlementAuthority = (*WalletPaymentClient)(nil)

// Refund reverses a held charge when a booking is cancelled or unmatched.
func (c *WalletPaymentClient) Refund(ctx context.Context, paymentReference string, amountKobo int64, reason string) error {
	_, err := c.wallet.RequestRefund(ctx, walletclient.RefundRequest{
		PaymentReference: paymentReference,
		AmountKobo:       amountKobo,
		Currency:         currencyNGN,
		Reason:           reason,
		IdempotencyKey:   "hauling-refund-" + paymentReference,
	})
	if err == nil {
		return nil
	}
	readBack, readErr := c.wallet.GetPayment(ctx, paymentReference)
	if readErr == nil && walletRefunded(readBack.Status) {
		return nil
	}
	if readErr != nil || !walletDefinitivelyFailed(readBack.Status) {
		return fmt.Errorf("%w: wallet refund requires reconciliation", bookingusecases.ErrPaymentOutcomeUnknown)
	}
	return err
}

func walletSecured(status string) bool {
	switch status {
	case "held", "paid", "completed", "settled":
		return true
	default:
		return false
	}
}

func walletSettled(status string) bool {
	switch status {
	case "paid", "completed", "settled":
		return true
	default:
		return false
	}
}

func walletRefunded(status string) bool {
	switch status {
	case "refunded", "reversed", "cancelled":
		return true
	default:
		return false
	}
}

func walletDefinitivelyFailed(status string) bool {
	switch status {
	case "failed", "rejected":
		return true
	default:
		return false
	}
}
