package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Retention, legal hold and deletion.
//
// Two rules run through this file, and both exist because deletion is the one
// operation that cannot be undone:
//
//  1. Fleet cannot mark a document physically deleted without media saying so.
//     A request that timed out is UNKNOWN, and UNKNOWN is resolved by reading
//     back from media, never by assuming.
//  2. A legal hold or an unexpired retention window blocks physical deletion
//     on both sides. Fleet refuses to schedule it and media refuses to perform
//     it, so neither a fleet bug nor a direct media call is sufficient on its
//     own to destroy evidence somebody is required to keep.
//
// The audit trail deliberately survives the file: evidence that a document
// existed and was destroyed is exactly what a later investigation needs.

// ScheduleDeletionInput asks for a document's bytes to be destroyed.
type ScheduleDeletionInput struct {
	Actor           Actor
	DocumentID      string
	ExpectedVersion int64
	Reason          string
	// ScheduledFor delays the deletion. Zero means as soon as the worker runs.
	ScheduledFor   time.Time
	IdempotencyKey string
	CorrelationID  string
	CausationID    string
	RequestID      string
}

// ScheduleDeletion records a durable deletion request.
//
// The retention decision is taken here, at scheduling time, and again by media
// at execution time. Checking twice is deliberate: fleet's copy of the
// retention policy can be stale, and media's is the one that governs the bytes.
func (s *Service) ScheduleDeletion(ctx context.Context, input ScheduleDeletionInput) (*models.DeletionRequest, error) {
	if err := s.writesAllowed(); err != nil {
		return nil, err
	}
	if input.Actor.OrganizationID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized, "Organization scope is required.")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed, "An idempotency key is required.")
	}

	fingerprint := deletionFingerprint(input)
	existing, err := s.opts.Documents.DeletionRequestByIdempotencyKey(ctx, nil,
		input.Actor.OrganizationID, input.IdempotencyKey)
	switch {
	case err == nil:
		if existing.RequestFingerprint != fingerprint {
			return nil, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
				"This idempotency key was already used for a different request.")
		}
		return existing, nil
	case !errors.Is(err, repositories.ErrNotFound):
		return nil, err
	}

	document, err := s.Get(ctx, input.Actor.OrganizationID, input.DocumentID)
	if err != nil {
		return nil, err
	}
	if input.ExpectedVersion > 0 && document.Version != input.ExpectedVersion {
		return nil, errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
			"This document changed since you loaded it. Reload and try again.")
	}
	if document.LegalHold {
		return nil, errorcodes.Conflict(errorcodes.DocumentLegalHoldActive,
			"This document is under a legal hold and cannot be deleted.")
	}
	now := s.opts.Now().UTC()
	if document.RetentionUntil != nil && document.RetentionUntil.After(now) {
		return nil, errorcodes.Conflict(errorcodes.DocumentRetentionActive,
			"This document is inside its retention period and cannot be deleted yet.")
	}

	scheduledFor := input.ScheduledFor
	if scheduledFor.IsZero() {
		scheduledFor = now
	}

	request := &models.DeletionRequest{
		OrganizationID:     document.OrganizationID,
		DocumentID:         document.ID,
		MediaObjectID:      document.MediaObjectID,
		Reason:             input.Reason,
		Status:             models.DeletionScheduled,
		ScheduledFor:       scheduledFor,
		RequestedBy:        input.Actor.ActorID,
		IdempotencyKey:     input.IdempotencyKey,
		RequestFingerprint: fingerprint,
		CorrelationID:      input.CorrelationID,
		CausationID:        input.CausationID,
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if createErr := s.opts.Documents.CreateDeletionRequest(ctx, tx, request); createErr != nil {
			if errors.Is(createErr, repositories.ErrConflict) {
				return errorcodes.Conflict(errorcodes.DocumentConflict,
					"Deletion has already been requested for this document.")
			}
			return createErr
		}
		if histErr := s.opts.Documents.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: document.OrganizationID,
			DocumentID:     document.ID,
			Action:         models.ActionDeletionScheduled,
			ToState:        document.LifecycleState,
			ActorType:      input.Actor.ActorType,
			ActorID:        input.Actor.ActorID,
			Metadata:       map[string]any{"scheduledFor": scheduledFor.Format(time.RFC3339)},
			CorrelationID:  input.CorrelationID,
			CausationID:    input.CausationID,
		}); histErr != nil {
			return histErr
		}
		return s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: document.OrganizationID,
			ActorType:      strings.ToLower(input.Actor.ActorType),
			ActorID:        input.Actor.ActorID,
			Action:         "document.deletion.scheduled",
			ResourceType:   "document",
			ResourceID:     document.ID,
			NewValues:      map[string]any{"scheduledFor": scheduledFor.Format(time.RFC3339)},
			Reason:         input.Reason,
			RequestID:      input.RequestID,
			CorrelationID:  input.CorrelationID,
		})
	})
	if err != nil {
		return nil, err
	}
	return request, nil
}

// LatestDeletion returns the newest deletion request for a document.
func (s *Service) LatestDeletion(ctx context.Context, organizationID string,
	documentID string) (*models.DeletionRequest, error) {
	if _, err := s.Get(ctx, organizationID, documentID); err != nil {
		return nil, err
	}
	request, err := s.opts.Documents.LatestDeletionRequest(ctx, nil, organizationID, documentID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, errorcodes.NotFound(errorcodes.DocumentNotFound, "No deletion request found.")
	}
	return request, err
}

// ReplayDeletion returns a dead-lettered deletion to the queue for an operator.
func (s *Service) ReplayDeletion(ctx context.Context, organizationID string, requestID string) error {
	err := s.opts.Documents.ReplayDeletionRequest(ctx, nil, organizationID, requestID)
	if errors.Is(err, repositories.ErrNotFound) {
		return errorcodes.NotFound(errorcodes.DocumentNotFound,
			"No dead-lettered deletion request found.")
	}
	return err
}

// RunDeletionPass executes one batch of due deletion requests.
func (s *Service) RunDeletionPass(ctx context.Context, owner string, lease time.Duration,
	batch int, backoff time.Duration, maxAttempts int) (int, error) {
	if s.opts.Flags.KillSwitch {
		return 0, nil
	}
	if err := s.mediaReady(); err != nil {
		return 0, err
	}
	if _, err := s.opts.Documents.RecoverExpiredDeletionLeases(ctx, s.opts.Pool); err != nil {
		return 0, err
	}

	requests, err := s.opts.Documents.ClaimDeletionRequests(ctx, s.opts.Pool, owner, lease, batch)
	if err != nil {
		return 0, err
	}
	for index := range requests {
		s.executeDeletion(ctx, &requests[index], backoff, maxAttempts)
	}
	return len(requests), nil
}

// executeDeletion asks media to destroy the bytes and records the outcome.
//
// Every branch here is about not lying to the rest of the platform:
//   - media says COMPLETED  -> the document is DELETED;
//   - media says BLOCKED    -> the document stays as it is, with the reason;
//   - the call times out    -> UNKNOWN, and the next pass reads back;
//   - the call fails        -> retry with backoff, dead-letter eventually.
//
// There is no branch that marks a document deleted on the strength of a
// request fleet never saw answered.
func (s *Service) executeDeletion(ctx context.Context, request *models.DeletionRequest,
	backoff time.Duration, maxAttempts int) {
	meta := platformclients.CallMeta{
		OrganizationID: request.OrganizationID,
		CorrelationID:  request.CorrelationID,
		CausationID:    request.CausationID,
		IdempotencyKey: request.IdempotencyKey,
	}

	outcome, err := s.opts.Media.RequestDeletion(ctx, meta, request.MediaObjectID, request.Reason)
	if err != nil {
		var clientErr *platformclients.Error
		if errors.As(err, &clientErr) && clientErr.Outcome == platformclients.OutcomeUnknown {
			// The outcome is genuinely unknown. Record it as such and let the
			// next pass establish the truth by reading back.
			s.settleDeletion(ctx, request, models.DeletionUnknown, "", "media_outcome_unknown",
				backoff, maxAttempts)
			return
		}
		if errors.As(err, &clientErr) && clientErr.Outcome == platformclients.OutcomeNotFound {
			// Media does not know the object. It cannot be reachable, so the
			// document is deleted as far as anybody can act on it.
			s.completeDeletion(ctx, request)
			return
		}
		s.settleDeletion(ctx, request, models.DeletionFailed, "", "media_call_failed",
			backoff, maxAttempts)
		return
	}

	switch outcome.Status {
	case "COMPLETED":
		s.completeDeletion(ctx, request)
	case "BLOCKED":
		// Media refused: a legal hold or retention window it holds and fleet
		// does not. The document stays exactly as it is.
		s.settleDeletion(ctx, request, models.DeletionBlocked, outcome.BlockReason, "",
			backoff, maxAttempts)
	case "FAILED":
		s.settleDeletion(ctx, request, models.DeletionFailed, "", "media_deletion_failed",
			backoff, maxAttempts)
	default:
		// PENDING or anything unrecognised: read back next pass rather than
		// guessing what it meant.
		s.settleDeletion(ctx, request, models.DeletionUnknown, "", "media_outcome_pending",
			backoff, maxAttempts)
	}
}

// completeDeletion records the authoritative deletion on the fleet side.
func (s *Service) completeDeletion(ctx context.Context, request *models.DeletionRequest) {
	now := s.opts.Now().UTC()
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		document, lockErr := s.opts.Documents.LockByID(ctx, tx, request.OrganizationID, request.DocumentID)
		if lockErr != nil {
			return lockErr
		}
		fromState := document.LifecycleState
		currentVersion := document.Version

		if voidErr := s.InvalidateVerification(ctx, tx, document, "DOCUMENT_DELETED",
			request.CorrelationID); voidErr != nil {
			return voidErr
		}
		document.LifecycleState = models.StateDeleted
		document.MediaDeletionState = models.MediaDeletionDeleted
		document.DeletedAt = &now
		if updateErr := s.opts.Documents.Update(ctx, tx, document, currentVersion); updateErr != nil {
			return updateErr
		}
		if statusErr := s.opts.Documents.SetDeletionStatus(ctx, tx, request.ID,
			models.DeletionCompleted, "", "", 0); statusErr != nil {
			return statusErr
		}
		if histErr := s.opts.Documents.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: document.OrganizationID,
			DocumentID:     document.ID,
			Action:         models.ActionDeletionCompleted,
			FromState:      fromState,
			ToState:        document.LifecycleState,
			ActorType:      models.ActorSystem,
			ActorID:        "fleet-partner-service",
			CorrelationID:  request.CorrelationID,
		}); histErr != nil {
			return histErr
		}
		// The audit row deliberately outlives the file.
		if auditErr := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: document.OrganizationID,
			ActorType:      "system",
			ActorID:        "fleet-partner-service",
			Action:         "document.deletion.completed",
			ResourceType:   "document",
			ResourceID:     document.ID,
			PreviousValues: map[string]any{"lifecycleState": fromState},
			NewValues: map[string]any{
				"lifecycleState": document.LifecycleState,
				"mediaObjectId":  document.MediaObjectID,
			},
			CorrelationID: request.CorrelationID,
		}); auditErr != nil {
			return auditErr
		}
		if invalidateErr := s.invalidateDecisions(ctx, tx, document,
			models.ReasonDocumentDeleted, request.CorrelationID); invalidateErr != nil {
			return invalidateErr
		}
		return s.publish(ctx, tx, EventDocumentDeleted, document, request.CorrelationID, nil)
	})
	if err != nil {
		s.opts.Metrics.IncCounter("fleet_partner_document_worker_failures_total",
			map[string]string{"worker": "deletion"}, 1)
		return
	}
	s.opts.Metrics.IncCounter("fleet_partner_documents_deleted_total", nil, 1)
}

// settleDeletion records a non-terminal outcome and schedules the retry.
func (s *Service) settleDeletion(ctx context.Context, request *models.DeletionRequest,
	status string, blockReason string, errorCode string, backoff time.Duration, maxAttempts int) {
	finalStatus := status
	if status != models.DeletionBlocked && request.Attempts >= maxAttempts {
		// A permanently failing request stops consuming the batch. It stays
		// visible and an operator can replay it.
		finalStatus = models.DeletionDeadLettered
	}

	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if statusErr := s.opts.Documents.SetDeletionStatus(ctx, tx, request.ID,
			finalStatus, blockReason, errorCode, backoff); statusErr != nil {
			return statusErr
		}

		document, lockErr := s.opts.Documents.LockByID(ctx, tx, request.OrganizationID, request.DocumentID)
		if lockErr != nil {
			return lockErr
		}
		action := models.ActionDeletionFailed
		switch status {
		case models.DeletionBlocked:
			action = models.ActionDeletionBlocked
		case models.DeletionUnknown:
			action = models.ActionDeletionUnknown
			// An unknown deletion outcome is exactly the condition
			// reconciliation exists for: the projection and the authority may
			// now disagree, and only a read-back settles it.
			currentVersion := document.Version
			document.MediaDeletionState = models.MediaDeletionUnknown
			document.ProjectionFreshness = models.FreshnessReconciling
			document.LifecycleState = models.StateReconciling
			if updateErr := s.opts.Documents.Update(ctx, tx, document, currentVersion); updateErr != nil {
				return updateErr
			}
			if caseErr := s.opts.Documents.OpenReconciliationCase(ctx, tx, &models.ReconciliationCase{
				OrganizationID: document.OrganizationID,
				DocumentID:     document.ID,
				MediaObjectID:  document.MediaObjectID,
				Reason:         models.ReconcileDeletionUnknown,
				CorrelationID:  request.CorrelationID,
			}); caseErr != nil {
				return caseErr
			}
		}

		return s.opts.Documents.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: document.OrganizationID,
			DocumentID:     document.ID,
			Action:         action,
			ToState:        document.LifecycleState,
			ActorType:      models.ActorSystem,
			ActorID:        "fleet-partner-service",
			Metadata: map[string]any{
				"status":      finalStatus,
				"blockReason": blockReason,
				"errorCode":   errorCode,
			},
			CorrelationID: request.CorrelationID,
		})
	})
	if err != nil {
		s.opts.Metrics.IncCounter("fleet_partner_document_worker_failures_total",
			map[string]string{"worker": "deletion"}, 1)
	}
}

// RunDeletionWorker sweeps deletion requests until ctx ends.
func (s *Service) RunDeletionWorker(ctx context.Context, owner string, interval time.Duration,
	lease time.Duration, batch int, backoff time.Duration, maxAttempts int) error {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.RunDeletionPass(ctx, owner, lease, batch, backoff, maxAttempts); err != nil {
			s.opts.Metrics.IncCounter("fleet_partner_document_worker_failures_total",
				map[string]string{"worker": "deletion"}, 1)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func deletionFingerprint(input ScheduleDeletionInput) string {
	return platformclients.MediaFingerprint(input.Actor.OrganizationID, input.DocumentID,
		input.Reason, input.ScheduledFor.UTC().Format(time.RFC3339))
}
