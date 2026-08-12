package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
)

// Reversal domain events.
const (
	EventReversalRequested = "wallet.reversal.requested.v1"
	EventReversalChanged   = "wallet.reversal.status_changed.v1"
)

// ErrReversalNotReversible means the request exceeds what is left to reverse.
var ErrReversalNotReversible = errors.New("finance: amount exceeds the reversible amount")

// ReversalRequest is the durable reversal command.
//
// AmountMinor and Currency are explicit rather than derived from the original:
// a caller that has the wrong idea of what it is reversing must be rejected,
// not quietly corrected.
type ReversalRequest struct {
	Meta              Meta
	OrganizationID    string
	OriginalJournalID string
	AmountMinor       int64
	Currency          string
	ReasonCode        string
	Reason            string
	ExpectedVersion   int64
}

func (r ReversalRequest) fingerprint() string {
	return fingerprint("REVERSAL", r.OrganizationID, r.OriginalJournalID, r.AmountMinor,
		strings.ToUpper(r.Currency), r.ReasonCode, r.ExpectedVersion)
}

// RequestReversal opens a durable reversal against a posted journal.
//
// The original entry is never touched here — nothing is posted at all. The
// request is recorded, routed through approval when policy demands it, and
// executed later by the reversal worker. That separation is what makes an
// interrupted reversal resumable instead of ambiguous.
func (s *Service) RequestReversal(ctx context.Context, input ReversalRequest) (*models.Reversal, error) {
	if err := input.Meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, input.Meta, financeauth.PermReversalRequest); err != nil {
		return nil, err
	}
	if input.AmountMinor <= 0 {
		return nil, fmt.Errorf("%w: reversal amount must be positive", ErrInvalid)
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if len(currency) != 3 {
		return nil, fmt.Errorf("%w: an ISO currency is required", ErrInvalid)
	}
	if strings.TrimSpace(input.OriginalJournalID) == "" {
		return nil, fmt.Errorf("%w: original journal is required", ErrInvalid)
	}
	if input.Meta.Actor.OrganizationID != "" && input.Meta.Actor.OrganizationID != input.OrganizationID {
		return nil, financeauth.ErrForbidden
	}

	fp := input.fingerprint()
	// An identical repeat returns the original result; a different one under the
	// same key is a conflict, never a second reversal.
	if existing, err := s.reversals.FindByKey(ctx, nil, input.OrganizationID, input.Meta.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != fp {
			return nil, ErrIdempotencyReuse
		}
		return existing, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}

	original, err := s.ledger.GetJournal(ctx, nil, input.OriginalJournalID)
	if err != nil {
		return nil, err
	}
	// Cross-tenant references are rejected before any state is created.
	if original.OrganizationID != input.OrganizationID {
		return nil, financeauth.ErrForbidden
	}
	if original.Currency != currency {
		return nil, fmt.Errorf("%w: currency does not match the original entry", ErrInvalid)
	}
	reversible, err := s.ledger.ReversibleMinor(ctx, nil, original.ID)
	if err != nil {
		return nil, err
	}
	if input.AmountMinor > reversible {
		return nil, fmt.Errorf("%w (%d remaining)", ErrReversalNotReversible, reversible)
	}

	policy, err := s.authorizer.ApprovalPolicyFor(ctx, input.OrganizationID, financeauth.OperationReversalApproval)
	if err != nil {
		return nil, err
	}
	status := models.ReversalRequested
	if policy.MinimumApprovals > 0 {
		status = models.ReversalPendingApproval
	}

	reversal := &models.Reversal{
		OrganizationID:     input.OrganizationID,
		OriginalJournalID:  original.ID,
		OriginalReference:  original.Reference,
		ReasonCode:         input.ReasonCode,
		Reason:             input.Reason,
		AmountMinor:        input.AmountMinor,
		Currency:           currency,
		RequestFingerprint: fp,
		IdempotencyKey:     input.Meta.IdempotencyKey,
		ExpectedVersion:    input.ExpectedVersion,
		Status:             status,
		CorrelationID:      input.Meta.CorrelationID,
		CausationID:        input.Meta.CausationID,
		CreatedByActorType: input.Meta.Actor.Type,
		CreatedByActorID:   input.Meta.Actor.ID,
	}
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.reversals.Create(ctx, tx, reversal); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, input.OrganizationID, "REVERSAL", reversal.ID,
			"", reversal.Status, input.Meta.Actor.Type, input.Meta.Actor.ID,
			reversal.ReasonCode, map[string]any{"originalJournalId": original.ID,
				"amountMinor": reversal.AmountMinor, "currency": reversal.Currency},
			input.Meta.CorrelationID, input.Meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID,
			ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID,
			Action: "reversal.requested", ResourceType: "reversal", ResourceID: reversal.ID,
			NewValues: map[string]any{"status": reversal.Status, "amountMinor": reversal.AmountMinor,
				"currency": reversal.Currency, "originalJournalId": original.ID,
				"reasonCode": reversal.ReasonCode},
			RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventReversalRequested, "REVERSAL", reversal.ID,
			reversal.Version, input.OrganizationID, input.Meta.CorrelationID, input.Meta.CausationID,
			map[string]any{"reversalId": reversal.ID, "status": reversal.Status,
				"originalJournalId": original.ID, "amountMinor": reversal.AmountMinor,
				"currency": reversal.Currency, "reasonCode": reversal.ReasonCode})
	})
	if err != nil {
		return nil, err
	}
	return reversal, nil
}

// GetReversal reads one reversal inside the caller's tenant.
func (s *Service) GetReversal(ctx context.Context, meta Meta,
	organizationID, id string) (*models.Reversal, error) {
	if err := s.AuthorizeRead(ctx, meta, financeauth.PermTransactionView); err != nil {
		return nil, err
	}
	if organizationID == "" {
		return nil, financeauth.ErrForbidden
	}
	return s.reversals.Get(ctx, nil, organizationID, id)
}

// ListReversals reads a tenant's reversals.
func (s *Service) ListReversals(ctx context.Context, meta Meta, organizationID string,
	filter repositories.ListFilter) ([]models.Reversal, error) {
	if err := s.AuthorizeRead(ctx, meta, financeauth.PermTransactionView); err != nil {
		return nil, err
	}
	if organizationID == "" {
		return nil, financeauth.ErrForbidden
	}
	return s.reversals.List(ctx, nil, organizationID, filter)
}

// FindReversalByKey resolves a reversal by the requester's idempotency key.
// This is the authoritative read-back a caller uses after an ambiguous submit.
func (s *Service) FindReversalByKey(ctx context.Context, meta Meta,
	organizationID, key string) (*models.Reversal, error) {
	if err := s.AuthorizeRead(ctx, meta, financeauth.PermTransactionView); err != nil {
		return nil, err
	}
	if organizationID == "" || key == "" {
		return nil, repositories.ErrNotFound
	}
	return s.reversals.FindByKey(ctx, nil, organizationID, key)
}

// ApproveReversal records an approval decision and releases the reversal for
// execution. Separation of duties is checked against the recorded requester
// identity, not against the approver's role.
func (s *Service) ApproveReversal(ctx context.Context, meta Meta, organizationID, id string,
	expectedVersion int64) (*models.Reversal, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	reversal, err := s.reversals.Get(ctx, nil, organizationID, id)
	if err != nil {
		return nil, err
	}
	if reversal.OrganizationID != organizationID {
		return nil, financeauth.ErrForbidden
	}
	if reversal.Version != expectedVersion {
		return nil, repositories.ErrVersionMismatch
	}
	policy, err := s.authorizer.RequireApproval(ctx, meta.Actor, financeauth.PermReversalApprove,
		financeauth.OperationReversalApproval, reversal.RequesterIdentity())
	if err != nil {
		return nil, err
	}
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.reversals.Approve(ctx, tx, id, expectedVersion, meta.Actor.Type,
			meta.Actor.ID, policy.ID, policy.PolicyVersion); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, organizationID, "REVERSAL", id,
			reversal.Status, models.ReversalApproved, meta.Actor.Type, meta.Actor.ID,
			"approved", map[string]any{"approvalPolicyId": policy.ID}, meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID,
			ActorType: meta.Actor.Type, ActorID: meta.Actor.ID, Action: "reversal.approved",
			ResourceType: "reversal", ResourceID: id,
			PreviousValues: map[string]any{"status": reversal.Status},
			NewValues: map[string]any{"status": models.ReversalApproved,
				"approvalPolicyId": policy.ID, "approvalPolicyVersion": policy.PolicyVersion},
			RequestID: meta.RequestID, CorrelationID: meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventReversalChanged, "REVERSAL", id, reversal.Version+1,
			organizationID, meta.CorrelationID, meta.CausationID,
			map[string]any{"reversalId": id, "status": models.ReversalApproved})
	})
	if err != nil {
		return nil, err
	}
	return s.reversals.Get(ctx, nil, organizationID, id)
}

// RejectReversal terminates a reversal request without any ledger effect.
func (s *Service) RejectReversal(ctx context.Context, meta Meta, organizationID, id string,
	expectedVersion int64, reasonCode string) (*models.Reversal, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	reversal, err := s.reversals.Get(ctx, nil, organizationID, id)
	if err != nil {
		return nil, err
	}
	if reversal.Version != expectedVersion {
		return nil, repositories.ErrVersionMismatch
	}
	if _, err := s.authorizer.RequireApproval(ctx, meta.Actor, financeauth.PermReversalApprove,
		financeauth.OperationReversalApproval, reversal.RequesterIdentity()); err != nil {
		return nil, err
	}
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.reversals.Reject(ctx, tx, id, expectedVersion, meta.Actor.Type,
			meta.Actor.ID, reasonCode); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, organizationID, "REVERSAL", id,
			reversal.Status, models.ReversalRejected, meta.Actor.Type, meta.Actor.ID,
			reasonCode, nil, meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID,
			ActorType: meta.Actor.Type, ActorID: meta.Actor.ID, Action: "reversal.rejected",
			ResourceType: "reversal", ResourceID: id,
			PreviousValues: map[string]any{"status": reversal.Status},
			NewValues:      map[string]any{"status": models.ReversalRejected, "reasonCode": reasonCode},
			RequestID:      meta.RequestID, CorrelationID: meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventReversalChanged, "REVERSAL", id, reversal.Version+1,
			organizationID, meta.CorrelationID, meta.CausationID,
			map[string]any{"reversalId": id, "status": models.ReversalRejected, "reasonCode": reasonCode})
	})
	if err != nil {
		return nil, err
	}
	return s.reversals.Get(ctx, nil, organizationID, id)
}

// ReversalHistory returns the append-only transition history for a reversal.
func (s *Service) ReversalHistory(ctx context.Context, meta Meta,
	organizationID, id string) ([]repositories.HistoryEntry, error) {
	if err := s.AuthorizeRead(ctx, meta, financeauth.PermTransactionView); err != nil {
		return nil, err
	}
	// Ownership is proved before history is read: history is metadata about a
	// record the caller must already be entitled to see.
	if _, err := s.reversals.Get(ctx, nil, organizationID, id); err != nil {
		return nil, err
	}
	return repositories.ListHistory(ctx, s.pool, "REVERSAL", id)
}

// processReversal executes or resolves one leased reversal.
func (s *Service) processReversal(ctx context.Context, gateway MoneyGateway,
	config WorkerConfig, reversal models.Reversal) error {
	outcome, err := s.resolveReversalOutcome(ctx, gateway, reversal)
	if err != nil && outcome.Kind == "" {
		outcome = ExternalOutcome{Kind: OutcomeUnknown, SafeErrorCode: "gateway_unavailable"}
	}
	next := time.Now().UTC().Add(backoff(config.BaseBackoff, reversal.Attempts))

	switch outcome.Kind {
	case OutcomeSucceeded:
		postErr := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
			return s.completeReversal(ctx, tx, config, reversal, outcome, next)
		})
		if postErr == nil {
			return nil
		}
		// The compensating journal could not be posted. That is a local,
		// deterministic failure: retry rather than declare the money moved.
		if errors.Is(postErr, repositories.ErrConflict) {
			return s.recordReversalOutcome(ctx, config, reversal, models.ReversalFailedPermanent,
				ExternalOutcome{Kind: OutcomePermanent, ExternalReference: outcome.ExternalReference,
					SafeErrorCode: "already_reversed"}, next, 1)
		}
		return s.recordReversalOutcome(ctx, config, reversal, models.ReversalFailedRetryable,
			ExternalOutcome{Kind: OutcomeRetryable, ExternalReference: outcome.ExternalReference,
				SafeErrorCode: "posting_failed"}, next, config.MaxAttempts)
	case OutcomeRetryable:
		return s.recordReversalOutcome(ctx, config, reversal, models.ReversalFailedRetryable,
			outcome, next, config.MaxAttempts)
	case OutcomePermanent:
		return s.recordReversalOutcome(ctx, config, reversal, models.ReversalFailedPermanent,
			outcome, next, 1)
	default:
		return s.recordReversalOutcome(ctx, config, reversal, models.ReversalUnknown,
			outcome, next, config.MaxAttempts)
	}
}

// resolveReversalOutcome decides whether this attempt executes the external
// operation or reads an earlier one back.
//
// A reversal that already recorded a compensating journal is complete no matter
// what the external provider says, and a reversal whose outcome is ambiguous is
// read back — never re-executed.
func (s *Service) resolveReversalOutcome(ctx context.Context, gateway MoneyGateway,
	reversal models.Reversal) (ExternalOutcome, error) {
	if reversal.ResultingJournalID != "" {
		return ExternalOutcome{Kind: OutcomeSucceeded,
			ExternalReference: reversal.ExternalOperationReference}, nil
	}
	// A compensating journal posted under this key by an interrupted attempt is
	// the authoritative answer: the work is already done.
	if journal, err := s.ledger.FindJournalByIdempotencyKey(ctx, nil,
		reversal.IdempotencyKey+":reversal"); err == nil {
		return ExternalOutcome{Kind: OutcomeSucceeded, ExternalReference: journal.ID}, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return ExternalOutcome{}, err
	}
	if reversal.NeedsReadBack() {
		return gateway.ReadReversal(ctx, reversal)
	}
	return gateway.ExecuteReversal(ctx, reversal)
}

// completeReversal posts the compensating journal and closes the reversal in
// one transaction, so a reversal can never be marked complete without the
// entry that makes it true.
func (s *Service) completeReversal(ctx context.Context, tx pgx.Tx, config WorkerConfig,
	reversal models.Reversal, outcome ExternalOutcome, next time.Time) error {
	journalID := reversal.ResultingJournalID
	if journalID == "" {
		journal, err := s.ledger.PostCompensating(ctx, tx, reversal.OriginalJournalID,
			reversal.AmountMinor, models.JournalReversal,
			defaultReason(reversal.ReasonCode, reversal.Reason), "SYSTEM", config.Owner,
			reversal.IdempotencyKey+":reversal", reversal.CorrelationID)
		if err != nil {
			return err
		}
		journalID = journal.ID
	}
	if err := s.reversals.SetOutcome(ctx, tx, reversal.ID, config.Owner, models.ReversalCompleted,
		outcome.ExternalReference, journalID, next, "", config.MaxAttempts); err != nil {
		return err
	}
	if err := repositories.AppendHistory(ctx, tx, reversal.OrganizationID, "REVERSAL", reversal.ID,
		reversal.Status, models.ReversalCompleted, "SYSTEM", config.Owner, "compensating journal posted",
		map[string]any{"resultingJournalId": journalID}, reversal.CorrelationID,
		reversal.OriginalJournalID); err != nil {
		return err
	}
	if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: reversal.OrganizationID,
		ActorType: "SYSTEM", ActorID: config.Owner, Action: "reversal.completed",
		ResourceType: "reversal", ResourceID: reversal.ID,
		PreviousValues: map[string]any{"status": reversal.Status},
		NewValues: map[string]any{"status": models.ReversalCompleted,
			"resultingJournalId": journalID}, CorrelationID: reversal.CorrelationID}); err != nil {
		return err
	}
	return s.events.Publish(ctx, tx, EventReversalCompleted, "REVERSAL", reversal.ID,
		reversal.Version+1, reversal.OrganizationID, reversal.CorrelationID,
		reversal.OriginalJournalID, map[string]any{"reversalId": reversal.ID,
			"status": models.ReversalCompleted, "originalJournalId": reversal.OriginalJournalID,
			"resultingJournalId": journalID, "amountMinor": reversal.AmountMinor,
			"currency": reversal.Currency})
}

func (s *Service) recordReversalOutcome(ctx context.Context, config WorkerConfig,
	reversal models.Reversal, status string, outcome ExternalOutcome, next time.Time,
	maxAttempts int) error {
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.reversals.SetOutcome(ctx, tx, reversal.ID, config.Owner, status,
			outcome.ExternalReference, "", next, outcome.SafeErrorCode, maxAttempts); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, reversal.OrganizationID, "REVERSAL",
			reversal.ID, reversal.Status, status, "SYSTEM", config.Owner, outcome.SafeErrorCode,
			nil, reversal.CorrelationID, reversal.OriginalJournalID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: reversal.OrganizationID,
			ActorType: "SYSTEM", ActorID: config.Owner, Action: "reversal.status_changed",
			ResourceType: "reversal", ResourceID: reversal.ID,
			PreviousValues: map[string]any{"status": reversal.Status},
			NewValues:      map[string]any{"status": status, "safeErrorCode": outcome.SafeErrorCode},
			CorrelationID:  reversal.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventReversalChanged, "REVERSAL", reversal.ID,
			reversal.Version+1, reversal.OrganizationID, reversal.CorrelationID,
			reversal.OriginalJournalID, map[string]any{"reversalId": reversal.ID,
				"status": status, "safeErrorCode": outcome.SafeErrorCode})
	})
}

func defaultReason(reasonCode, reason string) string {
	if strings.TrimSpace(reason) != "" {
		return reason
	}
	if strings.TrimSpace(reasonCode) != "" {
		return reasonCode
	}
	return "reversal"
}
