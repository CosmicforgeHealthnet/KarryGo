// Package events implements the transactional outbox, the consumer inbox and
// the corporate event envelope.
//
// Every organization-owned corporate event carries organizationId,
// correlationId, aggregate identity/version, a globally unique event ID, a
// versioned event type and producer. Platform-scoped events (no organization)
// must be explicitly documented in docs/events-and-jobs.md.
package events

import (
	"encoding/json"
	"fmt"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Envelope is the wire header of every corporate event.
type Envelope struct {
	EventID          string          `json:"eventId"`
	EventType        string          `json:"eventType"`
	SchemaVersion    int             `json:"schemaVersion"`
	AggregateType    string          `json:"aggregateType"`
	AggregateID      string          `json:"aggregateId"`
	AggregateVersion int64           `json:"aggregateVersion"`
	OrganizationID   string          `json:"organizationId,omitempty"`
	CorrelationID    string          `json:"correlationId"`
	OccurredAt       time.Time       `json:"occurredAt"`
	Producer         string          `json:"producer"`
	Payload          json.RawMessage `json:"payload"`
}

// NewEnvelope builds a validated corporate event envelope.
// platformScoped must be true for the documented platform-scoped events;
// otherwise an organization ID is mandatory.
func NewEnvelope(eventType string, schemaVersion int, aggregateType string, aggregateID string, aggregateVersion int64, organizationID string, correlationID string, payload any, platformScoped bool) (Envelope, error) {
	if eventType == "" || aggregateType == "" || aggregateID == "" {
		return Envelope{}, fmt.Errorf("events: event type and aggregate identity are required")
	}
	if schemaVersion < 1 {
		return Envelope{}, fmt.Errorf("events: schema version must be positive")
	}
	if organizationID == "" && !platformScoped {
		return Envelope{}, fmt.Errorf("events: organization id is required for organization-owned events")
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
		OccurredAt:       time.Now().UTC(),
		Producer:         config.ServiceName,
		Payload:          raw,
	}, nil
}
