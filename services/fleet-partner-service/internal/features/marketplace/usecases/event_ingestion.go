package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
)

// Marketplace event types accepted from a source execution service.
const (
	EventRequestOffered            = "platform.request.offered.v1"
	EventRequestUpdated            = "platform.request.updated.v1"
	EventRequestReserved           = "platform.request.reserved.v1"
	EventRequestReservationRelease = "platform.request.reservation_released.v1"
	EventRequestExpired            = "platform.request.expired.v1"
	EventRequestCancelled          = "platform.request.cancelled.v1"
	EventTripCreated               = "trip.created.v1"
	EventTripStatusChanged         = "trip.status_changed.v1"
	EventTripCompleted             = "trip.completed.v1"
	EventTripCancelled             = "trip.cancelled.v1"
	EventProofCompleted            = "proof.completed.v1"
	// WP6.3–6.4. Both are TRIP-aggregate events published by the authoritative
	// execution service; fleet only projects them.
	EventTripRecipientConfirmed = "trip.recipient_confirmed.v1"
	EventTripDeliveryFailed     = "trip.delivery_failed.v1"
)

// acceptedEventTypes is the closed set this service will ingest.
//
// A closed set rather than a prefix match: an unrecognised type must be a
// permanent rejection so the producer dead-letters it and someone looks,
// rather than a silent success that loses the event.
var acceptedEventTypes = map[string]bool{
	EventRequestOffered:            true,
	EventRequestUpdated:            true,
	EventRequestReserved:           true,
	EventRequestReservationRelease: true,
	EventRequestExpired:            true,
	EventRequestCancelled:          true,
	EventTripCreated:               true,
	EventTripStatusChanged:         true,
	EventTripCompleted:             true,
	EventTripCancelled:             true,
	EventProofCompleted:            true,
	EventTripRecipientConfirmed:    true,
	EventTripDeliveryFailed:        true,
}

// IngestStatus is the outcome reported back to the producer.
type IngestStatus string

const (
	// IngestAccepted — the event was committed by this service.
	IngestAccepted IngestStatus = "ACCEPTED"
	// IngestDuplicate — already processed; acknowledging without reprocessing.
	IngestDuplicate IngestStatus = "DUPLICATE"
	// IngestIgnored — valid but not applicable (for example, stale version).
	IngestIgnored IngestStatus = "IGNORED"
)

// IngestResult is the ingestion outcome.
type IngestResult struct {
	Status  IngestStatus `json:"status"`
	EventID string       `json:"eventId"`
	Code    string       `json:"code,omitempty"`
}

// InboundEvent is the envelope a source service delivers.
type InboundEvent struct {
	EventID          string         `json:"eventId"`
	EventType        string         `json:"eventType"`
	SchemaVersion    int            `json:"schemaVersion"`
	SourceService    string         `json:"sourceService"`
	AggregateType    string         `json:"aggregateType"`
	AggregateID      string         `json:"aggregateId"`
	AggregateVersion int64          `json:"aggregateVersion"`
	OccurredAt       time.Time      `json:"occurredAt"`
	CorrelationID    string         `json:"correlationId"`
	CausationID      string         `json:"causationId"`
	PayloadHash      string         `json:"payloadHash"`
	Payload          map[string]any `json:"payload"`
}

// IngestError is a rejection carrying the stable code the producer classifies
// on.
//
// A dedicated type rather than the generic error envelope because the code is
// a contract: the producer decides "dead-letter" versus "retry" from it, so it
// must not drift with the shared error format.
type IngestError struct {
	Code    string
	Message string
	Status  int
}

// Error renders the rejection.
func (e *IngestError) Error() string { return e.Code + ": " + e.Message }

// Validate checks the envelope before any database work.
func (e *InboundEvent) Validate() *IngestError {
	switch {
	case strings.TrimSpace(e.EventID) == "":
		return codedBadRequest("invalid_envelope", "eventId is required.")
	case strings.TrimSpace(e.EventType) == "":
		return codedBadRequest("invalid_envelope", "eventType is required.")
	case !acceptedEventTypes[e.EventType]:
		return codedBadRequest("unknown_event_type", "Event type is not supported.")
	case e.SchemaVersion < 1:
		return codedBadRequest("schema_version_invalid", "schemaVersion must be positive.")
	case strings.TrimSpace(e.SourceService) == "":
		return codedBadRequest("invalid_envelope", "sourceService is required.")
	case strings.TrimSpace(e.AggregateID) == "":
		return codedBadRequest("invalid_envelope", "aggregateId is required.")
	case e.AggregateVersion < 1:
		return codedBadRequest("invalid_envelope", "aggregateVersion must be positive.")
	case e.OccurredAt.IsZero():
		return codedBadRequest("invalid_envelope", "occurredAt is required.")
	}
	return nil
}

// codedBadRequest builds a permanent rejection carrying a stable code.
func codedBadRequest(code string, message string) *IngestError {
	return &IngestError{Code: code, Message: message, Status: http.StatusBadRequest}
}

// consumerName namespaces inbox entries by producing service.
//
// Two producers could legitimately mint the same event id; keying the inbox
// on the pair means one producer can never suppress another's event.
func consumerName(sourceService string) string {
	return "marketplace-events:" + sourceService
}

// IngestEvent applies one source event atomically.
//
// Everything that must be all-or-nothing happens in a single transaction: the
// inbox claim, the projection change and the inbox completion. A crash between
// them would otherwise either lose the effect or mark an unapplied event as
// processed, and a redelivery could not tell which.
//
// The envelope's declared payloadHash is verified against the bytes we
// actually received before anything is written. A mismatch means the payload
// was altered in transit or the producer's own hash is wrong; either way it is
// a permanent rejection, never a retry.
func (s *Service) IngestEvent(ctx context.Context, raw []byte, event InboundEvent) (IngestResult, error) {
	if appErr := event.Validate(); appErr != nil {
		return IngestResult{}, appErr
	}

	payloadBytes, err := json.Marshal(orEmptyPayload(event.Payload))
	if err != nil {
		return IngestResult{}, codedBadRequest("invalid_envelope", "payload is not encodable.")
	}
	if declared := strings.TrimSpace(event.PayloadHash); declared != "" {
		if events.HashPayload(payloadBytes) != declared {
			return IngestResult{}, codedBadRequest("payload_hash_mismatch",
				"Declared payload hash does not match the delivered payload.")
		}
	}

	consumer := consumerName(event.SourceService)
	inbox := events.NewInboxRepository(s.opts.Pool)
	result := IngestResult{Status: IngestAccepted, EventID: event.EventID}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if beginErr := inbox.Begin(ctx, tx, consumer, event.EventID, event.EventType,
			event.SchemaVersion, payloadBytes); beginErr != nil {
			return beginErr
		}
		outcome, applyErr := s.applyEvent(ctx, tx, event)
		if applyErr != nil {
			return applyErr
		}
		if outcome == repositories.SourceEventStale {
			result.Status = IngestIgnored
			result.Code = "stale_source_version"
		}
		return inbox.MarkProcessed(ctx, tx, consumer, event.EventID)
	})

	switch {
	case err == nil:
		return result, nil
	case errors.Is(err, events.ErrDuplicateEvent):
		// Already committed by an earlier delivery. Acknowledging is what makes
		// the producer's redelivery-after-unknown-outcome safe.
		return IngestResult{Status: IngestDuplicate, EventID: event.EventID}, nil
	case errors.Is(err, events.ErrPayloadMismatch):
		return IngestResult{}, codedBadRequest("payload_hash_mismatch",
			"This event id was already received with a different payload.")
	case errors.Is(err, repositories.ErrNotFound):
		// No projection for this request. Not an error the producer can fix by
		// retrying with the same event, and not something we invent state for.
		return IngestResult{Status: IngestIgnored, EventID: event.EventID,
			Code: "projection_not_found"}, nil
	default:
		var ingestErr *IngestError
		if errors.As(err, &ingestErr) {
			return IngestResult{}, ingestErr
		}
		return IngestResult{}, err
	}
}

// ApplyStreamEventTx applies a registry-validated stream event inside the
// common consumer's inbox transaction. The common StreamConsumer owns the
// producer/event-id deduplication and marks the inbox row processed only when
// this projection change commits with it.
func (s *Service) ApplyStreamEventTx(ctx context.Context, tx pgx.Tx, envelope events.Envelope) error {
	var payload map[string]any
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return codedBadRequest("invalid_envelope", "payload is not decodable.")
	}
	event := InboundEvent{
		EventID:          envelope.EventID,
		EventType:        envelope.EventType,
		SchemaVersion:    envelope.SchemaVersion,
		SourceService:    envelope.Producer,
		AggregateType:    envelope.AggregateType,
		AggregateID:      envelope.AggregateID,
		AggregateVersion: envelope.AggregateVersion,
		OccurredAt:       envelope.OccurredAt,
		CorrelationID:    envelope.CorrelationID,
		PayloadHash:      events.HashPayload(envelope.Payload),
		Payload:          payload,
	}
	if appErr := event.Validate(); appErr != nil {
		return appErr
	}
	_, err := s.applyEvent(ctx, tx, event)
	if errors.Is(err, repositories.ErrNotFound) {
		// The shared stream carries events for many fleet organizations. A
		// request/trip that has no local projection is intentionally ignored;
		// inventing one would fabricate source-owned state.
		return nil
	}
	return err
}

// orEmptyPayload normalises a nil payload so hashing is stable.
func orEmptyPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	return payload
}

// applyEvent routes one validated event to its handler.
func (s *Service) applyEvent(ctx context.Context, tx pgx.Tx,
	event InboundEvent) (repositories.SourceEventOutcome, error) {
	if privateExecutionOperationID(event.Payload) != "" {
		if s.opts.PrivateEvents == nil {
			return "", codedBadRequest("private_execution_projection_unavailable",
				"Private execution event projection is unavailable.")
		}
		return s.opts.PrivateEvents.ProjectPrivateExecutionEvent(ctx, tx, event)
	}
	// Route on which aggregate the event's version belongs to. Trip and proof
	// events carry TRIP versions; applying them to the request projection's
	// counter would compare two unrelated sequences.
	if tripAggregateEvents[event.EventType] {
		return s.applyTripEvent(ctx, tx, event)
	}

	projection, err := s.opts.Projections.FindBySourceRequestAnyOrganization(ctx, tx,
		event.SourceService, sourceRequestID(event))
	if err != nil {
		return "", err
	}

	return s.opts.Projections.ApplySourceEvent(ctx, tx, projection.ID, event.AggregateVersion,
		event.EventID, event.OccurredAt, func(freshness string) error {
			return s.applyProjectionChange(ctx, tx, projection, event)
		})
}

func privateExecutionOperationID(payload map[string]any) string {
	for _, key := range []string{"fleetExecutionOperationId", "fleetOperationId"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// sourceRequestID extracts the request identity an event refers to.
//
// Trip and proof events are aggregated on the trip, so they must name the
// originating request explicitly; there is no cross-service lookup that could
// recover it, and guessing is not an option.
func sourceRequestID(event InboundEvent) string {
	if event.AggregateType == "request" && event.AggregateID != "" {
		return event.AggregateID
	}
	for _, key := range []string{"requestId", "sourceRequestId", "platformRequestId"} {
		if value, ok := event.Payload[key].(string); ok && value != "" {
			return value
		}
	}
	return event.AggregateID
}

// applyProjectionChange performs the per-type state change.
func (s *Service) applyProjectionChange(ctx context.Context, tx pgx.Tx,
	projection *models.RequestProjection, event InboundEvent) error {
	projections := s.opts.Projections

	setStatus := func(status string) error {
		return projections.SetProjectionStatusUnversioned(ctx, tx, projection.ID, status)
	}
	if sourceStatus, ok := event.Payload["status"].(string); ok && sourceStatus != "" {
		if err := projections.SetSourceStatus(ctx, tx, projection.ID, sourceStatus); err != nil {
			return err
		}
	}

	switch event.EventType {
	case EventRequestOffered, EventRequestUpdated:
		return nil

	case EventRequestReserved:
		return setStatus(models.ProjectionReservationPending)

	case EventRequestReservationRelease:
		// A released reservation returns the request to the open pool; it is
		// not a terminal state and the organization may be offered it again.
		return setStatus(models.ProjectionOffered)

	case EventRequestExpired:
		if err := setStatus(models.ProjectionExpired); err != nil {
			return err
		}
		return s.cancelOpenWork(ctx, tx, projection, "source_request_expired")

	case EventRequestCancelled:
		if err := setStatus(models.ProjectionCancelled); err != nil {
			return err
		}
		return s.cancelOpenWork(ctx, tx, projection, "source_request_cancelled")

	case EventTripCreated:
		if tripID, ok := event.Payload["tripId"].(string); ok && tripID != "" {
			if err := projections.SetPlatformTripID(ctx, tx, projection.ID, tripID); err != nil {
				return err
			}
		}
		return setStatus(models.ProjectionExecutionCreated)

	case EventTripStatusChanged:
		return nil

	case EventTripCompleted:
		return setStatus(models.ProjectionCompleted)

	case EventTripCancelled:
		if err := setStatus(models.ProjectionCancelled); err != nil {
			return err
		}
		return s.cancelOpenWork(ctx, tx, projection, "source_trip_cancelled")

	case EventProofCompleted:
		return nil

	case EventTripRecipientConfirmed, EventTripDeliveryFailed:
		// Both are TRIP-aggregate events and never reach this request-aggregate
		// path; the case exists so the default below stays a genuine "unknown
		// type" rather than a routing bug in disguise.
		return nil

	default:
		return fmt.Errorf("marketplace: no handler for event type %q", event.EventType)
	}
}

// cancelOpenWork stands down fleet-side work when the source request dies.
//
// Ordering is deliberate: the offer is closed before the assignment, so a
// provider cannot be shown a live offer for a request the source has already
// cancelled. An assignment already in EXECUTION_START_PENDING is left alone —
// trip creation is in flight and cancelling underneath it is exactly the
// interleaving the pending state exists to prevent. The reconciliation worker
// resolves those once the outcome is known.
func (s *Service) cancelOpenWork(ctx context.Context, tx pgx.Tx,
	projection *models.RequestProjection, reasonCode string) error {
	offer, err := s.opts.Offers.GetLiveByProjection(ctx, tx, projection.OrganizationID, projection.ID)
	switch {
	case errors.Is(err, repositories.ErrNotFound):
		return nil
	case err != nil:
		return err
	}

	if err := s.opts.Offers.SetStatus(ctx, tx, projection.OrganizationID, offer.ID,
		models.OfferCancelled, offer.Version, "SYSTEM", "event-ingestion",
		reasonCode, ""); err != nil {
		return err
	}
	return nil
}
