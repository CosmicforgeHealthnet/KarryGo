package marketplace

import (
	"context"
	"fmt"
	"time"
)

// Canonical marketplace event types published to organization consumers.
//
// These names are a contract with every consumer. They are versioned in the
// type itself rather than only in schema_version so that an incompatible
// change is a new type and old consumers keep working.
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
)

// KnownEventTypes is the closed set of types this service publishes.
var KnownEventTypes = map[string]bool{
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
}

// Envelope is the wire format delivered to consumers.
//
// Deliberately flat and small. The payload is whatever the emitting
// transition allowlisted; nothing is copied wholesale from a domain row,
// because that is how commercial fields leak into an event stream.
type Envelope struct {
	EventID          string         `json:"eventId"`
	EventType        string         `json:"eventType"`
	SchemaVersion    int            `json:"schemaVersion"`
	SourceService    string         `json:"sourceService"`
	AggregateType    string         `json:"aggregateType"`
	AggregateID      string         `json:"aggregateId"`
	AggregateVersion int64          `json:"aggregateVersion"`
	OccurredAt       time.Time      `json:"occurredAt"`
	CorrelationID    string         `json:"correlationId,omitempty"`
	CausationID      string         `json:"causationId,omitempty"`
	PayloadHash      string         `json:"payloadHash"`
	Payload          map[string]any `json:"payload"`
}

// forbiddenPayloadKeys must never appear in a published payload.
//
// Enforced at publish time rather than reviewed by eye: a consumer-visible
// event stream is the easiest place for a customer's commercial terms to
// escape, and the check costs nothing.
var forbiddenPayloadKeys = map[string]bool{
	"fare":              true,
	"fareAmount":        true,
	"price":             true,
	"amount":            true,
	"commission":        true,
	"commissionRate":    true,
	"margin":            true,
	"walletId":          true,
	"walletBalance":     true,
	"balance":           true,
	"bankAccount":       true,
	"accountNumber":     true,
	"accessToken":       true,
	"refreshToken":      true,
	"secret":            true,
	"apiKey":            true,
	"password":          true,
	"customerPhone":     true,
	"customerEmail":     true,
	"payoutAmount":      true,
	"settlementAmount":  true,
	"contractRate":      true,
	"negotiatedRate":    true,
	"internalNotes":     true,
	"providerEarnings":  true,
	"platformFee":       true,
	"discountCode":      true,
	"promotionalCredit": true,
}

// ErrForbiddenPayloadField reports a payload carrying restricted data.
type ErrForbiddenPayloadField struct {
	EventType string
	Field     string
}

func (e *ErrForbiddenPayloadField) Error() string {
	return fmt.Sprintf("marketplace: event %s payload may not contain field %q",
		e.EventType, e.Field)
}

// validatePayload rejects unknown event types and restricted fields.
func validatePayload(eventType string, payload map[string]any) error {
	if !KnownEventTypes[eventType] {
		return fmt.Errorf("marketplace: unknown event type %q", eventType)
	}
	for key := range payload {
		if forbiddenPayloadKeys[key] {
			return &ErrForbiddenPayloadField{EventType: eventType, Field: key}
		}
	}
	return nil
}

// PublishMarketplaceEvent writes one validated event into the outbox.
//
// Must be called with the same transaction as the state change it describes.
// Passing a nil querier is a programming error rather than a fallback: an
// event committed independently of its cause is exactly the split this
// outbox exists to prevent.
func (r *Repository) PublishMarketplaceEvent(ctx context.Context, q Querier, eventType string,
	aggregateType string, aggregateID string, aggregateVersion int64,
	correlationID string, causationID string, payload map[string]any) error {
	if q == nil {
		return fmt.Errorf("marketplace: event %s must be published inside the "+
			"transaction that produced it", eventType)
	}
	if err := validatePayload(eventType, payload); err != nil {
		return err
	}
	return r.PublishEvent(ctx, q, eventType, aggregateType, aggregateID,
		aggregateVersion, correlationID, causationID, payload)
}
