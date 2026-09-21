package outbox

// Section 13: Taxi's network facts delivered to analytics.

import (
	"context"

	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

// TargetAnalytics is the outbox target for analytics copies, made by
// migration 019's trigger.
const TargetAnalytics = "analytics-service"

// AnalyticsEnvelopeDeliverer posts one canonical envelope.
type AnalyticsEnvelopeDeliverer interface {
	Deliver(ctx context.Context, envelope sharedevents.Envelope) error
}

// AnalyticsDestination delivers analytics copies as canonical envelopes.
type AnalyticsDestination struct {
	Deliverer AnalyticsEnvelopeDeliverer
	Enabled   bool
}

// Deliver sends one record. Without a configured analytics the row is
// dead-lettered, visibly and replayably, rather than dropped: analytics is
// advisory, so losing it harms nothing, but an operator enabling it later can
// requeue what was missed.
func (d AnalyticsDestination) Deliver(ctx context.Context, record Record) error {
	if !d.Enabled || d.Deliverer == nil {
		return &TerminalDeliveryError{Code: "analytics_not_configured"}
	}
	envelope := AnalyticsEnvelope(record)
	if err := envelope.Validate(); err != nil {
		return &TerminalDeliveryError{Code: "invalid_analytics_envelope", Err: err}
	}
	return d.Deliverer.Deliver(ctx, envelope)
}

// AnalyticsEnvelope builds the envelope entirely from the stored row.
//
// taxi_outbox has no per-aggregate version, so the row's own timestamp in
// microseconds stands in: it is fixed at insert, identical on every
// redelivery of the row, and increases for later facts about the same
// booking -- the three properties analytics needs from a version.
func AnalyticsEnvelope(record Record) sharedevents.Envelope {
	aggregateType := record.AggregateType
	if aggregateType == "" {
		aggregateType = "booking"
	}
	version := record.OccurredAt.UTC().UnixMicro()
	if version < 1 {
		version = 1
	}
	return sharedevents.Envelope{
		EventID: record.ID, EventName: record.EventType, SchemaVersion: 1,
		SourceService: "driver-taxi-service", SourceEntityType: aggregateType,
		SourceEntityID: record.AggregateID, SourceVersion: version,
		OccurredAt: record.OccurredAt.UTC(), Payload: record.Payload,
	}
}

// AnalyticsEventNames are the Taxi events migration 019 copies to analytics.
// A test holds this list and the trigger's list to the shared classification.
var AnalyticsEventNames = func() []string {
	var names []string
	for _, name := range networkanalytics.KnownEventNames() {
		if authority, _, ok := networkanalytics.Classify(name, ""); ok && authority == networkanalytics.AuthorityTaxi {
			names = append(names, name)
		}
	}
	return names
}()
