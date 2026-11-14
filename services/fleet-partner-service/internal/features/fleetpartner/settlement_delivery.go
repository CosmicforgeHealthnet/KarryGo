package fleetpartner

// FLEET-13.11: getting a committed settlement instruction to payment-wallet.
//
// RecordCompletedWork commits the instruction and, in the same transaction,
// a durable delivery job. That job is the only thing that sends the
// instruction, so an instruction can never be committed without a way to
// reach the money authority, and a crash between commit and send loses
// nothing: the job is leased, retried with backoff and, if it dead-letters on
// a transient condition, re-created by the recovery sweep.
//
// Exactly-once is held in three places that do not depend on each other:
// the job's dedupe key (one live delivery job per instruction), the posting
// operation's unique idempotency key (one operation per instruction), and
// Wallet's own posting identity (one journal per instruction id + version).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// JobManagedSettlementDelivery is the durable job that sends one instruction.
const JobManagedSettlementDelivery = "SECTION12_MANAGED_SETTLEMENT_DELIVERY"

// settlementDeliveryMaxAttempts bounds one delivery job. A job that exhausts
// it on a transient condition dead-letters; the instruction stays
// PENDING_WALLET and the recovery sweep starts a fresh job.
const settlementDeliveryMaxAttempts = 12

// Instruction statuses.
const (
	InstructionCreated                = "CREATED"
	InstructionPendingWallet          = "PENDING_WALLET"
	InstructionReconciliationRequired = "RECONCILIATION_REQUIRED"
	InstructionPosted                 = "POSTED"
	InstructionFailedTerminal         = "FAILED_TERMINAL"
	InstructionSuperseded             = "SUPERSEDED"
	InstructionCancelled              = "CANCELLED"
)

// Delivery dispositions reported by the money-authority port. They match the
// financeops posting dispositions one for one.
const (
	SettlementPosted         = "POSTED"
	SettlementRetry          = "RETRY"
	SettlementOutcomeUnknown = "OUTCOME_UNKNOWN"
	SettlementRefused        = "REFUSED"
	SettlementUnavailable    = "UNAVAILABLE"
)

// SettlementSubmission is one instruction as the money authority receives it.
// Every figure is copied from the committed instruction; nothing is
// recomputed at delivery time.
type SettlementSubmission struct {
	InstructionID      string
	InstructionVersion int64

	OrganizationID string
	MembershipID   string
	ProviderID     string
	ProviderType   string

	SourceService      string
	SourceJobID        string
	SourceAssignmentID string

	Currency               string
	GrossBasisMinor        int64
	ProviderShareMinor     int64
	OrganizationShareMinor int64

	CompensationPlanID            string
	CompensationPlanVersion       int64
	CompensationResolutionVersion int64

	CorrelationID  string
	IdempotencyKey string
}

// SettlementOutcome is what one submission established.
type SettlementOutcome struct {
	OperationID      string
	Disposition      string
	Reference        string
	ConfirmedVersion int64
	ErrorCode        string
}

// SettlementSubmitter is the money-authority port. The financeops posting
// service implements it (wired in internal/app); Fleet never posts a journal
// itself.
type SettlementSubmitter interface {
	SubmitManagedSettlement(ctx context.Context, submission SettlementSubmission) (SettlementOutcome, error)
}

// SetSettlementSubmitter wires the money-authority port. Without it every
// delivery attempt is refused as a dependency outage and nothing is sent.
func (s *Service) SetSettlementSubmitter(submitter SettlementSubmitter) { s.settlement = submitter }

// walletSettlementIdentity maps Fleet's source service onto the names
// payment-wallet keys a managed settlement on.
//
// source_service is the lower-case service name Wallet's managed-settlement
// contract and the shared read client (shared/go/managedsettlement, which
// matches case-insensitively) both use. provider_type is the namespace of the
// provider's payable account: hauling providers are credited under
// "truck_provider", which is where the truck provider app reads its balance.
func walletSettlementIdentity(source string) (walletSource, providerType string, ok bool) {
	switch source {
	case ServiceTaxi:
		return "taxi", "taxi", true
	case ServiceDispatch:
		return "dispatch", "dispatch", true
	case ServiceHauling:
		return "hauling", "truck_provider", true
	default:
		return "", "", false
	}
}

// enqueueSettlementDelivery creates the delivery job inside the caller's
// transaction, so the instruction and its delivery commit together.
func (s *Service) enqueueSettlementDelivery(ctx context.Context, q database.Querier, organizationID, instructionID, correlationID string) error {
	_, err := jobs.NewQueue(s.pool).Enqueue(ctx, q, jobs.EnqueueRequest{
		OrganizationID: organizationID,
		JobType:        JobManagedSettlementDelivery,
		DedupeKey:      "settlement-instruction:" + instructionID,
		Payload:        map[string]string{"organizationId": organizationID, "instructionId": instructionID},
		CorrelationID:  correlationID,
		MaxAttempts:    settlementDeliveryMaxAttempts,
		// A delivery may end with an unknown outcome; the next attempt must
		// read back before it sends.
		AuthoritativeReadbackRequired: true,
	})
	if err != nil {
		return fmt.Errorf("fleetpartner: enqueue settlement delivery: %w", err)
	}
	return nil
}

// RegisterWorkers binds the delivery job handler.
func (s *Service) RegisterWorkers(w *jobs.Worker) {
	if w == nil {
		return
	}
	w.Register(JobManagedSettlementDelivery, s.HandleSettlementDeliveryJob)
}

// HandleSettlementDeliveryJob is the durable job handler.
func (s *Service) HandleSettlementDeliveryJob(ctx context.Context, job jobs.Job) error {
	var payload struct {
		OrganizationID string `json:"organizationId"`
		InstructionID  string `json:"instructionId"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return &jobs.ClassifiedError{Class: jobs.ErrorClassValidation, Code: "invalid_payload", SafeMessage: "settlement delivery payload is not valid", Err: err}
	}
	if payload.OrganizationID == "" || payload.OrganizationID != job.OrganizationID || payload.InstructionID == "" {
		return &jobs.ClassifiedError{Class: jobs.ErrorClassValidation, Code: "tenant_mismatch", SafeMessage: "settlement delivery payload does not match its job", Err: jobs.ErrTenantMismatch}
	}
	_, err := s.DeliverSettlementInstruction(ctx, payload.OrganizationID, payload.InstructionID, job.CorrelationID)
	return err
}

// DeliverSettlementInstruction sends one committed instruction to the money
// authority and records what happened.
//
// The returned error is a jobs.ClassifiedError whose class tells the job queue
// what to do: nothing (posted or already final), retry (transient or
// unavailable), read back (unknown outcome) or stop (refused).
func (s *Service) DeliverSettlementInstruction(ctx context.Context, organizationID, instructionID, correlationID string) (ManagedWorkSettlementInstruction, error) {
	if !s.flags.SettlementInstruction {
		return ManagedWorkSettlementInstruction{}, &jobs.ClassifiedError{Class: jobs.ErrorClassDependencyUnavailable, Code: "settlement_instruction_disabled", SafeMessage: "settlement instruction stage is disabled"}
	}
	if s.settlement == nil {
		return ManagedWorkSettlementInstruction{}, &jobs.ClassifiedError{Class: jobs.ErrorClassDependencyUnavailable, Code: "settlement_submitter_unwired", SafeMessage: "no money authority is wired"}
	}
	instruction, err := s.repo.SettlementInstruction(ctx, s.pool, organizationID, instructionID, false)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ManagedWorkSettlementInstruction{}, &jobs.ClassifiedError{Class: jobs.ErrorClassValidation, Code: "instruction_not_found", SafeMessage: "settlement instruction was not found", Err: err}
		}
		return ManagedWorkSettlementInstruction{}, err
	}
	if settlementInstructionFinal(instruction.Status) {
		return instruction, nil
	}
	// One sender per instruction at a time. The job dedupe key already
	// guarantees one live job, but a manual replay or an operator-run sweep
	// must not be able to put two requests for the same money in flight.
	release, held, err := s.lockSettlementDelivery(ctx, instruction.InstructionID)
	if err != nil {
		return instruction, err
	}
	if !held {
		return instruction, &jobs.ClassifiedError{Class: jobs.ErrorClassTransient, Code: "delivery_in_progress", SafeMessage: "another delivery of this instruction is in progress"}
	}
	defer release()
	// Re-read under the lock: the delivery that held it may have finished.
	if instruction, err = s.repo.SettlementInstruction(ctx, s.pool, organizationID, instructionID, false); err != nil {
		return instruction, err
	}
	if settlementInstructionFinal(instruction.Status) {
		return instruction, nil
	}
	// The correlation id travels inside the signed instruction, and the
	// instruction is fingerprinted, so it must be the same on every attempt.
	// It is taken from the completed work the instruction settles, never
	// from whichever job happens to be delivering it.
	correlationID = s.instructionCorrelation(ctx, instruction)
	walletSource, providerType, ok := walletSettlementIdentity(instruction.SourceService)
	if !ok {
		outcome := SettlementOutcome{Disposition: SettlementRefused, ErrorCode: "unsupported_source_service"}
		applied, applyErr := s.applySettlementOutcome(ctx, instruction, outcome, correlationID)
		if applyErr != nil {
			return instruction, applyErr
		}
		return applied, &jobs.ClassifiedError{Class: jobs.ErrorClassPermanent, Code: outcome.ErrorCode, SafeMessage: "settlement instruction has no money-authority identity"}
	}
	submission := SettlementSubmission{
		InstructionID: instruction.InstructionID, InstructionVersion: instruction.Version,
		OrganizationID: instruction.OrganizationID, MembershipID: instruction.MembershipID,
		ProviderID: instruction.ProviderID, ProviderType: providerType,
		SourceService: walletSource, SourceJobID: instruction.SourceJobID,
		SourceAssignmentID: instruction.SourceAssignmentID, Currency: instruction.Currency,
		GrossBasisMinor: instruction.GrossBasisMinor, ProviderShareMinor: instruction.ProviderShareMinor,
		OrganizationShareMinor: instruction.OrganizationShareMinor,
		CompensationPlanID:     instruction.CompensationPlanID, CompensationPlanVersion: instruction.CompensationPlanVersion,
		CompensationResolutionVersion: instruction.CompensationResolutionVersion,
		CorrelationID:                 correlationID, IdempotencyKey: instruction.IdempotencyKey,
	}
	outcome, submitErr := s.settlement.SubmitManagedSettlement(ctx, submission)
	if outcome.Disposition == "" {
		// A port that returned no disposition told us nothing; treat it as
		// the one answer that never causes a second payment.
		outcome.Disposition = SettlementOutcomeUnknown
		if submitErr == nil {
			submitErr = errors.New("money authority returned no disposition")
		}
	}
	applied, applyErr := s.applySettlementOutcome(ctx, instruction, outcome, correlationID)
	if applyErr != nil {
		return instruction, applyErr
	}
	return applied, classifySettlementOutcome(outcome, submitErr)
}

// lockSettlementDelivery takes a session advisory lock keyed on the
// instruction, on a dedicated connection held for the delivery. The lock is
// released explicitly and, if the process dies, with the connection.
func (s *Service) lockSettlementDelivery(ctx context.Context, instructionID string) (func(), bool, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	var held bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('fleet-settlement-delivery:' || $1, 0))`, instructionID).Scan(&held); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !held {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended('fleet-settlement-delivery:' || $1, 0))`, instructionID)
		conn.Release()
	}, true, nil
}

func (s *Service) instructionCorrelation(ctx context.Context, instruction ManagedWorkSettlementInstruction) string {
	key := strings.TrimPrefix(instruction.IdempotencyKey, "settlement:")
	record, err := s.repo.CompletedWorkByIdempotency(ctx, instruction.OrganizationID, key)
	if err != nil || record.CorrelationID == "" {
		return instruction.InstructionID
	}
	return record.CorrelationID
}

func settlementInstructionFinal(status string) bool {
	switch status {
	case InstructionPosted, InstructionFailedTerminal, InstructionSuperseded, InstructionCancelled:
		return true
	default:
		return false
	}
}

func instructionStatusFor(disposition string) string {
	switch disposition {
	case SettlementPosted:
		return InstructionPosted
	case SettlementRefused:
		return InstructionFailedTerminal
	case SettlementOutcomeUnknown:
		return InstructionReconciliationRequired
	default:
		return InstructionPendingWallet
	}
}

func classifySettlementOutcome(outcome SettlementOutcome, err error) error {
	switch outcome.Disposition {
	case SettlementPosted:
		return nil
	case SettlementRefused:
		return &jobs.ClassifiedError{Class: jobs.ErrorClassPermanent, Code: outcome.ErrorCode, SafeMessage: "money authority refused the settlement instruction", Err: err}
	case SettlementOutcomeUnknown:
		return &jobs.ClassifiedError{Class: jobs.ErrorClassUnknownExternalOutcome, Code: outcome.ErrorCode, SafeMessage: "settlement outcome is unknown; it will be read back", Err: err}
	case SettlementUnavailable:
		return &jobs.ClassifiedError{Class: jobs.ErrorClassDependencyUnavailable, Code: outcome.ErrorCode, SafeMessage: "settlement posting is unavailable", Err: err}
	default:
		return &jobs.ClassifiedError{Class: jobs.ErrorClassTransient, Code: outcome.ErrorCode, SafeMessage: "settlement delivery failed transiently", Err: err}
	}
}

// applySettlementOutcome records one delivery result on the instruction.
//
// The row is locked and re-read first, so a delivery racing another (or a
// stale outcome arriving after a confirmed one) can never move a POSTED
// instruction anywhere else.
func (s *Service) applySettlementOutcome(ctx context.Context, instruction ManagedWorkSettlementInstruction, outcome SettlementOutcome, correlationID string) (ManagedWorkSettlementInstruction, error) {
	next := instructionStatusFor(outcome.Disposition)
	var applied ManagedWorkSettlementInstruction
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := s.repo.SettlementInstruction(ctx, tx, instruction.OrganizationID, instruction.InstructionID, true)
		if err != nil {
			return err
		}
		if settlementInstructionFinal(current.Status) {
			applied = current
			return nil
		}
		applied, err = s.repo.RecordSettlementDelivery(ctx, tx, current, next, outcome.OperationID, outcome.Reference, outcome.ErrorCode)
		if err != nil {
			return err
		}
		if next == InstructionPosted && outcome.OperationID != "" {
			if err := s.repo.LinkWorkSettledComponentsToPosting(ctx, tx, current.OrganizationID, current.InstructionID, outcome.OperationID); err != nil {
				return err
			}
		}
		if next == current.Status {
			// Another transient attempt: the attempt counter moved, the
			// meaning did not. No audit or event for a non-transition.
			return nil
		}
		t := tenant.Context{OrganizationID: current.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "managed-settlement-delivery", CorrelationID: correlationID}
		event := "fleet.settlement_instruction." + strings.ToLower(next) + ".v1"
		if s.audit != nil {
			entry := audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), "settlement_instruction", current.InstructionID)
			entry.PreviousValues = map[string]any{"status": current.Status}
			entry.NewValues = map[string]any{"status": next, "walletPostingOperationId": outcome.OperationID, "errorCode": outcome.ErrorCode}
			if err := s.audit.Record(ctx, tx, entry); err != nil {
				return err
			}
		}
		if s.events != nil {
			// The instruction's own version is immutable (it is part of
			// Wallet's posting identity); delivery transitions are sequenced
			// by the attempt counter instead.
			return s.events.Publish(ctx, tx, event, "settlement_instruction", current.InstructionID, int64(applied.DeliveryAttempts)+1,
				current.OrganizationID, correlationID, map[string]any{
					"instructionId": current.InstructionID, "status": next, "previousStatus": current.Status,
					"walletPostingOperationId": outcome.OperationID, "errorCode": outcome.ErrorCode,
				})
		}
		return nil
	})
	return applied, err
}

// ScheduleSettlementDelivery is the recovery sweep. It creates a delivery job
// for every undelivered instruction that has no live one: instructions whose
// job dead-lettered on a transient condition, and any committed before
// delivery existed. The job dedupe key keeps it from ever creating a second
// live job for one instruction.
func (s *Service) ScheduleSettlementDelivery(ctx context.Context, limit int) (int, error) {
	if !s.flags.SettlementInstruction || s.settlement == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 200
	}
	pending, err := s.repo.UndeliveredSettlementInstructions(ctx, JobManagedSettlementDelivery, limit)
	if err != nil {
		return 0, err
	}
	for _, p := range pending {
		if err := s.enqueueSettlementDelivery(ctx, s.pool, p.OrganizationID, p.InstructionID, ""); err != nil {
			return 0, err
		}
	}
	return len(pending), nil
}
