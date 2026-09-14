package walletrepositories

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/events"
	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 12 WAL-019: the durable events a managed settlement produces.
//
// These are appended to the transactional outbox inside the very transaction
// that posts (or reverses) the journal. That placement is the whole point: an
// event written afterwards can be lost while the money has already moved, and
// a consumer reconciling from events would then be permanently wrong about a
// settlement that did happen. Written with the journal, the event either
// exists with the money or neither does.
const (
	// EventManagedSettlementPosted announces that one instruction's journal
	// posted and the entitlements are recorded.
	EventManagedSettlementPosted = "wallet.managed_settlement.posted.v1"

	// EventManagedSettlementFailed announces that an instruction will not post
	// as stated. It is NOT emitted for an ambiguous outcome: "we could not
	// tell" is not a failure, and publishing it as one is how a settlement
	// that actually succeeded gets treated as owed.
	EventManagedSettlementFailed = "wallet.managed_settlement.failed.v1"

	// EventManagedSettlementReversed announces a compensating journal. The
	// original posted event is not retracted, because it remains true: the
	// money moved, and then it moved back.
	EventManagedSettlementReversed = "wallet.managed_settlement.reversed.v1"
)

// managedSettlementEventPayloadVersion is carried on the wire so a consumer
// can tell a payload it understands from one it does not, without inferring it
// from which fields happen to be present.
const managedSettlementEventPayloadVersion = 1

// managedSettlementEvent is the payload shape shared by all three events.
//
// Deliberately identifiers and status only. No rate, no basis, no rule: a
// consumer that could recompute an entitlement from this payload would
// eventually be asked to, and then two authorities would be deciding one
// number.
type managedSettlementEvent struct {
	EventType          string    `json:"eventType"`
	PayloadVersion     int       `json:"payloadVersion"`
	InstructionID      string    `json:"instructionId"`
	InstructionVersion int64     `json:"instructionVersion"`
	SourceService      string    `json:"sourceService"`
	SourceJobID        string    `json:"sourceJobId"`
	OrganizationID     string    `json:"organizationId"`
	MembershipID       string    `json:"membershipId,omitempty"`
	ProviderID         string    `json:"providerId"`
	ProviderType       string    `json:"providerType,omitempty"`
	Currency           string    `json:"currency"`
	Status             string    `json:"status"`
	FailureCode        string    `json:"failureCode,omitempty"`
	ReasonCode         string    `json:"reasonCode,omitempty"`
	OccurredAt         time.Time `json:"occurredAt"`
	CorrelationID      string    `json:"correlationId,omitempty"`
}

// appendManagedSettlementEvent writes one settlement event to the outbox.
//
// The aggregate is the instruction, versioned by the instruction's own
// version, so a consumer can reject a replayed or out-of-order delivery
// without needing to understand settlement semantics at all.
func appendManagedSettlementEvent(
	ctx context.Context, tx pgx.Tx, eventType string,
	settlement walletmodels.ManagedSettlement, reasonCode string,
) error {
	payload := managedSettlementEvent{
		EventType:          eventType,
		PayloadVersion:     managedSettlementEventPayloadVersion,
		InstructionID:      settlement.InstructionID,
		InstructionVersion: settlement.InstructionVersion,
		SourceService:      settlement.SourceService,
		SourceJobID:        settlement.SourceJobID,
		OrganizationID:     settlement.OrganizationID,
		MembershipID:       settlement.MembershipID,
		ProviderID:         settlement.ProviderID,
		ProviderType:       settlement.ProviderType,
		Currency:           settlement.Currency,
		Status:             settlement.Status,
		FailureCode:        settlement.FailureCode,
		ReasonCode:         reasonCode,
		OccurredAt:         time.Now().UTC(),
		CorrelationID:      settlement.CorrelationID,
	}
	envelope, err := events.NewEnvelope(
		eventType, managedSettlementEventPayloadVersion,
		"MANAGED_SETTLEMENT", settlement.InstructionID, settlement.InstructionVersion,
		settlement.OrganizationID, settlement.CorrelationID, "", payload, false)
	if err != nil {
		return err
	}
	return events.NewOutboxRepository(tx).Append(ctx, tx, envelope)
}

// terminalRefusal reports whether an error means "this instruction will never
// post as stated".
//
// The distinction matters more than it looks. A terminal refusal is a durable
// fact Fleet must learn about even if the HTTP response is lost -- the
// instruction is wrong and someone has to correct it. A transient or internal
// error is not: the same instruction resent unchanged may well post, and
// recording it as failed would tell Fleet a job is unsettleable when it is
// merely unsettled. Under ambiguity this answers false, so nothing is written.
func terminalRefusal(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if errors.Is(err, ErrManagedSettlementConflict) {
		return "INSTRUCTION_IDENTITY_REUSED", true
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		return "", false
	}
	switch appErr.Code {
	case apperrors.CodeConflict:
		return "INSTRUCTION_REFUSED", true
	case apperrors.CodeNotFound:
		return "JOB_NOT_FOUND", true
	case apperrors.CodeBadRequest, apperrors.CodeValidationFailed:
		return "INSTRUCTION_INVALID", true
	default:
		// Internal, unavailable, rate-limited: transient or ambiguous.
		return "", false
	}
}

// recordManagedSettlementRefusal durably records that an instruction was
// refused, and announces it (WAL-019).
//
// It runs in its own transaction, after the posting transaction has rolled
// back -- the refusal has to survive precisely because the posting did not.
//
// It is idempotent by the instruction's own identity (WAL-017A): a resent
// instruction that is refused again writes nothing further and emits no second
// event, so a consumer counting failures sees one refusal per instruction
// rather than one per retry. If a row already exists under that identity --
// including a POSTED one -- nothing is written at all, because a settlement
// that posted is not then also a failure.
func (r *PostgresWalletRepository) recordManagedSettlementRefusal(
	ctx context.Context, in walletmodels.ManagedSettlementInstruction, failureCode string,
) error {
	in = in.Normalized()
	return r.withTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO managed_settlements (
				id, instruction_id, instruction_version,
				organization_id, membership_id, provider_id, provider_type,
				source_service, source_job_id, source_assignment_id,
				currency, gross_basis_minor, provider_share_minor, organization_share_minor, platform_share_minor,
				compensation_plan_id, compensation_plan_version, compensation_resolution_version,
				status, failure_code, idempotency_key, request_fingerprint, correlation_id
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
			ON CONFLICT (instruction_id, instruction_version) DO NOTHING
			RETURNING `+managedSettlementColumns,
			uuid.NewString(), in.InstructionID, in.InstructionVersion,
			in.OrganizationID, in.MembershipID, in.ProviderID, in.ProviderType,
			in.SourceService, in.SourceJobID, in.SourceAssignmentID,
			in.Currency, in.GrossBasisMinor, in.ProviderShareMinor, in.OrganizationShareMinor, 0,
			in.CompensationPlanID, in.CompensationPlanVersion, in.CompensationResolutionVersion,
			walletmodels.ManagedSettlementFailedTerminal, failureCode,
			"failed:"+in.IdempotencyKey, "", in.CorrelationID,
		)
		settlement, err := scanManagedSettlement(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Already recorded under this identity. Nothing happened, so
				// nothing is announced.
				return nil
			}
			return err
		}
		return appendManagedSettlementEvent(ctx, tx, EventManagedSettlementFailed, settlement, failureCode)
	})
}
