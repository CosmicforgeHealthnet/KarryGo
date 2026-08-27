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
	"math"
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

func (a *section17FPaymentAuthority) PaymentCapability(ctx context.Context, parentOrderID, operationRef string) (orders.PaymentCapability, error) {
	if a == nil || !a.Configured() {
		return "", orders.ErrPaymentNotConfigured
	}
	// The current Wallet contract exposes escrow hold, completion and exact
	// refund operations, which is the CAP_AND_REFUND strategy. This is an
	// explicit adapter capability, not provider-name inference.
	return orders.PaymentCapAndRefund, nil
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

func (a *section17FPaymentAuthority) AuthorizeCommercePayment(ctx context.Context, request orders.CommercePaymentRequest) (orders.CommercePaymentResult, error) {
	if !a.Configured() {
		return orders.CommercePaymentResult{}, orders.ErrPaymentNotConfigured
	}
	allocations := make([]walletclient.CommerceAllocation, 0, len(request.Allocations))
	for _, value := range request.Allocations {
		allocations = append(allocations, walletclient.CommerceAllocation{AllocationReference: value.AllocationReference, Scope: value.Scope, VendorOrderID: value.VendorOrderID, VendorID: value.VendorID, BranchID: value.BranchID, DeliveryResponsibility: value.DeliveryResponsibility, Currency: request.Currency, CustomerPayableMinor: value.CustomerPayableMinor, MaxAuthorizedMinor: value.MaxAuthorizedMinor, MerchandiseGrossMinor: value.MerchandiseGrossMinor, VendorProductDiscountMinor: value.VendorProductDiscountMinor, PlatformProductDiscountMinor: value.PlatformProductDiscountMinor, PlatformCommissionMinor: value.PlatformCommissionMinor, DeliveryCustomerMinor: value.DeliveryCustomerMinor, VendorDeliverySubsidyMinor: value.VendorDeliverySubsidyMinor, PlatformDeliverySubsidyMinor: value.PlatformDeliverySubsidyMinor, OtherFundingMinor: value.OtherFundingMinor, VendorMerchandisePayableMinor: value.VendorMerchandisePayableMinor, VendorDeliveryPayableMinor: value.VendorDeliveryPayableMinor, ProviderDeliveryPayableMinor: value.ProviderDeliveryPayableMinor, PlatformRevenueMinor: value.PlatformRevenueMinor, Snapshot: value.Snapshot, SnapshotFingerprint: value.SnapshotFingerprint})
	}
	intent, err := a.client.CreateCommerceIntent(ctx, walletclient.CommerceIntentRequest{ParentCommerceReference: request.ParentOrderID, CustomerID: request.CustomerID, CustomerEmail: request.CustomerEmail, QuoteReference: request.QuoteID, QuoteVersion: request.QuoteVersion, QuoteFingerprint: request.QuoteFingerprint, FinalAuthorizedMinor: request.FinalAuthorizedMinor, MaxAuthorizedMinor: request.MaxAuthorizedMinor, AllocationFingerprint: request.AllocationFingerprint, Currency: request.Currency, PaymentMethod: request.PaymentMethod, IdempotencyKey: request.IdempotencyKey, Allocations: allocations})
	if err != nil {
		return commercePaymentResult(intent), err
	}
	if intent.Status == "CREATED" {
		intent, err = a.client.HoldCommerceIntent(ctx, request.ParentOrderID, request.IdempotencyKey+":hold")
	}
	return commercePaymentResult(intent), err
}

func (a *section17FPaymentAuthority) GetCommercePayment(ctx context.Context, parentOrderID string) (orders.CommercePaymentResult, error) {
	if !a.Configured() {
		return orders.CommercePaymentResult{}, orders.ErrPaymentNotConfigured
	}
	intent, err := a.client.GetCommerceIntent(ctx, parentOrderID)
	return commercePaymentResult(intent), err
}

func commercePaymentResult(intent walletclient.CommerceIntent) orders.CommercePaymentResult {
	fingerprints := make(map[string]string)
	for _, allocation := range intent.Allocations {
		if allocation.VendorOrderID != "" {
			fingerprints[allocation.VendorOrderID] = allocation.SnapshotFingerprint
		}
	}
	return orders.CommercePaymentResult{IntentID: intent.PaymentIntentID, Reference: intent.PaymentReference, Status: intent.Status, FinalAuthorizedMinor: intent.FinalAuthorizedMinor, MaxAuthorizedMinor: intent.MaxAuthorizedMinor, AllocationFingerprint: intent.AllocationFingerprint, AllocationFingerprints: fingerprints}
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
// This is the compatibility refund for already-created per-leg intents. New
// parent commerce payments use RefundCommerceAllocation below.
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

func (a *section17FPaymentAuthority) ReleaseCommerceAllocation(ctx context.Context, request orders.CommerceReleaseRequest) (orders.CommerceFinancialResult, error) {
	if !a.Configured() {
		return orders.CommerceFinancialResult{}, orders.ErrPaymentNotConfigured
	}
	operation, err := a.client.ReleaseVendorPayable(ctx, request.VendorOrderID, walletclient.VendorReleaseRequest{
		CompletionReference: request.CompletionReference, ExpectedSnapshotFingerprint: request.SnapshotFingerprint,
		IdempotencyKey: request.IdempotencyKey + ":vendor",
	})
	if err != nil {
		return commerceFinancialResult(operation), err
	}
	amount := operation.AmountMinor
	if operation.Status != "APPLIED" || amount < 0 {
		return commerceFinancialResult(operation), orders.ErrPaymentAuthority
	}
	if request.ReleaseVendorOwnedDelivery {
		delivery, releaseErr := a.client.ReleaseCommerceDelivery(ctx, request.VendorOrderID, walletclient.DeliveryReleaseRequest{
			CompletionReference: request.CompletionReference, ExpectedSnapshotFingerprint: request.SnapshotFingerprint,
			IdempotencyKey: request.IdempotencyKey + ":delivery",
		})
		if releaseErr != nil {
			return commerceFinancialResult(delivery), releaseErr
		}
		if delivery.Status != "APPLIED" || delivery.AmountMinor < 0 || delivery.AmountMinor > math.MaxInt64-amount {
			return commerceFinancialResult(delivery), orders.ErrPaymentAuthority
		}
		amount += delivery.AmountMinor
	}
	return orders.CommerceFinancialResult{Reference: operation.ID, Status: operation.Status, AmountMinor: amount}, nil
}

func (a *section17FPaymentAuthority) RefundCommerceAllocation(ctx context.Context, request orders.CommerceRefundRequest) (orders.CommerceFinancialResult, error) {
	if !a.Configured() {
		return orders.CommerceFinancialResult{}, orders.ErrPaymentNotConfigured
	}
	operation, err := a.client.RefundCommerceAllocation(ctx, request.VendorOrderID, walletclient.CommerceRefundRequest{
		ItemReference: request.ItemReference, CustomerRefundMinor: request.CustomerRefundMinor,
		PlatformFundingReversalMinor: request.PlatformFundingReversalMinor, OtherFundingReversalMinor: request.OtherFundingReversalMinor,
		EscrowRecoveryMinor: request.EscrowRecoveryMinor, VendorRecoveryMinor: request.VendorRecoveryMinor,
		ProviderRecoveryMinor: request.ProviderRecoveryMinor, PlatformRecoveryMinor: request.PlatformRecoveryMinor,
		PlatformLossFundingMinor:        request.PlatformLossFundingMinor,
		VendorMerchandiseReductionMinor: request.VendorMerchandiseReductionMinor,
		VendorDeliveryReductionMinor:    request.VendorDeliveryReductionMinor,
		ProviderDeliveryReductionMinor:  request.ProviderDeliveryReductionMinor,
		PlatformRevenueReductionMinor:   request.PlatformRevenueReductionMinor,
		AuthorizationReleaseMinor:       request.AuthorizationReleaseMinor,
		ExpectedSnapshotFingerprint:     request.SnapshotFingerprint, IdempotencyKey: request.IdempotencyKey,
	})
	return commerceFinancialResult(operation), err
}

func (a *section17FPaymentAuthority) GetCommerceAllocationState(ctx context.Context, parentOrderID, vendorOrderID string) (orders.CommerceAllocationState, error) {
	if !a.Configured() {
		return orders.CommerceAllocationState{}, orders.ErrPaymentNotConfigured
	}
	intent, err := a.client.GetCommerceIntent(ctx, parentOrderID)
	if err != nil {
		return orders.CommerceAllocationState{}, err
	}
	for _, allocation := range intent.Allocations {
		if allocation.VendorOrderID != vendorOrderID {
			continue
		}
		return orders.CommerceAllocationState{
			SnapshotFingerprint: allocation.SnapshotFingerprint, CustomerPayableMinor: allocation.CustomerPayableMinor,
			MaxAuthorizedMinor:  allocation.MaxAuthorizedMinor,
			VendorReleasedMinor: allocation.VendorReleasedMinor, DeliveryReleasedMinor: allocation.DeliveryReleasedMinor,
			RefundCustomerMinor: allocation.RefundCustomerMinor, RefundPlatformFundingMinor: allocation.RefundPlatformFundingMinor,
			RefundOtherFundingMinor: allocation.RefundOtherFundingMinor, RefundEscrowRecoveryMinor: allocation.RefundEscrowRecoveryMinor,
			RefundVendorRecoveryMinor: allocation.RefundVendorRecoveryMinor, RefundProviderRecoveryMinor: allocation.RefundProviderRecoveryMinor,
			RefundPlatformRecoveryMinor: allocation.RefundPlatformRecoveryMinor, RefundPlatformLossFundingMinor: allocation.RefundPlatformLossFundingMinor,
			RefundVendorMerchandiseReductionMinor: allocation.RefundVendorMerchandiseReductionMinor,
			RefundVendorDeliveryReductionMinor:    allocation.RefundVendorDeliveryReductionMinor,
			RefundProviderDeliveryReductionMinor:  allocation.RefundProviderDeliveryReductionMinor,
			RefundPlatformRevenueReductionMinor:   allocation.RefundPlatformRevenueReductionMinor,
			RefundAuthorizationReleaseMinor:       allocation.RefundAuthorizationReleaseMinor,
		}, nil
	}
	return orders.CommerceAllocationState{}, orders.ErrOrderNotFound
}

func (a *section17FPaymentAuthority) GetCommerceRefundOperation(ctx context.Context, idempotencyKey string) (orders.CommerceFinancialResult, error) {
	if !a.Configured() {
		return orders.CommerceFinancialResult{}, orders.ErrPaymentNotConfigured
	}
	operation, err := a.client.GetCommerceOperation(ctx, "PARTIAL_REFUND", idempotencyKey)
	return commerceFinancialResult(operation), err
}

func commerceFinancialResult(operation walletclient.CommerceOperation) orders.CommerceFinancialResult {
	return orders.CommerceFinancialResult{Reference: operation.ID, Status: operation.Status, AmountMinor: operation.AmountMinor}
}

func (a *section17FPaymentAuthority) ApplyDeliveryFunding(ctx context.Context, request orders.DeliveryFundingRequest) (orders.DeliveryFundingResult, error) {
	if !a.Configured() {
		return orders.DeliveryFundingResult{}, orders.ErrPaymentNotConfigured
	}
	result, err := a.client.ApplyDeliveryFunding(ctx, walletclient.DeliveryFundingRequest{SourceReference: request.SourceReference, VendorOrderID: request.VendorOrderID,
		SponsoringVendorID: request.SponsoringVendorID, ProviderType: request.ProviderType, ProviderID: request.ProviderID, Currency: request.Currency,
		AuthoritativeDeliveryMinor: request.AuthoritativeDeliveryMinor, CustomerContributionMinor: request.CustomerContributionMinor, VendorSubsidyMinor: request.VendorSubsidyMinor,
		PlatformSubsidyMinor: request.PlatformSubsidyMinor, PlatformApprovalRef: request.PlatformApprovalRef,
		CustomerPaymentReference: request.CustomerPaymentReference, IdempotencyKey: request.IdempotencyKey})

	return deliveryFundingResult(result), err
}
func (a *section17FPaymentAuthority) GetDeliveryFunding(ctx context.Context, sourceReference string) (orders.DeliveryFundingResult, error) {
	if !a.Configured() {
		return orders.DeliveryFundingResult{}, orders.ErrPaymentNotConfigured
	}
	result, err := a.client.GetDeliveryFunding(ctx, sourceReference)
	return deliveryFundingResult(result), err
}
func deliveryFundingResult(value walletclient.DeliveryFunding) orders.DeliveryFundingResult {
	return orders.DeliveryFundingResult{Reference: value.ID, Status: value.Status, VendorOrderID: value.VendorOrderID, SponsoringVendorID: value.SponsoringVendorID, AuthoritativeDeliveryMinor: value.AuthoritativeDeliveryMinor, CustomerContributionMinor: value.CustomerContributionMinor, VendorSubsidyMinor: value.VendorSubsidyMinor, PlatformSubsidyMinor: value.PlatformSubsidyMinor}
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
var _ orders.CommercePaymentAuthority = (*section17FPaymentAuthority)(nil)
var _ orders.PaymentSettlementAuthority = (*section17FPaymentAuthority)(nil)
var _ orders.CommerceAllocationStateAuthority = (*section17FPaymentAuthority)(nil)
var _ orders.DeliveryFundingAuthority = (*section17FPaymentAuthority)(nil)
