package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/finance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// CurrentProjection returns only a tenant-scoped, current Wallet projection.
// Sensitive callers deliberately cannot opt into stale or gapped data.
func (s *Service) CurrentProjection(ctx context.Context, organizationID, projectionType, reference string) (*models.Projection, error) {
	if organizationID == "" || projectionType == "" || reference == "" {
		return nil, models.ErrFinancialStateBlocked
	}
	p, err := s.repo.CurrentProjection(ctx, organizationID, projectionType, reference)
	if errors.Is(err, models.ErrNotFound) {
		return nil, models.ErrFinancialStateBlocked
	}
	return &p, err
}

func (s *Service) RequirePaidSubscription(ctx context.Context, organizationID string) error {
	status, found, err := s.repo.LatestSubscriptionStatus(ctx, organizationID)
	if err == nil && (!found || !models.StatusIn(status, "ACTIVE")) {
		return models.ErrFinancialStateBlocked
	}
	return err
}

// RequireNoOpenReconciliation refuses to act while an authoritative value is
// being re-established. A reconciliation case is an admission that Fleet's copy
// and Wallet's record disagreed; acting during one is acting on a value nobody
// currently vouches for.
func (s *Service) RequireNoOpenReconciliation(ctx context.Context, organizationID,
	projectionType, reference string) error {
	open, err := s.repo.OpenReconciliation(ctx, organizationID, projectionType, reference)
	if err != nil {
		return models.ErrFinancialStateBlocked
	}
	if open {
		return models.ErrFinancialStateBlocked
	}
	return nil
}

// VerifyJobPayment resolves the authoritative payment for a job.
//
// The Wallet lookup is the authority. Fleet's own PAYMENT_VERIFICATION
// projection is only consulted when it is CURRENT and agrees with what Wallet
// says; a projection that has drifted is treated as no evidence at all.
func (s *Service) VerifyJobPayment(ctx context.Context, organizationID, referenceType,
	reference string) (platformclients.JobPaymentVerification, error) {
	if s.wallet == nil || !s.wallet.Configured() {
		return platformclients.JobPaymentVerification{}, models.ErrFinancialStateBlocked
	}
	if strings.TrimSpace(organizationID) == "" || strings.TrimSpace(reference) == "" {
		return platformclients.JobPaymentVerification{}, models.ErrFinancialStateBlocked
	}
	meta := platformclients.CallMeta{OrganizationID: organizationID,
		CorrelationID: "job-payment-verify-" + reference}
	verification, err := s.wallet.VerifyJobPayment(ctx, meta, organizationID, referenceType, reference)
	if err != nil {
		// Unavailable, forbidden or missing all mean the same thing to a gate:
		// the payment is not proven, so nothing may proceed.
		return platformclients.JobPaymentVerification{}, models.ErrFinancialStateBlocked
	}
	if verification.OrganizationID != "" && verification.OrganizationID != organizationID {
		return platformclients.JobPaymentVerification{}, models.ErrFinancialStateBlocked
	}
	if !verification.Settled {
		return platformclients.JobPaymentVerification{}, models.ErrFinancialStateBlocked
	}
	// A local projection that has fallen behind the authoritative record blocks
	// the operation rather than being silently overridden: the disagreement is
	// itself the signal that something needs reconciling.
	if err := s.requireProjectionAgrees(ctx, organizationID, verification); err != nil {
		return platformclients.JobPaymentVerification{}, err
	}
	return verification, nil
}

// requireProjectionAgrees fails closed when Fleet holds a payment projection
// that is stale, gapped, reconciling or behind the authoritative version.
func (s *Service) requireProjectionAgrees(ctx context.Context, organizationID string,
	verification platformclients.JobPaymentVerification) error {
	freshness, reconciliation, sourceVersion, found, err := s.repo.PaymentProjectionState(ctx, organizationID, verification.PaymentVerificationID)
	if err == nil && !found {
		// No local copy at all is fine: the authoritative answer stands alone.
		return nil
	}
	if err != nil {
		return models.ErrFinancialStateBlocked
	}
	if freshness != "CURRENT" || reconciliation != "IDLE" || sourceVersion > verification.SourceVersion {
		return models.ErrFinancialStateBlocked
	}
	return nil
}

// RequireJobFinancialReadiness binds Wallet payment state to Dispatch-owned
// completion facts. Failed, cancelled, stale, unconfirmed, or unknown work can
// never authorize an earning or settlement request.
//
// The payment half of the gate goes to Wallet's authoritative lookup rather
// than to Fleet's copy: an earning is the moment where a wrong belief about
// payment turns into real money leaving the platform.
func (s *Service) RequireJobFinancialReadiness(ctx context.Context, organizationID, tripID, paymentReference string, requireRecipientConfirmation bool) error {
	if _, err := s.VerifyJobPayment(ctx, organizationID,
		platformclients.JobPaymentByPaymentRef, paymentReference); err != nil {
		return err
	}
	return s.RequireCompletedDelivery(ctx, organizationID, tripID, requireRecipientConfirmation)
}

// RequireCompletedDelivery enforces the Dispatch-owned completion facts.
func (s *Service) RequireCompletedDelivery(ctx context.Context, organizationID, tripID string,
	requireRecipientConfirmation bool) error {
	trip, err := s.repo.TripCompletion(ctx, organizationID, tripID)
	if err != nil || trip.Freshness != "CURRENT" || !trip.Completed || trip.Cancelled || trip.Failed ||
		!models.StatusIn(trip.TripStatus, "COMPLETED", "DELIVERED") || (requireRecipientConfirmation && !trip.RecipientConfirmed) {
		return models.ErrFinancialStateBlocked
	}
	return nil
}

func (s *Service) RequestSubscriptionInvoice(ctx context.Context, in models.SubscriptionInvoiceRequest) (*models.Operation, error) {
	if strings.TrimSpace(in.PlanID) == "" || len(strings.TrimSpace(in.Currency)) != 3 {
		return nil, fmt.Errorf("finance subscription invoice: %w", models.ErrFinancialStateBlocked)
	}
	payload, _ := json.Marshal(map[string]any{"planId": in.PlanID, "currency": strings.ToUpper(in.Currency)})
	return s.Request(ctx, models.Request{OrganizationID: in.OrganizationID, SubjectType: "ORGANIZATION", SubjectID: in.OrganizationID, OperationType: "SUBSCRIPTION_INVOICE", IdempotencyKey: in.IdempotencyKey, CorrelationID: in.CorrelationID, CausationID: in.CausationID, ActorType: in.ActorType, ActorID: in.ActorID, Payload: payload, PayloadVersion: 1, ExpectedVersion: in.ExpectedVersion})
}

func (s *Service) RequestJobEarning(ctx context.Context, in models.JobEarningRequest) (*models.Operation, error) {
	if err := s.RequireJobFinancialReadiness(ctx, in.OrganizationID, in.TripID, in.PaymentReference, in.RequireRecipientConfirmation); err != nil {
		return nil, err
	}
	return s.Request(ctx, models.Request{OrganizationID: in.OrganizationID, SubjectType: "JOB", SubjectID: in.TripID, OperationType: "EARNING_ALLOCATION", IdempotencyKey: in.IdempotencyKey, CorrelationID: in.CorrelationID, CausationID: in.CausationID, ActorType: in.ActorType, ActorID: in.ActorID, Payload: in.Payload, PayloadVersion: 1, ExpectedVersion: in.ExpectedVersion})
}

// RequestWalletOrchestration is the narrow Fleet command boundary for
// non-ledger financial workflows. Wallet remains the sole ledger writer.
func (s *Service) RequestWalletOrchestration(ctx context.Context, in models.Request) (*models.Operation, error) {
	// Releasing a hold and posting a fee move money as surely as a settlement
	// does, so they pass the same gate. PAID_FEATURE covers the rest: an
	// organization whose subscription lapsed does not get to keep using the
	// paid orchestration surface because its requests arrive by a different
	// method name.
	operation := eligibilitymodels.OperationPaidFeature
	switch in.OperationType {
	case "SETTLEMENT_REQUEST", "HOLD_RELEASE", "EARNING_ALLOCATION":
		operation = eligibilitymodels.OperationFinancialRelease
	case "REFUND_REQUEST", "REVERSAL_REQUEST":
		operation = eligibilitymodels.OperationFinancialReversal
	}
	if err := s.requireEligible(ctx, in.OrganizationID, in.BranchID, operation,
		models.Actor{Type: in.ActorType, ID: in.ActorID, CorrelationID: in.CorrelationID,
			CausationID: in.CausationID}); err != nil {
		return nil, err
	}
	switch in.OperationType {
	case "HOLD_CREATE", "HOLD_RELEASE", "COMMISSION_POST", "EARNING_ALLOCATION", "FLEET_FEE_POST", "SETTLEMENT_REQUEST", "REFUND_REQUEST", "REVERSAL_REQUEST":
		return s.Request(ctx, in)
	default:
		return nil, fmt.Errorf("finance orchestration: unsupported operation %q", in.OperationType)
	}
}
