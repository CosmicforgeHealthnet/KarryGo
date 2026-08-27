package booking

import (
	"context"
	"errors"

	"cosmicforge/logistics/shared/go/walletclient"
)

// PaymentClient is the wallet surface the booking usecase needs. The escrow
// intent is keyed by source_service + source_reference = booking id, which is
// exactly what trip completion settles via CompleteJob (trip/service.go).
type PaymentClient interface {
	// Hold creates (idempotently) a wallet payment intent for the fare and
	// debits the customer wallet into job escrow. Returns the intent reference.
	Hold(ctx context.Context, bookingID, customerID string, amountKobo int64) (string, error)
	// AssignProvider attaches the accepted rider to the held intent so
	// settlement can credit the rider's payable ledger.
	AssignProvider(ctx context.Context, paymentRef, providerID string) error
	// Refund releases held funds back to the customer wallet. paymentRef is
	// the intent reference from Hold; the booking id source pair is sent as a
	// fallback so refunds resolve even when only the booking id is known.
	Refund(ctx context.Context, bookingID, paymentRef string, amountKobo int64, currency, reason, idempotencyKey string) error
}

const (
	// walletSourceService must equal what trip completion passes to
	// CompleteJob ("dispatch-delivery-service", trip/service.go) so the hold,
	// settlement, and refunds all resolve the same intent.
	walletSourceService = "dispatch-delivery-service"
	// walletProviderType must equal the payment-wallet provider token map key
	// for dispatch riders ("dispatch") so settled funds land in the ledger the
	// rider's wallet endpoints read.
	walletProviderType = "dispatch"
	currencyNGN        = "NGN"
)

// WalletPaymentClient adapts shared/go/walletclient to PaymentClient.
type WalletPaymentClient struct {
	wallet walletclient.Client
}

type WalletCommerceSettlementClient struct{ wallet walletclient.Client }

func NewWalletCommerceSettlementClient(client walletclient.Client) *WalletCommerceSettlementClient {
	return &WalletCommerceSettlementClient{wallet: client}
}

func (c *WalletCommerceSettlementClient) ReleaseCommerceDelivery(ctx context.Context, _ string, vendorOrderID, providerID, completionReference string) error {
	result, err := c.wallet.ReleaseCommerceDelivery(ctx, vendorOrderID, walletclient.DeliveryReleaseRequest{ProviderType: "dispatch_provider", ProviderID: providerID, CompletionReference: completionReference, IdempotencyKey: "dispatch-commerce-delivery-release-" + completionReference + "-" + vendorOrderID})
	if err != nil {
		return err
	}
	if result.Status != "APPLIED" {
		return errors.New("commerce delivery settlement was not applied")
	}
	return nil
}

var _ CommerceDeliverySettlementAuthority = (*WalletCommerceSettlementClient)(nil)

func NewWalletPaymentClient(client walletclient.Client) *WalletPaymentClient {
	return &WalletPaymentClient{wallet: client}
}

func (c *WalletPaymentClient) Hold(ctx context.Context, bookingID, customerID string, amountKobo int64) (string, error) {
	intent, err := c.wallet.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{
		SourceService:   walletSourceService,
		SourceReference: bookingID,
		CustomerID:      customerID,
		ProviderType:    walletProviderType,
		AmountKobo:      amountKobo,
		Currency:        currencyNGN,
		PaymentMethod:   walletclient.MethodWallet,
		IdempotencyKey:  "dispatch-hold-" + bookingID,
	})
	if err != nil {
		return "", err
	}
	if _, err := c.wallet.PayFromWallet(ctx, intent.ID, "dispatch-pay-"+bookingID); err != nil {
		return intent.Reference, err
	}
	return intent.Reference, nil
}

func (c *WalletPaymentClient) AssignProvider(ctx context.Context, paymentRef, providerID string) error {
	_, err := c.wallet.AssignProvider(ctx, paymentRef, walletProviderType, providerID)
	return err
}

func (c *WalletPaymentClient) Refund(ctx context.Context, bookingID, paymentRef string, amountKobo int64, currency, reason, idempotencyKey string) error {
	if currency == "" {
		currency = currencyNGN
	}
	if paymentRef == "" {
		paymentRef = bookingID
	}
	_, err := c.wallet.RequestRefund(ctx, walletclient.RefundRequest{
		PaymentReference: paymentRef,
		SourceService:    walletSourceService,
		SourceReference:  bookingID,
		AmountKobo:       amountKobo,
		Currency:         currency,
		Reason:           reason,
		IdempotencyKey:   idempotencyKey,
	})
	return err
}
