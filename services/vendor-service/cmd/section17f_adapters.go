package main

// Section 17F adapters.
//
// The orders feature never learns a Wallet base URL, a secret or an HTTP
// contract. It receives the money authority through a narrow port, and this
// adapter maps that port onto shared/go/walletclient — the platform's real
// payment contract, unchanged.

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/walletclient"
)

// section17FPaymentAuthority adapts payment-wallet-service to the order
// feature's PaymentAuthority port.
//
// It performs the two-step the Wallet contract requires — create the intent,
// then pay it from the customer's wallet — and reports the authority's own
// answer verbatim. It never decides that a payment succeeded.
type section17FPaymentAuthority struct {
	client walletclient.Client

	// enabled is the deployment's own switch, kept separate from credentials
	// so an operator can hold working secrets and still deliberately not
	// accept payments.
	enabled bool
}

func newSection17FPaymentAuthority(
	enabled bool, baseURL, secret string, timeout time.Duration,
) *section17FPaymentAuthority {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &section17FPaymentAuthority{
		client: walletclient.Client{
			// BaseURL must be a BARE ORIGIN: walletclient appends the full
			// /api/v1/payment-wallet path itself. A full path here produces a
			// doubled path that 404s every call - a mistake this platform has
			// already made once with HAULING_PAYMENT_URL.
			BaseURL:     strings.TrimRight(baseURL, "/"),
			ServiceName: orders.PaymentSourceService,
			Secret:      []byte(secret),
		},
		enabled: enabled && strings.TrimSpace(baseURL) != "" && strings.TrimSpace(secret) != "",
	}
}

// Configured implements orders.PaymentAuthority.
func (a *section17FPaymentAuthority) Configured() bool {
	return a != nil && a.enabled
}

// HoldLeg implements orders.PaymentAuthority.
//
// Idempotency is the caller's deterministic key, which payment-wallet-service
// honours on intent creation, so a retry returns the same intent rather than
// creating a second one.
func (a *section17FPaymentAuthority) HoldLeg(
	ctx context.Context, request orders.HoldLegRequest,
) (orders.LegResult, error) {
	if !a.Configured() {
		return orders.LegResult{}, orders.ErrPaymentNotConfigured
	}

	intent, err := a.client.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{
		SourceService:   request.SourceService,
		SourceReference: request.SourceReference,
		CustomerID:      request.CustomerID,
		CustomerEmail:   request.CustomerEmail,
		ProviderID:      request.ProviderID,
		ProviderType:    request.ProviderType,
		AmountKobo:      request.AmountKobo,
		Currency:        request.Currency,
		PaymentMethod:   walletclient.MethodWallet,
		IdempotencyKey:  request.IdempotencyKey,
		Metadata:        toMetadata(request.Metadata),
	})
	if err != nil {
		return orders.LegResult{}, err
	}

	// An intent that the authority already moved past "pending" is reported as
	// it stands. Paying it again would be the caller inventing a second
	// financial effect.
	if !strings.EqualFold(intent.Status, "pending") {
		return legResult(intent), nil
	}

	paid, err := a.client.PayFromWallet(ctx, intent.ID, request.IdempotencyKey)
	if err != nil {
		// The intent exists and may or may not have moved. Returning the error
		// with the last known reference lets recovery re-read it rather than
		// assuming either outcome.
		return legResult(intent), err
	}
	return legResult(paid), nil
}

// GetLeg implements orders.PaymentAuthority. It is the recovery read: the
// authority's own current view of a leg, used to learn what actually happened
// across a crash.
func (a *section17FPaymentAuthority) GetLeg(
	ctx context.Context, reference string,
) (orders.LegResult, error) {
	if !a.Configured() {
		return orders.LegResult{}, orders.ErrPaymentNotConfigured
	}
	if strings.TrimSpace(reference) == "" {
		return orders.LegResult{}, errors.New("payment reference is required")
	}
	intent, err := a.client.GetPayment(ctx, reference)
	if err != nil {
		return orders.LegResult{}, err
	}
	return legResult(intent), nil
}

// RefundLeg implements orders.PaymentAuthority.
//
// It refunds exactly one leg. Because each VendorOrder has its own payment
// intent, refunding one vendor's allocation cannot touch another's - which is
// the whole reason the payment was split at creation.
func (a *section17FPaymentAuthority) RefundLeg(
	ctx context.Context, request orders.RefundLegRequest,
) (orders.RefundResult, error) {
	if !a.Configured() {
		return orders.RefundResult{}, orders.ErrPaymentNotConfigured
	}
	if strings.TrimSpace(request.PaymentReference) == "" {
		return orders.RefundResult{}, errors.New("payment reference is required for a refund")
	}

	refund, err := a.client.RequestRefund(ctx, walletclient.RefundRequest{
		PaymentReference: request.PaymentReference,
		SourceService:    request.SourceService,
		SourceReference:  request.SourceReference,
		AmountKobo:       request.AmountKobo,
		Currency:         request.Currency,
		Reason:           request.Reason,
		IdempotencyKey:   request.IdempotencyKey,
	})
	if err != nil {
		return orders.RefundResult{}, err
	}
	return orders.RefundResult{
		Reference:  refund.Reference,
		Status:     refund.Status,
		AmountKobo: refund.AmountKobo,
	}, nil
}

// CompleteLeg maps isolated VendorOrder settlement to Wallet's real commerce
// contract. The source reference is the VendorOrder id, so one completion can
// never settle a sibling allocation.
func (a *section17FPaymentAuthority) CompleteLeg(
	ctx context.Context, sourceService, sourceReference string,
) (orders.LegResult, error) {
	if !a.Configured() {
		return orders.LegResult{}, orders.ErrPaymentNotConfigured
	}
	intent, err := a.client.CompleteJob(ctx, sourceService, sourceReference)
	if err != nil {
		return orders.LegResult{}, err
	}
	return legResult(intent), nil
}

func legResult(intent walletclient.PaymentIntent) orders.LegResult {
	return orders.LegResult{
		IntentID:   intent.ID,
		Reference:  intent.Reference,
		Status:     intent.Status,
		AmountKobo: intent.AmountKobo,
	}
}

func toMetadata(values map[string]any) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

var _ orders.PaymentAuthority = (*section17FPaymentAuthority)(nil)
var _ orders.PaymentSettlementAuthority = (*section17FPaymentAuthority)(nil)
