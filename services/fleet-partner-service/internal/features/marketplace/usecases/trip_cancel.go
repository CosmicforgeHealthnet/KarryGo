package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
)

// CorporateCancelTripInput asks the source service to cancel a canonical
// trip the organization holds (FLEET-12.2).
type CorporateCancelTripInput struct {
	TripID         string
	Reason         string
	IdempotencyKey string
}

// CorporateCancelTripResult reports what the source service decided.
type CorporateCancelTripResult struct {
	TripID string `json:"tripId"`
	// Status is CANCELLED when the source confirmed, or
	// CANCELLATION_PENDING when the outcome is unknown and is being read back.
	Status string `json:"status"`
}

const (
	tripCancelConfirmed = "CANCELLED"
	tripCancelPending   = "CANCELLATION_PENDING"
)

func tripIsCancelled(status string, cancelledAt *time.Time) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return cancelledAt != nil || s == "cancelled" || s == "canceled"
}

func tripIsCompleted(status string, completedAt *time.Time) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return completedAt != nil || s == "completed"
}

// CancelCorporateTrip cancels a live canonical trip through the authoritative
// execution service. Fleet never marks a trip cancelled on its own: the
// projection moves when the source's trip event arrives, and this command is
// recorded durably so an unknown outcome is read back by the command
// reconciler rather than guessed. Another organization's trip, or one outside
// the caller's branches, is not found.
func (s *Service) CancelCorporateTrip(ctx context.Context, t tenant.Context, input CorporateCancelTripInput) (*CorporateCancelTripResult, error) {
	if s.opts.Trips == nil {
		return nil, apperrors.Unavailable("Trip projections are not available.", nil)
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" || len(reason) > 500 {
		return nil, apperrors.Validation("A cancellation reason of up to 500 characters is required.", nil)
	}
	trip, err := s.opts.Trips.GetScoped(ctx, tenant.Query{
		OrganizationID: t.OrganizationID, PermittedBranchIDs: t.PermittedBranchIDs,
	}, input.TripID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, apperrors.NotFound("Trip not found.", nil)
	}
	if err != nil {
		return nil, err
	}
	if tripIsCancelled(trip.TripStatus, trip.CancelledAt) {
		return &CorporateCancelTripResult{TripID: trip.ID, Status: tripCancelConfirmed}, nil
	}
	if tripIsCompleted(trip.TripStatus, trip.CompletedAt) {
		return nil, apperrors.Conflict("A completed trip cannot be cancelled.", nil)
	}
	contract, ok := s.opts.Executions.For(trip.SourceService)
	if !ok || !contract.Configured() {
		return nil, apperrors.Unavailable("The execution service for this trip is not available.", nil)
	}

	// One cancellation per trip: the key is the trip, so a second request
	// (another dispatcher, a retried click) re-uses the first command and its
	// known outcome instead of issuing a second remote call.
	key := "trip-cancel:" + trip.ID
	command := &models.CommandOperation{
		OrganizationID:     t.OrganizationID,
		OperationType:      models.CommandCancelExecution,
		SourceService:      trip.SourceService,
		SourceRequestID:    trip.PlatformTripID, // the source trip for a cancel command
		AssignmentID:       trip.AssignmentID,
		IdempotencyKey:     key,
		RequestFingerprint: fingerprint(trip.SourceService, trip.PlatformTripID, t.OrganizationID),
	}
	var operation *models.CommandOperation
	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		stored, _, err := s.opts.Commands.Begin(ctx, tx, command)
		if err != nil {
			return err
		}
		operation = stored
		entry := audit.EntryFromTenant(t, "trip.cancellation_requested", "trip_projection", trip.ID)
		entry.NewValues = map[string]any{"sourceService": trip.SourceService, "platformTripId": trip.PlatformTripID}
		entry.Reason = reason
		return s.opts.Audit.Record(ctx, tx, entry)
	}); err != nil {
		return nil, err
	}
	if operation.Status == models.CommandConfirmed {
		return &CorporateCancelTripResult{TripID: trip.ID, Status: tripCancelConfirmed}, nil
	}

	meta := platformclients.CallMeta{OrganizationID: t.OrganizationID, RequestID: t.RequestID,
		CorrelationID: t.CorrelationID, IdempotencyKey: key}
	err = contract.CancelExecution(ctx, meta, trip.PlatformTripID, reason, key)
	if err == nil {
		s.recordCommand(ctx, operation, models.CommandConfirmed, trip.PlatformTripID, "")
		return &CorporateCancelTripResult{TripID: trip.ID, Status: tripCancelConfirmed}, nil
	}
	var clientErr *platformclients.Error
	if errors.As(err, &clientErr) && clientErr.Outcome == platformclients.OutcomeUnknown {
		// The source may or may not have cancelled: the reconciler reads the
		// trip back and re-sends the same idempotent command if needed.
		s.recordCommand(ctx, operation, models.CommandUnknownOutcome, "", "cancel_unknown_outcome")
		return &CorporateCancelTripResult{TripID: trip.ID, Status: tripCancelPending}, nil
	}
	s.recordCommand(ctx, operation, models.CommandRejected, "", "cancel_refused")
	if errors.As(err, &clientErr) && clientErr.Status == 404 {
		return nil, apperrors.NotFound("Trip not found.", nil)
	}
	return nil, apperrors.Conflict("The execution service refused to cancel this trip in its current state.", nil)
}

// reconcileCancel resolves an uncertain cancellation by reading the trip from
// its source: cancelled is confirmed, completed is rejected, and a trip that
// is still live gets the same idempotent cancel re-sent.
func (s *Service) reconcileCancel(ctx context.Context, operation *models.CommandOperation, contract platformclients.ExecutionContract) bool {
	meta := platformclients.CallMeta{OrganizationID: operation.OrganizationID, IdempotencyKey: operation.IdempotencyKey}
	view, err := contract.ReadTrip(ctx, meta, operation.SourceRequestID)
	if err != nil {
		s.backoffCommand(ctx, operation, "cancel_readback_failed")
		return false
	}
	switch {
	case tripIsCancelled(view.Status, view.CancelledAt):
		s.recordCommand(ctx, operation, models.CommandConfirmed, view.TripID, "")
		return true
	case tripIsCompleted(view.Status, view.CompletedAt):
		s.recordCommand(ctx, operation, models.CommandRejected, "", "trip_completed")
		return true
	}
	if err := contract.CancelExecution(ctx, meta, operation.SourceRequestID, "reconciliation", operation.IdempotencyKey); err != nil {
		var clientErr *platformclients.Error
		if errors.As(err, &clientErr) && clientErr.Outcome == platformclients.OutcomePermanent {
			s.recordCommand(ctx, operation, models.CommandRejected, "", "cancel_refused")
			return true
		}
		s.backoffCommand(ctx, operation, "cancel_resend_failed")
		return false
	}
	s.recordCommand(ctx, operation, models.CommandConfirmed, operation.SourceRequestID, "")
	return true
}
