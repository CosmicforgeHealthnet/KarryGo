package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
)

// ApplyMediaStreamEventTx applies Media's authoritative state inside the
// common stream inbox transaction. The regular HMAC endpoint continues to use
// IngestMediaEvent; both paths share the same source-version ledger and
// applyMediaState, so neither can regress or fork the document projection.
func (s *Service) ApplyMediaStreamEventTx(ctx context.Context, tx pgx.Tx, envelope events.Envelope) error {
	if s.opts.Flags.KillSwitch || !s.opts.Flags.EventIngestionEnabled {
		return fmt.Errorf("%w: event ingestion is disabled", ErrEventInvalid)
	}
	if envelope.Producer != "media-file-service" || envelope.EventID == "" || envelope.AggregateID == "" || envelope.AggregateVersion < 1 || envelope.OrganizationID == "" {
		return ErrEventInvalid
	}
	var payload mediaEventPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("%w: %v", ErrEventInvalid, err)
	}
	if payload.MediaObjectID == "" {
		payload.MediaObjectID = envelope.AggregateID
	}
	if payload.MediaObjectID != envelope.AggregateID {
		return ErrEventInvalid
	}
	sourceVersion := payload.SourceVersion
	if sourceVersion <= 0 {
		sourceVersion = envelope.AggregateVersion
	}
	document, err := s.opts.Documents.GetByMediaObject(ctx, tx, envelope.AggregateID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if document.OrganizationID != envelope.OrganizationID {
		return ErrEventConflict
	}
	payloadHash := events.HashPayload(envelope.Payload)
	prior, versionErr := s.opts.Documents.LockSourceVersion(ctx, tx, envelope.AggregateID)
	if versionErr != nil && !errors.Is(versionErr, repositories.ErrNotFound) {
		return versionErr
	}
	known := versionErr == nil
	if known && sourceVersion < prior.Version {
		return nil
	}
	if known && sourceVersion == prior.Version {
		if payloadHash != prior.PayloadHash {
			return ErrEventConflict
		}
		return nil
	}
	freshness := models.FreshnessCurrent
	gap := known && sourceVersion > prior.Version+1
	if gap {
		freshness = models.FreshnessGapDetected
	}
	if err = s.applyMediaState(ctx, tx, document, payload, sourceVersion, freshness, envelope.EventID, envelope.CorrelationID); err != nil {
		return err
	}
	if gap {
		if err = s.opts.Documents.OpenReconciliationCase(ctx, tx, &models.ReconciliationCase{
			OrganizationID: document.OrganizationID, DocumentID: document.ID,
			MediaObjectID: document.MediaObjectID, Reason: models.ReconcileVersionGap,
			CorrelationID: envelope.CorrelationID,
		}); err != nil {
			return err
		}
		if err = s.publish(ctx, tx, EventDocumentReconciling, document, envelope.CorrelationID, map[string]any{"reason": models.ReconcileVersionGap}); err != nil {
			return err
		}
	}
	return s.opts.Documents.RecordSourceVersion(ctx, tx, repositories.SourceVersion{
		MediaObjectID: envelope.AggregateID, Version: sourceVersion,
		LastEventID: envelope.EventID, PayloadHash: payloadHash,
	})
}
