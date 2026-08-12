package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Reconciliation.
//
// A reconciliation case is opened whenever fleet's projection and media's
// authority might disagree: a version gap in the event stream, an outcome that
// timed out, a projection nobody has confirmed for too long. While a case is
// open the document authorizes nothing — not eligibility, not download —
// because the honest answer is that nobody currently knows the file's state.
//
// The repair is always the same and always one-directional: ask media, believe
// the answer, write it down. Fleet never reconciles by deciding what the state
// probably was.

// ReconciliationConfig tunes the worker.
type ReconciliationConfig struct {
	Owner       string
	BatchSize   int
	Lease       time.Duration
	Backoff     time.Duration
	MaxAttempts int
	// StaleAfter is how long a CURRENT projection may go unconfirmed before a
	// sweep marks it STALE and opens a case.
	StaleAfter time.Duration
	SweepLimit int
}

func (c ReconciliationConfig) normalized() ReconciliationConfig {
	if c.Owner == "" {
		c.Owner = "fleet-partner-service"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 25
	}
	if c.Lease <= 0 {
		c.Lease = 2 * time.Minute
	}
	if c.Backoff <= 0 {
		c.Backoff = 30 * time.Second
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	if c.StaleAfter <= 0 {
		c.StaleAfter = 15 * time.Minute
	}
	if c.SweepLimit <= 0 {
		c.SweepLimit = 200
	}
	return c
}

// RunReconciliationPass performs one full reconciliation cycle: recover
// abandoned leases, mark stale projections, then repair claimed cases.
func (s *Service) RunReconciliationPass(ctx context.Context, config ReconciliationConfig) (int, error) {
	if s.opts.Flags.KillSwitch {
		return 0, nil
	}
	config = config.normalized()

	// A worker that died holding leases must not strand its cases.
	if _, err := s.opts.Documents.RecoverExpiredCaseLeases(ctx, s.opts.Pool); err != nil {
		return 0, err
	}
	if err := s.markStaleProjections(ctx, config); err != nil {
		return 0, err
	}
	if err := s.mediaReady(); err != nil {
		// Without a configured authority there is nothing to read back from.
		// The cases stay open and the documents stay blocked, which is the
		// correct degraded posture.
		return 0, err
	}

	cases, err := s.opts.Documents.ClaimReconciliationCases(ctx, s.opts.Pool,
		config.Owner, config.Lease, config.BatchSize)
	if err != nil {
		return 0, err
	}
	for index := range cases {
		s.repairCase(ctx, &cases[index], config)
	}
	return len(cases), nil
}

// markStaleProjections downgrades projections nobody has confirmed recently.
//
// This is the sweep that catches silence. Without it a document whose media
// events simply stopped arriving would stay CURRENT forever and keep
// authorizing work on the strength of an answer that might be months old.
func (s *Service) markStaleProjections(ctx context.Context, config ReconciliationConfig) error {
	documents, err := s.opts.Documents.ClaimStaleProjections(ctx, s.opts.Pool,
		config.StaleAfter, config.SweepLimit)
	if err != nil {
		return err
	}
	for index := range documents {
		document := documents[index]
		err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			locked, lockErr := s.opts.Documents.LockByID(ctx, tx, document.OrganizationID, document.ID)
			if lockErr != nil {
				return lockErr
			}
			if locked.ProjectionFreshness != models.FreshnessCurrent {
				return nil
			}
			fromState := locked.LifecycleState
			currentVersion := locked.Version
			locked.ProjectionFreshness = models.FreshnessStale
			locked.LifecycleState = models.LifecycleForMedia(locked.MediaUploadStatus,
				locked.SafetyProjection, locked.MediaDeletionState, models.FreshnessStale,
				locked.VerificationState == models.VerificationVerified)
			if updateErr := s.opts.Documents.Update(ctx, tx, locked, currentVersion); updateErr != nil {
				return updateErr
			}
			if caseErr := s.opts.Documents.OpenReconciliationCase(ctx, tx, &models.ReconciliationCase{
				OrganizationID: locked.OrganizationID,
				DocumentID:     locked.ID,
				MediaObjectID:  locked.MediaObjectID,
				Reason:         models.ReconcileStaleProjection,
			}); caseErr != nil {
				return caseErr
			}
			if histErr := s.opts.Documents.AppendHistory(ctx, tx, &models.HistoryEntry{
				OrganizationID: locked.OrganizationID,
				DocumentID:     locked.ID,
				Action:         models.ActionStaleDetected,
				FromState:      fromState,
				ToState:        locked.LifecycleState,
				ActorType:      models.ActorSystem,
				ActorID:        "fleet-partner-service",
			}); histErr != nil {
				return histErr
			}
			// A stale projection cannot keep authorizing work.
			return s.invalidateDecisions(ctx, tx, locked, models.ReasonDocumentProjectionStale, "")
		})
		if err != nil {
			s.opts.Metrics.IncCounter("fleet_partner_document_worker_failures_total",
				map[string]string{"worker": "stale_sweep"}, 1)
		}
	}
	return nil
}

// repairCase reads media back and writes down whatever it says.
func (s *Service) repairCase(ctx context.Context, record *models.ReconciliationCase,
	config ReconciliationConfig) {
	meta := platformclients.CallMeta{
		OrganizationID: record.OrganizationID,
		CorrelationID:  record.CorrelationID,
	}

	state, err := s.opts.Media.ReadObject(ctx, meta, record.MediaObjectID)
	if err != nil {
		if errors.Is(err, platformclients.ErrMediaObjectNotFound) {
			// Media does not know the object. That IS the authoritative
			// answer: the file is gone, and the document says so.
			s.applyReadBack(ctx, record, platformclients.MediaObjectState{
				MediaObjectID: record.MediaObjectID,
				UploadStatus:  models.MediaUploadUnknown,
				SafetyStatus:  models.SafetyUnknown,
				DeletionState: models.MediaDeletionDeleted,
			}, config)
			return
		}
		_ = s.opts.Documents.FailReconciliationCase(ctx, s.opts.Pool, record.ID,
			"media_read_failed", config.Backoff, config.MaxAttempts)
		s.opts.Metrics.IncCounter("fleet_partner_document_reconciliation_failures_total", nil, 1)
		return
	}
	s.applyReadBack(ctx, record, state, config)
}

// applyReadBack writes the authoritative answer onto the document and closes
// the case.
//
// The version is taken forward-only: an authority answer older than what fleet
// already recorded is not applied, because reading back must never move the
// projection backwards.
func (s *Service) applyReadBack(ctx context.Context, record *models.ReconciliationCase,
	state platformclients.MediaObjectState, config ReconciliationConfig) {
	now := s.opts.Now().UTC()

	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		document, lockErr := s.opts.Documents.LockByID(ctx, tx, record.OrganizationID, record.DocumentID)
		if errors.Is(lockErr, repositories.ErrNotFound) {
			// The document is gone; nothing left to reconcile.
			return s.opts.Documents.ResolveReconciliationCase(ctx, tx, record.ID)
		}
		if lockErr != nil {
			return lockErr
		}

		fromState := document.LifecycleState
		currentVersion := document.Version
		previousSafety := document.SafetyProjection

		if state.UploadStatus != "" {
			document.MediaUploadStatus = state.UploadStatus
		}
		if state.SafetyStatus != "" {
			document.SafetyProjection = state.SafetyStatus
		}
		if state.DeletionState != "" {
			document.MediaDeletionState = state.DeletionState
		}
		if state.SourceVersion > document.MediaSourceVersion {
			document.MediaSourceVersion = state.SourceVersion
		}
		if state.IntegrityHash != "" {
			document.MediaChecksumSHA256 = state.IntegrityHash
		} else if state.ChecksumSHA256 != "" {
			document.MediaChecksumSHA256 = state.ChecksumSHA256
		}
		if state.OriginalMIMEType != "" {
			document.OriginalMimeType = state.OriginalMIMEType
		}
		if state.OriginalSizeBytes > 0 {
			document.OriginalSizeBytes = state.OriginalSizeBytes
		}
		if state.CaptureMethod != "" {
			document.CaptureMethod = state.CaptureMethod
		}
		if state.DeviceReference != "" {
			document.DeviceReference = state.DeviceReference
		}
		if state.ActorReference != "" {
			document.ActorReference = state.ActorReference
		}
		if state.CapturedAt != "" {
			if value, parseErr := time.Parse(time.RFC3339, state.CapturedAt); parseErr == nil {
				document.CapturedAt = &value
			}
		}
		if state.SourceTimestamp != "" {
			if value, parseErr := time.Parse(time.RFC3339, state.SourceTimestamp); parseErr == nil {
				document.SourceTimestamp = &value
			}
		}
		document.ProjectionFreshness = models.FreshnessCurrent
		document.SyncedAt = &now

		if document.SafetyProjection != models.SafetySafe && previousSafety == models.SafetySafe {
			if voidErr := s.InvalidateVerification(ctx, tx, document,
				models.RejectUnsafeContent, record.CorrelationID); voidErr != nil {
				return voidErr
			}
		}
		if document.MediaDeletionState == models.MediaDeletionDeleted {
			if voidErr := s.InvalidateVerification(ctx, tx, document,
				"DOCUMENT_DELETED", record.CorrelationID); voidErr != nil {
				return voidErr
			}
			if document.DeletedAt == nil {
				document.DeletedAt = &now
			}
		}

		document.LifecycleState = models.LifecycleForMedia(document.MediaUploadStatus,
			document.SafetyProjection, document.MediaDeletionState, models.FreshnessCurrent,
			document.VerificationState == models.VerificationVerified)

		if updateErr := s.opts.Documents.Update(ctx, tx, document, currentVersion); updateErr != nil {
			return updateErr
		}
		// The forward-only version marker is advanced too, so the next event
		// after a repaired gap is not itself treated as a gap.
		if state.SourceVersion > 0 {
			if versionErr := s.opts.Documents.RecordSourceVersion(ctx, tx, repositories.SourceVersion{
				MediaObjectID: document.MediaObjectID,
				Version:       document.MediaSourceVersion,
				LastEventID:   "reconciliation:" + record.ID,
				PayloadHash:   "",
			}); versionErr != nil {
				return versionErr
			}
		}
		if histErr := s.opts.Documents.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: document.OrganizationID,
			DocumentID:     document.ID,
			Action:         models.ActionReconciled,
			FromState:      fromState,
			ToState:        document.LifecycleState,
			ActorType:      models.ActorSystem,
			ActorID:        "fleet-partner-service",
			Metadata: map[string]any{
				"reason":        record.Reason,
				"sourceVersion": document.MediaSourceVersion,
			},
			CorrelationID: record.CorrelationID,
		}); histErr != nil {
			return histErr
		}
		if document.SafetyProjection != models.SafetySafe ||
			document.MediaDeletionState != models.MediaDeletionActive {
			reason := models.ReasonDocumentNotSafe
			if document.MediaDeletionState == models.MediaDeletionDeleted {
				reason = models.ReasonDocumentDeleted
			}
			if invalidateErr := s.invalidateDecisions(ctx, tx, document, reason,
				record.CorrelationID); invalidateErr != nil {
				return invalidateErr
			}
		}
		if publishErr := s.publish(ctx, tx, EventDocumentStateChanged, document,
			record.CorrelationID, map[string]any{"reconciled": true}); publishErr != nil {
			return publishErr
		}
		return s.opts.Documents.ResolveReconciliationCase(ctx, tx, record.ID)
	})
	if err != nil {
		_ = s.opts.Documents.FailReconciliationCase(ctx, s.opts.Pool, record.ID,
			"repair_failed", config.Backoff, config.MaxAttempts)
		s.opts.Metrics.IncCounter("fleet_partner_document_reconciliation_failures_total", nil, 1)
		return
	}
	s.opts.Metrics.IncCounter("fleet_partner_document_reconciliations_total",
		map[string]string{"reason": record.Reason}, 1)
}

// RunReconciliationWorker reconciles until ctx ends.
func (s *Service) RunReconciliationWorker(ctx context.Context, interval time.Duration,
	config ReconciliationConfig) error {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.RunReconciliationPass(ctx, config); err != nil {
			s.opts.Metrics.IncCounter("fleet_partner_document_worker_failures_total",
				map[string]string{"worker": "reconciliation"}, 1)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// ReplayReconciliationCase returns a dead-lettered case to the queue.
func (s *Service) ReplayReconciliationCase(ctx context.Context, organizationID string,
	caseID string) error {
	err := s.opts.Documents.ReplayReconciliationCase(ctx, nil, organizationID, caseID)
	if errors.Is(err, repositories.ErrNotFound) {
		return errorcodes.NotFound(errorcodes.DocumentNotFound,
			"No dead-lettered reconciliation case found.")
	}
	return err
}

// OpenReconciliation records a disagreement for one document, for callers that
// detected it outside the event path.
func (s *Service) OpenReconciliation(ctx context.Context, organizationID string,
	documentID string, reason string) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		document, err := s.opts.Documents.LockByID(ctx, tx, organizationID, documentID)
		if errors.Is(err, repositories.ErrNotFound) {
			return errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
		}
		if err != nil {
			return err
		}
		fromState := document.LifecycleState
		currentVersion := document.Version
		document.ProjectionFreshness = models.FreshnessReconciling
		document.LifecycleState = models.StateReconciling
		if updateErr := s.opts.Documents.Update(ctx, tx, document, currentVersion); updateErr != nil {
			return updateErr
		}
		if caseErr := s.opts.Documents.OpenReconciliationCase(ctx, tx, &models.ReconciliationCase{
			OrganizationID: document.OrganizationID,
			DocumentID:     document.ID,
			MediaObjectID:  document.MediaObjectID,
			Reason:         reason,
		}); caseErr != nil {
			return caseErr
		}
		if histErr := s.opts.Documents.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: document.OrganizationID,
			DocumentID:     document.ID,
			Action:         models.ActionReconciliationOpened,
			FromState:      fromState,
			ToState:        document.LifecycleState,
			ActorType:      models.ActorSystem,
			ActorID:        "fleet-partner-service",
			Metadata:       map[string]any{"reason": reason},
		}); histErr != nil {
			return histErr
		}
		return s.invalidateDecisions(ctx, tx, document, models.ReasonDocumentReconciling, "")
	})
}
