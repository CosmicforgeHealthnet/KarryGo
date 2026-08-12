package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// ExecutionStartWorkerConfig tunes the worker.
type ExecutionStartWorkerConfig struct {
	Owner          string
	Lease          time.Duration
	BatchSize      int
	MaxAttempts    int
	BaseBackoff    time.Duration
	MaxBackoff     time.Duration
	EnabledSources []string
	WritesEnabled  func() bool
}

func (c ExecutionStartWorkerConfig) withDefaults() ExecutionStartWorkerConfig {
	if c.Owner == "" {
		c.Owner = "execution-start-worker"
	}
	if c.Lease <= 0 {
		c.Lease = 2 * time.Minute
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 10
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 5 * time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 10 * time.Minute
	}
	if c.EnabledSources == nil {
		c.EnabledSources = []string{models.SourceDispatch, models.SourceHauling}
	}
	return c
}

// backoffFor computes bounded exponential backoff.
func (c ExecutionStartWorkerConfig) backoffFor(attempt int) time.Duration {
	delay := c.BaseBackoff
	for i := 0; i < attempt && delay < c.MaxBackoff; i++ {
		delay *= 2
	}
	if delay > c.MaxBackoff {
		delay = c.MaxBackoff
	}
	return delay
}

// ExecutionStartResult summarises one worker pass.
type ExecutionStartResult struct {
	Claimed   int
	Confirmed int
	Rejected  int
	Unknown   int
	Retried   int
	Recovered int64
}

// RunExecutionStart processes one batch of durable execution-start work.
//
// Shape of each iteration, and why:
//  1. claim and COMMIT — so a crash releases the lease by timeout instead of
//     stranding the row;
//  2. call dispatch with NO database transaction open — holding a row lock
//     across an HTTP call would tie a database transaction to another
//     service's availability;
//  3. apply the outcome in a second short transaction.
func (s *Service) RunExecutionStart(ctx context.Context, config ExecutionStartWorkerConfig) (ExecutionStartResult, error) {
	config = config.withDefaults()
	var result ExecutionStartResult
	if config.WritesEnabled != nil && !config.WritesEnabled() {
		return result, nil
	}

	recovered, err := s.opts.ExecutionStarts.RecoverExpiredLeases(ctx, nil)
	if err != nil {
		return result, err
	}
	result.Recovered = recovered

	var claimed []models.ExecutionStartOperation
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		var claimErr error
		claimed, claimErr = s.opts.ExecutionStarts.ClaimEnabled(ctx, tx, config.Owner, config.Lease, config.BatchSize, config.EnabledSources)
		return claimErr
	})
	if err != nil {
		return result, err
	}
	result.Claimed = len(claimed)

	for index := range claimed {
		operation := claimed[index]
		outcome, err := s.processExecutionStart(ctx, operation, config)
		if err != nil {
			return result, err
		}
		switch outcome {
		case models.ExecutionStartConfirmed:
			result.Confirmed++
		case models.ExecutionStartRejected:
			result.Rejected++
		case models.ExecutionStartUnknown:
			result.Unknown++
		default:
			result.Retried++
		}
	}
	return result, nil
}

// processExecutionStart handles one operation end to end.
func (s *Service) processExecutionStart(ctx context.Context, operation models.ExecutionStartOperation,
	config ExecutionStartWorkerConfig) (string, error) {
	if config.WritesEnabled != nil && !config.WritesEnabled() {
		return models.ExecutionStartRetryScheduled, s.opts.ExecutionStarts.Fail(ctx, nil, operation.ID, "integration_kill_switch", config.backoffFor(operation.AttemptCount), config.MaxAttempts, false)
	}
	// Re-read state under the claim: the provider may have been cancelled or
	// the assignment may have moved since the row was queued.
	assignment, err := s.opts.Assignments.GetByID(ctx, operation.OrganizationID, operation.AssignmentID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return models.ExecutionStartRejected, s.opts.ExecutionStarts.Fail(ctx, nil,
				operation.ID, "assignment_not_found", 0, config.MaxAttempts, true)
		}
		return "", err
	}
	// Only an accepted assignment may start execution. Anything else means the
	// world changed after acceptance and this work is void.
	if assignment.Status != models.AssignmentAccepted &&
		assignment.Status != models.AssignmentExecutionStartPending {
		return models.ExecutionStartRejected, s.opts.ExecutionStarts.Fail(ctx, nil,
			operation.ID, "assignment_not_executable", 0, config.MaxAttempts, true)
	}

	// The provider's acceptance must still exist and still be an acceptance.
	response, err := s.opts.Assignments.GetResponse(ctx, nil, operation.AssignmentID)
	if err != nil || response.Response != models.ResponseAccepted {
		return models.ExecutionStartRejected, s.opts.ExecutionStarts.Fail(ctx, nil,
			operation.ID, "provider_acceptance_not_confirmed", 0, config.MaxAttempts, true)
	}

	projection, err := s.opts.Projections.GetByID(ctx, operation.OrganizationID, operation.RequestProjectionID)
	if err != nil {
		return "", err
	}
	// A stale or gapped projection means we do not reliably know the source
	// state; issuing a state-changing command on that basis is unsafe.
	if models.BlocksStateChangingCommands(projection.FreshnessState) {
		return models.ExecutionStartRetryScheduled, s.opts.ExecutionStarts.Fail(ctx, nil,
			operation.ID, "marketplace_projection_stale",
			config.backoffFor(operation.AttemptCount), config.MaxAttempts, false)
	}

	reservationID := ""
	acceptanceSource := models.AcceptanceDispatcher
	if assignment.OfferID != nil {
		acceptance, acceptanceErr := s.opts.Offers.GetAcceptance(ctx, operation.OrganizationID, *assignment.OfferID)
		if acceptanceErr != nil {
			return models.ExecutionStartRejected, s.opts.ExecutionStarts.Fail(ctx, nil,
				operation.ID, "organization_acceptance_not_found", 0, config.MaxAttempts, true)
		}
		reservationID = acceptance.SourceReservationID
		acceptanceSource = acceptance.AcceptanceSource
	}

	// Commit point: move the assignment to EXECUTION_START_PENDING BEFORE the
	// external call. Fleet cancellation refuses this state, which is what stops
	// a cancel from interleaving with in-flight trip creation.
	if assignment.Status == models.AssignmentAccepted {
		if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Assignments.Transition(ctx, tx, assignment,
				models.AssignmentExecutionStartPending, assignment.Version,
				models.StateHistoryEntry{ActorType: "SYSTEM", ActorID: config.Owner})
		}); err != nil {
			if errors.Is(err, repositories.ErrInvalidTransition) {
				return models.ExecutionStartRejected, s.opts.ExecutionStarts.Fail(ctx, nil,
					operation.ID, "assignment_not_executable", 0, config.MaxAttempts, true)
			}
			// A version conflict means someone else moved it; retry and re-read.
			return models.ExecutionStartRetryScheduled, s.opts.ExecutionStarts.Fail(ctx, nil,
				operation.ID, "expected_version_mismatch",
				config.backoffFor(operation.AttemptCount), config.MaxAttempts, false)
		}
	}
	if err := s.opts.ExecutionStarts.SetStatus(ctx, nil, operation.ID, models.ExecutionStartInFlight); err != nil {
		return "", err
	}
	if config.WritesEnabled != nil && !config.WritesEnabled() {
		return models.ExecutionStartRetryScheduled, s.opts.ExecutionStarts.Fail(ctx, nil, operation.ID, "integration_kill_switch", config.backoffFor(operation.AttemptCount), config.MaxAttempts, false)
	}

	// ---- No transaction is open from here until the outcome is known. ----
	contract, ok := s.opts.Executions.For(projection.SourceService)
	if !ok || !contract.Configured() {
		return models.ExecutionStartRetryScheduled, s.opts.ExecutionStarts.Fail(ctx, nil,
			operation.ID, "source_service_unavailable",
			config.backoffFor(operation.AttemptCount), config.MaxAttempts, false)
	}

	meta := platformclients.CallMeta{CorrelationID: operation.ID, IdempotencyKey: operation.IdempotencyKey}
	request := platformclients.ConfirmProviderRequest{
		SourceRequestID:          projection.PlatformRequestID,
		ReservationID:            reservationID,
		OrganizationID:           operation.OrganizationID,
		SettlementOrganizationID: operation.OrganizationID,
		ExecutingProviderID:      assignment.ExecutingProviderID,
		OrganizationMembershipID: assignment.MembershipID,
		FleetAssignmentID:        assignment.ID,
		ProviderResponseID:       response.ID,
		AcceptanceSource:         acceptanceSource,
		IdempotencyKey:           operation.IdempotencyKey,
		CorrelationID:            operation.ID,
	}

	var outcome platformclients.ExecutionOutcome
	if operation.NeedsReadBack() {
		outcome, err = contract.ReadExecutionOutcome(ctx, meta, projection.PlatformRequestID,
			operation.IdempotencyKey)
	} else {
		outcome, err = contract.ConfirmProviderAndCreateExecution(ctx, meta, request)
	}
	if err != nil {
		// Ambiguous: resolve by read-back, never by a fresh command.
		return models.ExecutionStartUnknown, s.opts.ExecutionStarts.MarkUnknown(ctx, nil,
			operation.ID, "execution_outcome_unknown", config.backoffFor(operation.AttemptCount))
	}

	switch outcome.Status {
	case platformclients.ExecutionCreatedStatus, platformclients.ExecutionAlreadyExists:
		// ALREADY_EXISTS is a success: it is exactly what a read-back after a
		// lost response returns, and it names the same canonical trip.
		return models.ExecutionStartConfirmed, s.confirmExecutionStart(ctx, operation, assignment, outcome)
	case platformclients.ExecutionRejectedStatus, platformclients.ExecutionNotReserved:
		return models.ExecutionStartRejected, s.opts.ExecutionStarts.Fail(ctx, nil, operation.ID,
			outcome.RejectionCode, 0, config.MaxAttempts, true)
	default:
		return models.ExecutionStartUnknown, s.opts.ExecutionStarts.MarkUnknown(ctx, nil,
			operation.ID, "execution_outcome_unknown", config.backoffFor(operation.AttemptCount))
	}
}

// confirmExecutionStart applies a confirmed canonical execution.
func (s *Service) confirmExecutionStart(ctx context.Context, operation models.ExecutionStartOperation,
	assignment *models.Assignment, outcome platformclients.ExecutionOutcome) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		current, err := s.opts.Assignments.LockByID(ctx, tx, assignment.ID)
		if err != nil {
			return err
		}
		if current.Status == models.AssignmentExecutionStartPending {
			if err := s.opts.Assignments.Transition(ctx, tx, current,
				models.AssignmentReadyForExecution, current.Version,
				models.StateHistoryEntry{ActorType: "SYSTEM", ActorID: "execution-start"}); err != nil {
				return err
			}
		}
		if err := s.opts.Assignments.SetExecutionStatus(ctx, tx, assignment.ID,
			models.ExecutionConfirmed, outcome.TripID); err != nil {
			return err
		}
		if err := s.opts.ExecutionStarts.Confirm(ctx, tx, operation.ID, outcome.TripID, outcome); err != nil {
			return err
		}

		entry := audit.Entry{
			OrganizationID: operation.OrganizationID,
			Action:         "assignment.execution_started",
			ResourceType:   "assignment",
			ResourceID:     assignment.ID,
			ActorType:      "SYSTEM",
			ActorID:        "execution-start",
		}
		entry.NewValues = map[string]any{"tripId": outcome.TripID}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.assignment.execution_started.v1",
			"assignment", assignment.ID, current.Version+1, operation.OrganizationID, "",
			map[string]any{
				"assignmentId":  assignment.ID,
				"tripId":        outcome.TripID,
				"sourceService": operation.SourceService,
			})
	})
}
