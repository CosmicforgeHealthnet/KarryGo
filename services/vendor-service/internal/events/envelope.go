// Package events implements vendor-service's transactional outbox, inbox
// dedupe, and event envelope.
//
// Every event carries a globally unique event ID, a versioned event type,
// aggregate identity and version, correlation and causation IDs and the
// producing service, matching the platform's existing convention (see
// payment-wallet-service's and fleet-partner-service's internal/events
// packages, and docs/architecture/08_EVENT_OWNERSHIP.md). This package is
// vendor-service's own copy, not an import from those services: as of this
// architecture phase there is no single shared/go event-envelope package to
// reuse (see docs/architecture/00_REPOSITORY_AUDIT_BASELINE.md, "Outbox /
// inbox / event patterns" — four different tiers of this pattern coexist in
// the platform today, none of them in shared/go).
package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// ServiceName identifies this producer on the wire.
const ServiceName = "vendor-service"

// Envelope is the wire header of every vendor-service event.
type Envelope struct {
	EventID          string          `json:"eventId"`
	EventType        string          `json:"eventType"`
	SchemaVersion    int             `json:"schemaVersion"`
	AggregateType    string          `json:"aggregateType"`
	AggregateID      string          `json:"aggregateId"`
	AggregateVersion int64           `json:"aggregateVersion"`
	VendorID         string          `json:"vendorId,omitempty"`
	CorrelationID    string          `json:"correlationId"`
	CausationID      string          `json:"causationId,omitempty"`
	OccurredAt       time.Time       `json:"occurredAt"`
	Producer         string          `json:"producer"`
	Payload          json.RawMessage `json:"payload"`
}

// NewEnvelope builds a validated event envelope. platformScoped must be true
// for events with no single owning vendor business; otherwise a vendor ID is
// mandatory.
func NewEnvelope(eventType string, schemaVersion int, aggregateType, aggregateID string,
	aggregateVersion int64, vendorID, correlationID, causationID string,
	payload any, platformScoped bool) (Envelope, error) {
	if eventType == "" || aggregateType == "" || aggregateID == "" {
		return Envelope{}, fmt.Errorf("events: event type and aggregate identity are required")
	}
	if schemaVersion < 1 {
		return Envelope{}, fmt.Errorf("events: schema version must be positive")
	}
	if vendorID == "" && !platformScoped {
		return Envelope{}, fmt.Errorf("events: vendor id is required for vendor-owned events")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("events: marshal payload: %w", err)
	}
	return Envelope{
		EventID:          newEventID(),
		EventType:        eventType,
		SchemaVersion:    schemaVersion,
		AggregateType:    aggregateType,
		AggregateID:      aggregateID,
		AggregateVersion: aggregateVersion,
		VendorID:         vendorID,
		CorrelationID:    correlationID,
		CausationID:      causationID,
		OccurredAt:       time.Now().UTC(),
		Producer:         ServiceName,
		Payload:          raw,
	}, nil
}

func newEventID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return "evt_" + hex.EncodeToString(buf)
}
