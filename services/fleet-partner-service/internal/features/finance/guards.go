package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

var ErrFinancialStateBlocked = errors.New("finance: authoritative state is not current and eligible")

// CurrentProjection returns only a tenant-scoped, current Wallet projection.
// Sensitive callers deliberately cannot opt into stale or gapped data.
func (s *Service) CurrentProjection(ctx context.Context, organizationID, projectionType, reference string) (*Projection, error) {
	if organizationID == "" || projectionType == "" || reference == "" {
		return nil, ErrFinancialStateBlocked
	}
	p, err := scanProjection(s.pool.QueryRow(ctx, `SELECT `+projectionColumns+`
		FROM fleet_partner_finance_projections
		WHERE organization_id=$1 AND projection_type=$2 AND wallet_reference=$3
		  AND freshness_state='CURRENT' AND reconciliation_status='IDLE'`, organizationID, projectionType, reference))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrFinancialStateBlocked
	}
	return &p, err
}

func (s *Service) RequirePaidSubscription(ctx context.Context, organizationID string) error {
	var status string
	err := s.pool.QueryRow(ctx, `SELECT safe_status FROM fleet_partner_finance_projections
		WHERE organization_id=$1 AND projection_type='SUBSCRIPTION'
		  AND freshness_state='CURRENT' AND reconciliation_status='IDLE'
		ORDER BY last_synced_at DESC LIMIT 1`, organizationID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !statusIn(status, "ACTIVE")) {
		return ErrFinancialStateBlocked
	}
	return err
}

// RequireNoOpenReconciliation refuses to act while an authoritative value is
// being re-established. A reconciliation case is an admission that Fleet's copy
// and Wallet's record disagreed; acting during one is acting on a value nobody
// currently vouches for.
func (s *Service) RequireNoOpenReconciliation(ctx context.Context, organizationID,
	projectionType, reference string) error {
	var open bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM fleet_partner_finance_projections
		WHERE organization_id=$1 AND projection_type=$2 AND wallet_reference=$3
		  AND reconciliation_status<>'IDLE')`, organizationID, projectionType, reference).Scan(&open)
	if err != nil {
		return ErrFinancialStateBlocked
	}
	if open {
		return ErrFinancialStateBlocked
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
		return platformclients.JobPaymentVerification{}, ErrFinancialStateBlocked
	}
	if strings.TrimSpace(organizationID) == "" || strings.TrimSpace(reference) == "" {
		return platformclients.JobPaymentVerification{}, ErrFinancialStateBlocked
	}
	meta := platformclients.CallMeta{OrganizationID: organizationID,
		CorrelationID: "job-payment-verify-" + reference}
	verification, err := s.wallet.VerifyJobPayment(ctx, meta, organizationID, referenceType, reference)
	if err != nil {
		// Unavailable, forbidden or missing all mean the same thing to a gate:
		// the payment is not proven, so nothing may proceed.
		return platformclients.JobPaymentVerification{}, ErrFinancialStateBlocked
	}
	if verification.OrganizationID != "" && verification.OrganizationID != organizationID {
		return platformclients.JobPaymentVerification{}, ErrFinancialStateBlocked
	}
	if !verification.Settled {
		return platformclients.JobPaymentVerification{}, ErrFinancialStateBlocked
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
	var freshness, reconciliation string
	var sourceVersion int64
	err := s.pool.QueryRow(ctx, `SELECT freshness_state,reconciliation_status,source_version
		FROM fleet_partner_finance_projections
		WHERE organization_id=$1 AND projection_type='PAYMENT_VERIFICATION' AND wallet_reference=$2`,
		organizationID, verification.PaymentVerificationID).
		Scan(&freshness, &reconciliation, &sourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		// No local copy at all is fine: the authoritative answer stands alone.
		return nil
	}
	if err != nil {
		return ErrFinancialStateBlocked
	}
	if freshness != "CURRENT" || reconciliation != "IDLE" || sourceVersion > verification.SourceVersion {
		return ErrFinancialStateBlocked
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
	var freshness, tripStatus string
	var completed, cancelled, failed, recipientConfirmed bool
	err := s.pool.QueryRow(ctx, `SELECT freshness_state,trip_status,
		completed_at IS NOT NULL,cancelled_at IS NOT NULL,failed_at IS NOT NULL,
		recipient_confirmed_at IS NOT NULL
		FROM fleet_partner_trip_projections
		WHERE organization_id=$1 AND platform_trip_id=$2`, organizationID, tripID).
		Scan(&freshness, &tripStatus, &completed, &cancelled, &failed, &recipientConfirmed)
	if err != nil || freshness != "CURRENT" || !completed || cancelled || failed ||
		!statusIn(tripStatus, "COMPLETED", "DELIVERED") || (requireRecipientConfirmation && !recipientConfirmed) {
		return ErrFinancialStateBlocked
	}
	return nil
}

func statusIn(value string, allowed ...string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

type SubscriptionInvoiceRequest struct {
	OrganizationID, PlanID, Currency, IdempotencyKey, CorrelationID, CausationID, ActorType, ActorID string
	ExpectedVersion                                                                                  int64
}

func (s *Service) RequestSubscriptionInvoice(ctx context.Context, in SubscriptionInvoiceRequest) (*Operation, error) {
	if strings.TrimSpace(in.PlanID) == "" || len(strings.TrimSpace(in.Currency)) != 3 {
		return nil, fmt.Errorf("finance subscription invoice: %w", ErrFinancialStateBlocked)
	}
	payload, _ := json.Marshal(map[string]any{"planId": in.PlanID, "currency": strings.ToUpper(in.Currency)})
	return s.Request(ctx, Request{OrganizationID: in.OrganizationID, SubjectType: "ORGANIZATION", SubjectID: in.OrganizationID, OperationType: "SUBSCRIPTION_INVOICE", IdempotencyKey: in.IdempotencyKey, CorrelationID: in.CorrelationID, CausationID: in.CausationID, ActorType: in.ActorType, ActorID: in.ActorID, Payload: payload, PayloadVersion: 1, ExpectedVersion: in.ExpectedVersion})
}

type JobEarningRequest struct {
	OrganizationID, TripID, PaymentReference, IdempotencyKey, CorrelationID, CausationID, ActorType, ActorID string
	RequireRecipientConfirmation                                                                             bool
	ExpectedVersion                                                                                          int64
	Payload                                                                                                  json.RawMessage
}

func (s *Service) RequestJobEarning(ctx context.Context, in JobEarningRequest) (*Operation, error) {
	if err := s.RequireJobFinancialReadiness(ctx, in.OrganizationID, in.TripID, in.PaymentReference, in.RequireRecipientConfirmation); err != nil {
		return nil, err
	}
	return s.Request(ctx, Request{OrganizationID: in.OrganizationID, SubjectType: "JOB", SubjectID: in.TripID, OperationType: "EARNING_ALLOCATION", IdempotencyKey: in.IdempotencyKey, CorrelationID: in.CorrelationID, CausationID: in.CausationID, ActorType: in.ActorType, ActorID: in.ActorID, Payload: in.Payload, PayloadVersion: 1, ExpectedVersion: in.ExpectedVersion})
}

// RequestWalletOrchestration is the narrow Fleet command boundary for
// non-ledger financial workflows. Wallet remains the sole ledger writer.
func (s *Service) RequestWalletOrchestration(ctx context.Context, in Request) (*Operation, error) {
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
		Actor{Type: in.ActorType, ID: in.ActorID, CorrelationID: in.CorrelationID,
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
