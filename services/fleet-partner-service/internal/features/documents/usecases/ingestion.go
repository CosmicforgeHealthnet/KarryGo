package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
)

// Media event ingestion.
//
// Six protections apply to every inbound event, and each exists because of a
// specific way a projection goes wrong:
//
//	inbox deduplication  - the same event delivered twice must not apply twice
//	payload hash         - the same event id with a DIFFERENT body is a
//	                       contract or security error and is never applied
//	older-event guard    - a late delivery must not overwrite a newer state
//	version-gap detection- a jump in versions means something was missed, and
//	                       a projection with a hole in it cannot be trusted
//	tenant check         - the event's organization must match the document's
//	consumer name        - so a second consumer's progress is tracked separately
//
// The transport (HMAC, caller allowlist, timestamp freshness, nonce replay) is
// enforced by the route before anything here runs.

// ConsumerName identifies this consumer in the inbox.
const ConsumerName = "fleet-documents"

// Ingestion outcomes, returned to the caller so a publisher can distinguish
// "applied" from "already had it" without parsing an error string.
const (
	OutcomeProcessed        = "PROCESSED"
	OutcomeDuplicate        = "DUPLICATE"
	OutcomeDuplicateVersion = "DUPLICATE_VERSION"
	OutcomeOlder            = "OLDER"
	OutcomeGapDetected      = "GAP_DETECTED"
	OutcomeIgnored          = "IGNORED"
)

// ErrEventConflict means the same event id or source version arrived with a
// different payload. It is never applied: one of the two bodies is wrong, and
// applying either would make the projection unexplainable.
var ErrEventConflict = errors.New("document event: payload conflict")

// ErrEventInvalid means the envelope is unusable.
var ErrEventInvalid = errors.New("document event: invalid envelope")

// mediaEventPayload is the subset of media's event body fleet consumes.
//
// Deliberately narrow. Fleet does not want the storage key, and a field it
// does not decode is a field it cannot accidentally store.
type mediaEventPayload struct {
	MediaObjectID         string `json:"mediaObjectId"`
	OrganizationID        string `json:"organizationId"`
	UploadStatus          string `json:"uploadStatus"`
	SafetyStatus          string `json:"safetyStatus"`
	SafetyReason          string `json:"safetyReason"`
	DeletionState         string `json:"deletionState"`
	SignatureValidation   string `json:"signatureValidation"`
	DetectedMIME          string `json:"detectedMime"`
	SizeBytes             int64  `json:"sizeBytes"`
	ChecksumSHA256        string `json:"checksumSha256"`
	IntegrityHash         string `json:"integrityHash"`
	OriginalMIMEType      string `json:"originalMimeType"`
	OriginalSizeBytes     int64  `json:"originalSizeBytes"`
	ValidationFailureCode string `json:"validationFailureCode"`
	SourceVersion         int64  `json:"sourceVersion"`

	// Capture metadata. media-file-service does not send these fields today
	// (verified against internal/features/mediaobjects/usecases/service.go in
	// that service as of this change — its event payload carries only
	// detectedMime/sizeBytes/checksumSha256). They are decoded here so the
	// mapping is ready the moment an upstream contract adds them; until then
	// every field below stays empty/zero and the corresponding document
	// column is never set to anything but its default.
	CapturedAt      *time.Time `json:"capturedAt"`
	SourceTimestamp *time.Time `json:"sourceTimestamp"`
	CaptureMethod   string     `json:"captureMethod"`
	DeviceReference string     `json:"deviceReference"`
	ActorReference  string     `json:"actorReference"`
}

// IngestMediaEvent applies one authenticated media event.
func (s *Service) IngestMediaEvent(ctx context.Context, raw []byte) (string, error) {
	if s.opts.Flags.KillSwitch || !s.opts.Flags.EventIngestionEnabled {
		return "", fmt.Errorf("%w: event ingestion is disabled", ErrEventInvalid)
	}

	var envelope events.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", fmt.Errorf("%w: %v", ErrEventInvalid, err)
	}
	if envelope.Producer != "media-file-service" || envelope.EventID == "" ||
		envelope.AggregateID == "" || envelope.AggregateVersion < 1 || envelope.OrganizationID == "" {
		return "", ErrEventInvalid
	}

	var payload mediaEventPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return "", fmt.Errorf("%w: %v", ErrEventInvalid, err)
	}
	if payload.MediaObjectID == "" {
		payload.MediaObjectID = envelope.AggregateID
	}
	if payload.MediaObjectID != envelope.AggregateID {
		// The envelope and the body disagree about what the event is about.
		return "", ErrEventInvalid
	}
	sourceVersion := payload.SourceVersion
	if sourceVersion <= 0 {
		sourceVersion = envelope.AggregateVersion
	}

	payloadHash := events.HashPayload(envelope.Payload)
	// Correlation IDs and delivery timestamps are transport metadata and may be
	// regenerated by a relay. Deduplication is therefore bound to the stable
	// event identity and business payload, not to the raw envelope bytes.
	dedupePayload := append([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s\x00%d\x00%s\x00",
		envelope.EventType, envelope.SchemaVersion, envelope.AggregateType,
		envelope.AggregateID, envelope.AggregateVersion, envelope.OrganizationID)), envelope.Payload...)
	inbox := events.NewInboxRepository(s.opts.Pool)
	outcome := OutcomeProcessed

	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := inbox.Begin(ctx, tx, ConsumerName, envelope.EventID,
			envelope.EventType, envelope.SchemaVersion, dedupePayload); err != nil {
			if errors.Is(err, events.ErrDuplicateEvent) {
				outcome = OutcomeDuplicate
				return nil
			}
			if errors.Is(err, events.ErrPayloadMismatch) {
				return ErrEventConflict
			}
			return err
		}

		document, err := s.opts.Documents.GetByMediaObject(ctx, tx, envelope.AggregateID)
		if errors.Is(err, repositories.ErrNotFound) {
			// A media object fleet holds no document for. Acknowledged rather
			// than retried forever: media serves other consumers too, and an
			// event about somebody else's object is not fleet's problem.
			outcome = OutcomeIgnored
			return inbox.MarkProcessed(ctx, tx, ConsumerName, envelope.EventID)
		}
		if err != nil {
			return err
		}
		if document.OrganizationID != envelope.OrganizationID {
			// The event claims a tenant the document does not belong to.
			// Applying it would let one tenant's event rewrite another's
			// document state.
			return ErrEventConflict
		}

		prior, versionErr := s.opts.Documents.LockSourceVersion(ctx, tx, envelope.AggregateID)
		if versionErr != nil && !errors.Is(versionErr, repositories.ErrNotFound) {
			return versionErr
		}
		known := versionErr == nil

		if known {
			if sourceVersion < prior.Version {
				// A late delivery. Acknowledged, never applied.
				outcome = OutcomeOlder
				return inbox.MarkProcessed(ctx, tx, ConsumerName, envelope.EventID)
			}
			if sourceVersion == prior.Version {
				if payloadHash != prior.PayloadHash {
					// The same version with a different body. One of the two is
					// wrong; applying either would make the projection
					// unexplainable afterwards.
					return ErrEventConflict
				}
				outcome = OutcomeDuplicateVersion
				return inbox.MarkProcessed(ctx, tx, ConsumerName, envelope.EventID)
			}
		}

		freshness := models.FreshnessCurrent
		gap := known && sourceVersion > prior.Version+1
		if gap {
			// Something was missed. A projection with a hole in it cannot be
			// trusted, so it is marked and a reconciliation case is opened.
			freshness = models.FreshnessGapDetected
			outcome = OutcomeGapDetected
		}

		if applyErr := s.applyMediaState(ctx, tx, document, payload, sourceVersion,
			freshness, envelope.EventID, envelope.CorrelationID); applyErr != nil {
			return applyErr
		}

		if gap {
			if caseErr := s.opts.Documents.OpenReconciliationCase(ctx, tx, &models.ReconciliationCase{
				OrganizationID: document.OrganizationID,
				DocumentID:     document.ID,
				MediaObjectID:  document.MediaObjectID,
				Reason:         models.ReconcileVersionGap,
				CorrelationID:  envelope.CorrelationID,
			}); caseErr != nil {
				return caseErr
			}
			if publishErr := s.publish(ctx, tx, EventDocumentReconciling, document,
				envelope.CorrelationID, map[string]any{"reason": models.ReconcileVersionGap}); publishErr != nil {
				return publishErr
			}
		}

		if versionErr := s.opts.Documents.RecordSourceVersion(ctx, tx, repositories.SourceVersion{
			MediaObjectID: envelope.AggregateID,
			Version:       sourceVersion,
			LastEventID:   envelope.EventID,
			PayloadHash:   payloadHash,
		}); versionErr != nil {
			return versionErr
		}

		return inbox.MarkProcessed(ctx, tx, ConsumerName, envelope.EventID)
	})
	if err != nil {
		return "", err
	}

	s.opts.Metrics.IncCounter("fleet_partner_document_events_total",
		map[string]string{"type": envelope.EventType, "outcome": outcome}, 1)
	return outcome, nil
}

// applyMediaState projects media's answer onto the document.
//
// Fleet's own decisions are preserved where they still hold and voided where
// they do not: a verification survives a routine status refresh, and is
// invalidated the moment the file stops being SAFE, is deleted, or arrives at
// a source version the verification was not taken against.
func (s *Service) applyMediaState(ctx context.Context, tx pgx.Tx, document *models.Document,
	payload mediaEventPayload, sourceVersion int64, freshness string,
	eventID string, correlationID string) error {
	fromState := document.LifecycleState
	currentVersion := document.Version
	now := s.opts.Now().UTC()

	previousSafety := document.SafetyProjection
	previousSourceVersion := document.MediaSourceVersion

	if payload.UploadStatus != "" {
		document.MediaUploadStatus = payload.UploadStatus
	}
	if payload.SafetyStatus != "" {
		document.SafetyProjection = payload.SafetyStatus
	}
	if payload.DeletionState != "" {
		document.MediaDeletionState = payload.DeletionState
	}
	document.MediaSourceVersion = sourceVersion
	document.ProjectionFreshness = freshness
	document.LastMediaEventID = &eventID
	document.SyncedAt = &now

	// Authoritative media facts: checksum, detected MIME and size, plus
	// capture metadata if the event carries it. applyMediaState only ever
	// runs on a strictly-newer source version (equal/older versions return
	// early in IngestMediaEvent), so overwriting unconditionally here cannot
	// regress a newer fact with an older one — the forward-only guard is
	// already enforced by the caller.
	if payload.IntegrityHash != "" {
		document.MediaChecksumSHA256 = payload.IntegrityHash
	} else if payload.ChecksumSHA256 != "" {
		document.MediaChecksumSHA256 = payload.ChecksumSHA256
	}
	if payload.OriginalMIMEType != "" {
		document.OriginalMimeType = payload.OriginalMIMEType
	} else if payload.DetectedMIME != "" {
		document.OriginalMimeType = payload.DetectedMIME
	}
	if payload.OriginalSizeBytes > 0 {
		document.OriginalSizeBytes = payload.OriginalSizeBytes
	} else if payload.SizeBytes > 0 {
		document.OriginalSizeBytes = payload.SizeBytes
	}
	if payload.CapturedAt != nil {
		document.CapturedAt = payload.CapturedAt
	}
	if payload.SourceTimestamp != nil {
		document.SourceTimestamp = payload.SourceTimestamp
	}
	if payload.CaptureMethod != "" {
		document.CaptureMethod = payload.CaptureMethod
	}
	if payload.DeviceReference != "" {
		document.DeviceReference = payload.DeviceReference
	}
	if payload.ActorReference != "" {
		document.ActorReference = payload.ActorReference
	}

	// Does the standing verification still hold?
	//
	// Three things void it, and each is a real way a verified document stops
	// meaning what it meant: the content is no longer confirmed safe, the
	// object is gone, or media is describing a different version of the file
	// from the one that was signed off.
	verificationVoided := ""
	switch {
	case document.SafetyProjection != models.SafetySafe && previousSafety == models.SafetySafe:
		verificationVoided = models.RejectUnsafeContent
	case document.MediaDeletionState == models.MediaDeletionDeleted:
		verificationVoided = "DOCUMENT_DELETED"
	case previousSourceVersion > 0 && sourceVersion > previousSourceVersion &&
		document.VerificationState == models.VerificationVerified &&
		payload.ChecksumSHA256 != "" && document.MediaUploadStatus == models.MediaUploadCompleted &&
		previousSafety != models.SafetyPending:
		// A new source version on an already-verified document means the file
		// changed underneath the sign-off.
		verificationVoided = models.RejectSourceChanged
	}
	if verificationVoided != "" {
		if err := s.InvalidateVerification(ctx, tx, document, verificationVoided, correlationID); err != nil {
			return err
		}
	}

	document.LifecycleState = models.LifecycleForMedia(document.MediaUploadStatus,
		document.SafetyProjection, document.MediaDeletionState, freshness,
		document.VerificationState == models.VerificationVerified)
	if document.MediaDeletionState == models.MediaDeletionDeleted && document.DeletedAt == nil {
		document.DeletedAt = &now
	}

	if err := s.opts.Documents.Update(ctx, tx, document, currentVersion); err != nil {
		return err
	}

	action := models.ActionSafetyProjected
	switch {
	case freshness == models.FreshnessGapDetected:
		action = models.ActionGapDetected
	case document.SafetyProjection == models.SafetyQuarantined:
		action = models.ActionQuarantined
	case document.MediaDeletionState == models.MediaDeletionDeleted:
		action = models.ActionDeletionCompleted
	case payload.ValidationFailureCode != "":
		action = models.ActionValidationFailed
	case document.MediaUploadStatus == models.MediaUploadCompleted && fromState == models.StateUploadPending:
		action = models.ActionUploadCompleted
	}

	if err := s.opts.Documents.AppendHistory(ctx, tx, &models.HistoryEntry{
		OrganizationID: document.OrganizationID,
		DocumentID:     document.ID,
		Action:         action,
		FromState:      fromState,
		ToState:        document.LifecycleState,
		ActorType:      models.ActorPlatformService,
		ActorID:        "media-file-service",
		Metadata: map[string]any{
			"uploadStatus":  document.MediaUploadStatus,
			"safetyStatus":  document.SafetyProjection,
			"deletionState": document.MediaDeletionState,
			"sourceVersion": sourceVersion,
			"failureCode":   payload.ValidationFailureCode,
		},
		CorrelationID: correlationID,
	}); err != nil {
		return err
	}

	if err := s.opts.Audit.Record(ctx, tx, audit.Entry{
		OrganizationID: document.OrganizationID,
		ActorType:      "service",
		ActorID:        "media-file-service",
		Action:         "document.media_state.projected",
		ResourceType:   "document",
		ResourceID:     document.ID,
		PreviousValues: map[string]any{
			"lifecycleState": fromState, "safetyProjection": previousSafety,
		},
		NewValues: map[string]any{
			"lifecycleState":   document.LifecycleState,
			"safetyProjection": document.SafetyProjection,
			"sourceVersion":    sourceVersion,
			"freshness":        freshness,
		},
		CorrelationID: correlationID,
	}); err != nil {
		return err
	}

	// Anything that could stop this document satisfying a requirement must
	// invalidate the decisions that relied on it.
	if verificationVoided != "" || document.SafetyProjection != models.SafetySafe ||
		document.MediaDeletionState != models.MediaDeletionActive {
		reason := models.ReasonDocumentNotSafe
		if document.MediaDeletionState == models.MediaDeletionDeleted {
			reason = models.ReasonDocumentDeleted
		}
		if err := s.invalidateDecisions(ctx, tx, document, reason, correlationID); err != nil {
			return err
		}
	}

	return s.publish(ctx, tx, EventDocumentStateChanged, document, correlationID, nil)
}

// staleThreshold exposes the configured stale window to the workers.
func (s *Service) staleThreshold() time.Duration { return s.opts.StaleAfter }
