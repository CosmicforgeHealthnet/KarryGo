package networkanalytics

import (
	"encoding/json"
	"time"

	sharedevents "cosmicforge/logistics/shared/go/events"
)

// WithVanishedZones adds an explicit zero for every zone and product that had
// supply last bucket and has none now. Without it a zone whose last provider
// went offline would keep showing its old count until it aged out -- stale
// supply, not zero supply. A zero here is observed, so its counts are known.
func WithVanishedZones(previous, current []SupplyFact, now time.Time) []SupplyFact {
	present := map[string]bool{}
	for _, fact := range current {
		present[fact.EntityID()] = true
	}
	out := append([]SupplyFact(nil), current...)
	bucket := BucketStart(now)
	for _, fact := range previous {
		if present[fact.EntityID()] || fact.OnlineCount == 0 {
			continue
		}
		eligible, occupied := int64(0), int64(0)
		out = append(out, SupplyFact{Authority: fact.Authority, MarketID: fact.MarketID, Zone: fact.Zone,
			ProductCode: fact.ProductCode, OnlineCount: 0, EligibleCount: &eligible, OccupiedCount: &occupied,
			State: SupplyLimited, ObservedAt: now.UTC(), BucketStart: bucket, PolicyVersion: fact.PolicyVersion})
	}
	return out
}

// SupplyEnvelope is the canonical envelope for a supply fact.
func SupplyEnvelope(sourceService string, fact SupplyFact) (sharedevents.Envelope, error) {
	payload, err := json.Marshal(fact)
	if err != nil {
		return sharedevents.Envelope{}, err
	}
	return sharedevents.Envelope{EventID: fact.EventID(), EventName: EventSupplySnapshot, SchemaVersion: 1,
		SourceService: sourceService, SourceEntityType: "network_supply", SourceEntityID: fact.EntityID(),
		SourceVersion: fact.Version(), OccurredAt: fact.ObservedAt, Payload: payload}, nil
}

// DemandEnvelope is the canonical envelope for a demand bucket.
func DemandEnvelope(sourceService string, bucket DemandBucket) (sharedevents.Envelope, error) {
	payload, err := json.Marshal(bucket)
	if err != nil {
		return sharedevents.Envelope{}, err
	}
	return sharedevents.Envelope{EventID: bucket.EventID(), EventName: EventDemandBucket, SchemaVersion: 1,
		SourceService: sourceService, SourceEntityType: "network_demand", SourceEntityID: bucket.EntityID(),
		SourceVersion: bucket.Version(), OccurredAt: bucket.ObservedAt, Payload: payload}, nil
}
