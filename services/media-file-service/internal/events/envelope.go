// Package events implements the transactional outbox and the versioned event
// envelope for media-file-service.
//
// Every media event carries a globally unique event ID, a versioned event
// type, aggregate identity and version, correlation and causation IDs and the
// producing service. Consumers use the aggregate version to reject
// out-of-order deliveries and to detect gaps.
//
// What an event never carries: raw file bytes, a signed URL, a storage
// credential, or the storage object key. A consumer that needs the file asks
// for a fresh short-lived grant; a consumer that only needs to know the file's
// standing gets exactly that.
package events

import (
	"encoding/json"
	"fmt"
	"time"

	"cosmicforge/logistics/services/media-file-service/internal/platform/ids"
)

// ServiceName identifies this producer on the wire.
const ServiceName = "media-file-service"

// Envelope is the wire header of every media event.
type Envelope struct {
	EventID          string          `json:"eventId"`
	EventType        string          `json:"eventType"`
	SchemaVersion    int             `json:"schemaVersion"`
	AggregateType    string          `json:"aggregateType"`
	AggregateID      string          `json:"aggregateId"`
	AggregateVersion int64           `json:"aggregateVersion"`
	OrganizationID   string          `json:"organizationId,omitempty"`
	CorrelationID    string          `json:"correlationId"`
	CausationID      string          `json:"causationId,omitempty"`
	OccurredAt       time.Time       `json:"occurredAt"`
	Producer         string          `json:"producer"`
	Payload          json.RawMessage `json:"payload"`
}

// AggregateMediaObject is the only aggregate type this service publishes.
const AggregateMediaObject = "MEDIA_OBJECT"

// Media event types. Fleet consumes these.
const (
	EventUploadCompleted        = "media.upload_completed.v1"
	EventValidationPassed       = "media.validation_passed.v1"
	EventValidationFailed       = "media.validation_failed.v1"
	EventScanCompleted          = "media.scan_completed.v1"
	EventUnsafeDetected         = "media.unsafe_detected.v1"
	EventQuarantined            = "media.quarantined.v1"
	EventDeletionCompleted      = "media.deletion_completed.v1"
	EventDeletionFailed         = "media.deletion_failed.v1"
	EventReconciliationRequired = "media.reconciliation_required.v1"
	EventAssetStatusChanged     = "media.asset.status_changed.v1"
)

// NewEnvelope builds a validated media event envelope.
//
// An organization ID is mandatory: every authoritative media object is owned
// by a tenant, and an event with no tenant could not be routed to a tenant-
// scoped consumer without guessing.
func NewEnvelope(eventType string, schemaVersion int, aggregateType string, aggregateID string,
	aggregateVersion int64, organizationID string, correlationID string, causationID string,
	payload any) (Envelope, error) {
	if eventType == "" || aggregateType == "" || aggregateID == "" {
		return Envelope{}, fmt.Errorf("events: event type and aggregate identity are required")
	}
	if schemaVersion < 1 {
		return Envelope{}, fmt.Errorf("events: schema version must be positive")
	}
	if aggregateVersion < 1 {
		return Envelope{}, fmt.Errorf("events: aggregate version must be positive")
	}
	if organizationID == "" {
		return Envelope{}, fmt.Errorf("events: organization id is required")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("events: marshal payload: %w", err)
	}
	return Envelope{
		EventID:          ids.New(ids.PrefixEvent),
		EventType:        eventType,
		SchemaVersion:    schemaVersion,
		AggregateType:    aggregateType,
		AggregateID:      aggregateID,
		AggregateVersion: aggregateVersion,
		OrganizationID:   organizationID,
		CorrelationID:    correlationID,
		CausationID:      causationID,
		OccurredAt:       time.Now().UTC(),
		Producer:         ServiceName,
		Payload:          raw,
	}, nil
}
