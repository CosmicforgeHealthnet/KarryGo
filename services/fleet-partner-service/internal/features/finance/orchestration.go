package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// ErrSeparationOfDuties means the acting identity may not act on a request it
// made itself.
var ErrSeparationOfDuties = errors.New("finance: separation of duties forbids acting on your own request")

// Actor identifies the operator or service behind an orchestrated command.
type Actor struct {
	Type          string
	ID            string
	CorrelationID string
	CausationID   string
}

// withAudit runs mutate, its audit record and its outbox event in one
// transaction, so an operator action can never be visible without the record
// that explains it.
func (s *Service) withAudit(ctx context.Context, organizationID, actorType, actorID, action,
	resourceType, resourceID, correlationID string, values map[string]any,
	mutate func(pgx.Tx) error) error {
	if strings.TrimSpace(organizationID) == "" || strings.TrimSpace(actorID) == "" {
		return ErrNotFound
	}
	if actorType == "" {
		actorType = "ORGANIZATION_USER"
	}
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := mutate(tx); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID,
			ActorType: actorType, ActorID: actorID, Action: action,
			ResourceType: resourceType, ResourceID: resourceID, NewValues: values,
			CorrelationID: correlationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, "fleet."+action+".v1", strings.ToUpper(resourceType),
			resourceID, 0, organizationID, correlationID,
			map[string]any{"resourceId": resourceID, "actorType": actorType})
	})
}

// SettlementRequest asks Wallet to settle earnings to a provider or to the
// organization itself.
type SettlementRequest struct {
	Actor                Actor
	OrganizationID       string
	BranchID             string
	BeneficiaryType      string
	BeneficiaryReference string
	EarningAccountID     string
	WalletID             string
	Currency             string
	IdempotencyKey       string
	ExpectedVersion      int64
}

// RequestSettlement queues an authoritative settlement command.
//
// The gate is the projection, not the caller's belief: settling against a
// stale, gapped or reconciling account is how a payout gets made twice.
func (s *Service) RequestSettlement(ctx context.Context, in SettlementRequest) (*Operation, error) {
	beneficiary := strings.ToUpper(strings.TrimSpace(in.BeneficiaryType))
	if beneficiary != "PROVIDER" && beneficiary != "COMPANY" {
		return nil, fmt.Errorf("finance settlement: %w", ErrFinancialStateBlocked)
	}
	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if len(currency) != 3 {
		return nil, fmt.Errorf("finance settlement: %w", ErrFinancialStateBlocked)
	}
	// The eligibility gate runs before any state is read: a suspended,
	// restricted or unsubscribed organization must not be able to probe
	// account state by asking for a settlement it can never get.
	if err := s.requireEligible(ctx, in.OrganizationID, in.BranchID,
		eligibilitymodels.OperationFinancialRelease, in.Actor); err != nil {
		return nil, err
	}
	subject := in.EarningAccountID
	projectionType := "EARNING_ACCOUNT"
	if beneficiary == "COMPANY" {
		subject = in.WalletID
		projectionType = "WALLET"
	}
	if strings.TrimSpace(subject) == "" {
		return nil, fmt.Errorf("finance settlement: %w", ErrFinancialStateBlocked)
	}
	projection, err := s.CurrentProjection(ctx, in.OrganizationID, projectionType, subject)
	if err != nil {
		return nil, err
	}
	// The caller's expected version must match what the authoritative account
	// actually is; otherwise the request describes a state that no longer holds.
	if in.ExpectedVersion != 0 && in.ExpectedVersion != projection.SourceVersion {
		return nil, ErrFinancialStateBlocked
	}
	if err := s.RequireNoOpenReconciliation(ctx, in.OrganizationID, projectionType, subject); err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]any{
		"beneficiaryType": beneficiary, "beneficiaryReference": in.BeneficiaryReference,
		"earningAccountId": in.EarningAccountID, "walletId": in.WalletID, "currency": currency})
	return s.Request(ctx, Request{OrganizationID: in.OrganizationID, BranchID: in.BranchID,
		SubjectType: projectionType, SubjectID: subject, OperationType: "SETTLEMENT_REQUEST",
		IdempotencyKey: in.IdempotencyKey, CorrelationID: in.Actor.CorrelationID,
		CausationID: in.Actor.CausationID, ActorType: in.Actor.Type, ActorID: in.Actor.ID,
		Payload: payload, PayloadVersion: 1, ExpectedVersion: projection.SourceVersion})
}

// RefundRequest asks Wallet to refund against an original posted entry.
type RefundRequest struct {
	Actor             Actor
	OrganizationID    string
	BranchID          string
	OriginalJournalID string
	PaymentReference  string
	AmountMinor       int64
	Currency          string
	ReasonCode        string
	Reason            string
	IdempotencyKey    string
	ExpectedVersion   int64
}

// RequestRefund queues an authoritative refund command.
func (s *Service) RequestRefund(ctx context.Context, in RefundRequest) (*Operation, error) {
	if in.AmountMinor <= 0 || len(strings.TrimSpace(in.Currency)) != 3 ||
		strings.TrimSpace(in.OriginalJournalID) == "" {
		return nil, fmt.Errorf("finance refund: %w", ErrFinancialStateBlocked)
	}
	if err := s.requireEligible(ctx, in.OrganizationID, in.BranchID,
		eligibilitymodels.OperationFinancialReversal, in.Actor); err != nil {
		return nil, err
	}
	// A refund against a payment whose authoritative state Fleet cannot confirm
	// is refused: the amount to give back is not knowable from a stale copy.
	if strings.TrimSpace(in.PaymentReference) != "" {
		verification, err := s.VerifyJobPayment(ctx, in.OrganizationID,
			platformclients.JobPaymentByPaymentRef, in.PaymentReference)
		if err != nil {
			return nil, err
		}
		if verification.Currency != strings.ToUpper(in.Currency) ||
			in.AmountMinor > verification.AmountMinor {
			return nil, ErrFinancialStateBlocked
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"originalJournalId": in.OriginalJournalID, "amountMinor": in.AmountMinor,
		"reasonCode": in.ReasonCode, "reason": in.Reason})
	return s.Request(ctx, Request{OrganizationID: in.OrganizationID, BranchID: in.BranchID,
		SubjectType: "JOURNAL", SubjectID: in.OriginalJournalID, OperationType: "REFUND_REQUEST",
		IdempotencyKey: in.IdempotencyKey, CorrelationID: in.Actor.CorrelationID,
		CausationID: in.Actor.CausationID, ActorType: in.Actor.Type, ActorID: in.Actor.ID,
		Payload: payload, PayloadVersion: 1, ExpectedVersion: in.ExpectedVersion})
}

// ReversalRequest asks Wallet to open a durable reversal.
type ReversalRequest struct {
	Actor             Actor
	OrganizationID    string
	BranchID          string
	OriginalJournalID string
	SourceOperationID string
	AmountMinor       int64
	Currency          string
	ReasonCode        string
	Reason            string
	IdempotencyKey    string
	ExpectedVersion   int64
}

// RequestReversal queues an authoritative reversal command.
//
// When the reversal points at an operation Fleet itself requested, the actor
// who made that request may not be the one asking to undo it. One person
// making and unmaking the same financial fact is exactly what separation of
// duties exists to prevent.
func (s *Service) RequestReversal(ctx context.Context, in ReversalRequest) (*Operation, error) {
	if in.AmountMinor <= 0 || len(strings.TrimSpace(in.Currency)) != 3 ||
		strings.TrimSpace(in.OriginalJournalID) == "" {
		return nil, fmt.Errorf("finance reversal: %w", ErrFinancialStateBlocked)
	}
	if err := s.requireEligible(ctx, in.OrganizationID, in.BranchID,
		eligibilitymodels.OperationFinancialReversal, in.Actor); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.SourceOperationID) != "" {
		var requesterType, requesterID string
		err := s.pool.QueryRow(ctx, `SELECT requested_by_actor_type,requested_by_actor_id
			FROM fleet_partner_finance_operations WHERE id=$1 AND organization_id=$2`,
			in.SourceOperationID, in.OrganizationID).Scan(&requesterType, &requesterID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if requesterID != "" && requesterID == in.Actor.ID && requesterType == in.Actor.Type {
			return nil, ErrSeparationOfDuties
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"originalJournalId": in.OriginalJournalID, "amountMinor": in.AmountMinor,
		"currency":   strings.ToUpper(strings.TrimSpace(in.Currency)),
		"reasonCode": in.ReasonCode, "reason": in.Reason})
	return s.Request(ctx, Request{OrganizationID: in.OrganizationID, BranchID: in.BranchID,
		SubjectType: "JOURNAL", SubjectID: in.OriginalJournalID, OperationType: "REVERSAL_REQUEST",
		IdempotencyKey: in.IdempotencyKey, CorrelationID: in.Actor.CorrelationID,
		CausationID: in.Actor.CausationID, ActorType: in.Actor.Type, ActorID: in.Actor.ID,
		Payload: payload, PayloadVersion: 1, ExpectedVersion: in.ExpectedVersion})
}
