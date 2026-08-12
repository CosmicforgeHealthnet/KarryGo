package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// versionToken renders an expected version for inclusion in a fingerprint.
func versionToken(version int64) string {
	return strconv.FormatInt(version, 10)
}

// fingerprint produces a stable hash of a command's meaningful inputs so the
// same idempotency key cannot be reused for a different request.
func fingerprint(parts ...string) string {
	hasher := sha256.New()
	for _, part := range parts {
		_, _ = hasher.Write([]byte(part))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

// requestExecution asks the owning service to create canonical execution.
//
// The provider's acceptance is already durably committed before this runs, so
// every failure mode below preserves it. The three outcomes are:
//   - confirmed  -> assignment becomes READY_FOR_EXECUTION;
//   - rejected   -> recorded, assignment stays ACCEPTED for operator review;
//   - uncertain  -> RECONCILIATION_REQUIRED, resolved later by read-back.
//
// It returns the execution-start status to surface to the provider.
func (s *Service) requestExecution(ctx context.Context, assignment *models.Assignment, correlationID string) string {
	// Organization policy may require the assigned party to confirm receipt
	// before the work leaves. The acceptance is already durable; withholding
	// the departure is the whole of the policy's effect, and acknowledging
	// later resumes it through resumeBlockedDeparture.
	if s.departureBlocked(ctx, assignment) {
		_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Assignments.SetExecutionStatus(ctx, tx, assignment.ID,
				models.ExecutionNotRequested, "")
		})
		s.opts.Metrics.IncCounter("fleet_partner_departure_blocked_unacknowledged_total",
			map[string]string{"source": assignment.SourceService}, 1)
		return models.ExecutionNotRequested
	}

	if !s.opts.Flags.ExecutionStartEnabled || !s.opts.Flags.SourceEnabled(assignment.SourceService) {
		// Live execution for this service category is switched off. The
		// acceptance stands; nothing is fabricated downstream.
		_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Assignments.SetExecutionStatus(ctx, tx, assignment.ID,
				models.ExecutionNotRequested, "")
		})
		return models.ExecutionNotRequested
	}

	contract, ok := s.opts.Executions.For(assignment.SourceService)
	if !ok || !contract.Configured() {
		s.markExecutionReconciliation(ctx, assignment, "execution_contract_unavailable")
		return models.ExecutionReconciliationRequired
	}

	projection, err := s.opts.Projections.GetByID(ctx, assignment.OrganizationID, assignment.RequestProjectionID)
	if err != nil {
		s.markExecutionReconciliation(ctx, assignment, "projection_unavailable")
		return models.ExecutionReconciliationRequired
	}
	response, err := s.opts.Assignments.GetResponse(ctx, nil, assignment.ID)
	if err != nil {
		s.markExecutionReconciliation(ctx, assignment, "response_unavailable")
		return models.ExecutionReconciliationRequired
	}

	idempotencyKey := "execution:" + assignment.ID
	assetReference := ""
	if assignment.AssetReference != nil {
		assetReference = *assignment.AssetReference
	}
	command := &models.CommandOperation{
		OrganizationID:  assignment.OrganizationID,
		OperationType:   models.CommandCreateExecution,
		SourceService:   assignment.SourceService,
		SourceRequestID: projection.PlatformRequestID,
		AssignmentID:    &assignment.ID,
		IdempotencyKey:  idempotencyKey,
		RequestFingerprint: fingerprint(projection.PlatformRequestID, assignment.ID,
			assignment.ExecutingProviderID, assetReference),
	}

	var operation *models.CommandOperation
	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		stored, _, err := s.opts.Commands.Begin(ctx, tx, command)
		if err != nil && !errors.Is(err, repositories.ErrIdempotencyConflict) {
			return err
		}
		operation = stored
		return s.opts.Assignments.SetExecutionStatus(ctx, tx, assignment.ID, models.ExecutionSubmitted, "")
	}); err != nil {
		s.markExecutionReconciliation(ctx, assignment, "command_record_failed")
		return models.ExecutionReconciliationRequired
	}

	// A previously confirmed command is not re-sent: return the known result.
	if operation != nil && operation.Status == models.CommandConfirmed {
		s.confirmExecution(ctx, assignment, operation.ExternalReferenceOrEmpty())
		return models.ExecutionConfirmed
	}

	outcome, err := contract.ConfirmProviderAndCreateExecution(ctx,
		platformclients.CallMeta{CorrelationID: correlationID},
		platformclients.ConfirmProviderRequest{
			SourceRequestID:          projection.PlatformRequestID,
			ExpectedSourceVersion:    projection.SourceVersion,
			OrganizationID:           assignment.OrganizationID,
			SettlementOrganizationID: assignment.OrganizationID,
			ExecutingProviderID:      assignment.ExecutingProviderID,
			OrganizationMembershipID: assignment.MembershipID,
			AssetOrVehicleReference:  assetReference,
			FleetAssignmentID:        assignment.ID,
			ProviderResponseID:       response.ID,
			AcceptanceSource:         models.AcceptanceDispatcher,
			IdempotencyKey:           idempotencyKey,
			CorrelationID:            correlationID,
		})

	if err != nil {
		clientErr, ok := err.(*platformclients.Error)
		// An unknown outcome means the trip may or may not have been created.
		// It is never retried blindly — the reconciler reads back with the
		// same idempotency key.
		if ok && clientErr.Outcome == platformclients.OutcomeUnknown {
			s.recordCommand(ctx, operation, models.CommandUnknownOutcome, "", "execution_unknown_outcome")
			s.markExecutionReconciliation(ctx, assignment, "execution_unknown_outcome")
			s.opts.Metrics.IncCounter("fleet_partner_source_unknown_outcomes_total",
				map[string]string{"operation": models.CommandCreateExecution}, 1)
			return models.ExecutionReconciliationRequired
		}
		s.recordCommand(ctx, operation, models.CommandUnknownOutcome, "", "execution_call_failed")
		s.markExecutionReconciliation(ctx, assignment, "execution_call_failed")
		s.opts.Metrics.IncCounter("fleet_partner_source_command_errors_total",
			map[string]string{"operation": models.CommandCreateExecution}, 1)
		return models.ExecutionReconciliationRequired
	}

	switch outcome.Status {
	case platformclients.ExecutionCreatedStatus, platformclients.ExecutionAlreadyExists:
		s.recordCommand(ctx, operation, models.CommandConfirmed, outcome.TripID, "")
		s.confirmExecution(ctx, assignment, outcome.TripID)
		return models.ExecutionConfirmed
	default:
		s.recordCommand(ctx, operation, models.CommandRejected, "", outcome.RejectionCode)
		_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Assignments.SetExecutionStatus(ctx, tx, assignment.ID, models.ExecutionRejected, "")
		})
		return models.ExecutionRejected
	}
}

// confirmExecution advances the assignment and projection once the source
// service has confirmed canonical execution exists.
func (s *Service) confirmExecution(ctx context.Context, assignment *models.Assignment, tripID string) {
	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		current, err := s.opts.Assignments.LockByID(ctx, tx, assignment.ID)
		if err != nil {
			return err
		}
		if err := s.opts.Assignments.SetExecutionStatus(ctx, tx, assignment.ID,
			models.ExecutionConfirmed, tripID); err != nil {
			return err
		}
		if current.Status == models.AssignmentAccepted || current.Status == models.AssignmentExecutionStartPending ||
			current.Status == models.AssignmentReconciliationRequired {
			if err := s.opts.Assignments.Transition(ctx, tx, current, models.AssignmentReadyForExecution,
				current.Version, models.StateHistoryEntry{
					ActorType: "SYSTEM", ActorID: "execution-start",
					Metadata: map[string]any{"tripId": tripID},
				}); err != nil {
				return err
			}
		}
		projection, err := s.opts.Projections.LockByID(ctx, tx, assignment.OrganizationID, assignment.RequestProjectionID)
		if err == nil {
			_ = s.opts.Projections.SetStatus(ctx, tx, assignment.OrganizationID, projection.ID,
				models.ProjectionExecutionCreated, projection.Version)
		}
		return s.opts.Events.Publish(ctx, tx, "organization.assignment.execution_requested.v1",
			"assignment", assignment.ID, current.Version+1, assignment.OrganizationID, "",
			map[string]any{
				"assignmentId": assignment.ID,
				"tripId":       tripID,
				"status":       models.ExecutionConfirmed,
			})
	})
}

func (s *Service) markExecutionReconciliation(ctx context.Context, assignment *models.Assignment, reason string) {
	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Assignments.SetExecutionStatus(ctx, tx, assignment.ID,
			models.ExecutionReconciliationRequired, ""); err != nil {
			return err
		}
		projection, err := s.opts.Projections.GetByID(ctx, assignment.OrganizationID, assignment.RequestProjectionID)
		if err != nil {
			return nil
		}
		return s.opts.Trips.OpenReconciliation(ctx, tx, assignment.OrganizationID,
			assignment.SourceService, "COMMAND", projection.PlatformRequestID, reason,
			s.opts.Now().Add(time.Minute))
	})
	s.opts.Metrics.IncCounter("fleet_partner_assignment_execution_reconciliation_total", nil, 1)
}

func (s *Service) recordCommand(ctx context.Context, operation *models.CommandOperation, status string, reference string, errorCode string) {
	if operation == nil {
		return
	}
	var next *time.Time
	if status == models.CommandUnknownOutcome {
		retryAt := s.opts.Now().Add(time.Minute)
		next = &retryAt
	}
	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Commands.SetStatus(ctx, tx, operation.ID, status, reference, errorCode, next)
	})
}

// ReconcileCommands resolves uncertain external writes by read-back.
//
// This is the safety net behind every timed-out reservation and execution
// call: it asks the source service what happened using the original
// idempotency key rather than repeating the write.
func (s *Service) ReconcileCommands(ctx context.Context, limit int) (int, error) {
	var operations []models.CommandOperation
	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		claimed, err := s.opts.Commands.ClaimDue(ctx, tx, limit)
		operations = claimed
		return err
	}); err != nil {
		return 0, err
	}

	resolved := 0
	for index := range operations {
		if s.reconcileCommand(ctx, &operations[index]) {
			resolved++
		}
	}
	if len(operations) > 0 {
		s.opts.Metrics.SetGauge("fleet_partner_source_reconciliation_backlog", nil, float64(len(operations)-resolved))
	}
	return resolved, nil
}

func (s *Service) reconcileCommand(ctx context.Context, operation *models.CommandOperation) bool {
	contract, ok := s.opts.Executions.For(operation.SourceService)
	if !ok || !contract.Configured() {
		// Nothing to ask yet; leave the case open for when the contract
		// exists rather than guessing an outcome.
		next := s.opts.Now().Add(10 * time.Minute)
		s.recordCommandAt(ctx, operation, models.CommandReconciliationRequired, "", "contract_unavailable", &next)
		return false
	}
	meta := platformclients.CallMeta{}

	switch operation.OperationType {
	case models.CommandReserveRequest:
		outcome, err := contract.ReadReservation(ctx, meta, operation.SourceRequestID, operation.IdempotencyKey)
		if err != nil {
			s.backoffCommand(ctx, operation, "reservation_readback_failed")
			return false
		}
		status := models.CommandRejected
		if outcome.Status == platformclients.ReservationReserved {
			status = models.CommandConfirmed
		}
		s.recordCommand(ctx, operation, status, outcome.ReservationID, outcome.Status)
		return true

	case models.CommandCreateExecution:
		outcome, err := contract.ReadExecutionOutcome(ctx, meta, operation.SourceRequestID, operation.IdempotencyKey)
		if err != nil {
			s.backoffCommand(ctx, operation, "execution_readback_failed")
			return false
		}
		if outcome.Status == platformclients.ExecutionCreatedStatus || outcome.Status == platformclients.ExecutionAlreadyExists {
			s.recordCommand(ctx, operation, models.CommandConfirmed, outcome.TripID, "")
			if operation.AssignmentID != nil {
				if assignment, err := s.opts.Assignments.GetByID(ctx, operation.OrganizationID, *operation.AssignmentID); err == nil {
					s.confirmExecution(ctx, assignment, outcome.TripID)
				}
			}
			return true
		}
		// The source service says no execution exists: it is safe to let the
		// operator re-drive, because read-back proved nothing was created.
		s.recordCommand(ctx, operation, models.CommandRejected, "", outcome.RejectionCode)
		return true

	default:
		s.backoffCommand(ctx, operation, "unsupported_operation")
		return false
	}
}

func (s *Service) backoffCommand(ctx context.Context, operation *models.CommandOperation, errorCode string) {
	delay := time.Duration(operation.Attempts+1) * time.Minute
	if delay > 30*time.Minute {
		delay = 30 * time.Minute
	}
	next := s.opts.Now().Add(delay)
	s.recordCommandAt(ctx, operation, models.CommandReconciliationRequired, "", errorCode, &next)
}

func (s *Service) recordCommandAt(ctx context.Context, operation *models.CommandOperation, status string, reference string, errorCode string, next *time.Time) {
	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Commands.SetStatus(ctx, tx, operation.ID, status, reference, errorCode, next)
	})
}
