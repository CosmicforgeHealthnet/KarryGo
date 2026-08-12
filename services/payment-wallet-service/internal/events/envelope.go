// Package events implements the transactional outbox and the financial event
// envelope for payment-wallet-service.
//
// Every financial event carries a globally unique event ID, a versioned event
// type, aggregate identity and version, correlation and causation IDs and the
// producing service. Consumers use the aggregate version to reject
// out-of-order deliveries and to detect gaps.
package events

import (
	"encoding/json"
	"fmt"
	"time"

	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/ids"
)

// ServiceName identifies this producer on the wire.
const ServiceName = "payment-wallet-service"

// Envelope is the wire header of every financial event.
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

// NewEnvelope builds a validated financial event envelope. platformScoped must
// be true for the documented platform-wide events; otherwise an organization
// ID is mandatory.
func NewEnvelope(eventType string, schemaVersion int, aggregateType string, aggregateID string,
	aggregateVersion int64, organizationID string, correlationID string, causationID string,
	payload any, platformScoped bool) (Envelope, error) {
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
		CausationID:      causationID,
		OccurredAt:       time.Now().UTC(),
		Producer:         ServiceName,
		Payload:          raw,
	}, nil
}

// Financial event types published by this service. Fleet consumes these.
const (
	EventWalletCreated          = "finance.wallet.created.v1"
	EventEarningAccountCreated  = "finance.earning_account.created.v1"
	EventInvoiceCreated         = "finance.invoice.created.v1"
	EventInvoicePaid            = "finance.invoice.paid.v1"
	EventInvoicePaymentFailed   = "finance.invoice.payment_failed.v1"
	EventSubscriptionActivated  = "finance.subscription.activated.v1"
	EventSubscriptionPastDue    = "finance.subscription.past_due.v1"
	EventSubscriptionSuspended  = "finance.subscription.suspended.v1"
	EventHoldCreated            = "finance.hold.created.v1"
	EventHoldReleased           = "finance.hold.released.v1"
	EventCommissionPosted       = "finance.commission.posted.v1"
	EventEarningPosted          = "finance.earning.posted.v1"
	EventSettlementProcessing   = "finance.settlement.processing.v1"
	EventSettlementSucceeded    = "finance.settlement.succeeded.v1"
	EventSettlementFailed       = "finance.settlement.failed.v1"
	EventRefundProcessing       = "finance.refund.processing.v1"
	EventRefundCompleted        = "finance.refund.completed.v1"
	EventRefundFailed           = "finance.refund.failed.v1"
	EventReversalCompleted      = "finance.reversal.completed.v1"
	EventReconciliationRequired = "finance.reconciliation.required.v1"
)
