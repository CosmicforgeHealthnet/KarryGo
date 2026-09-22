package networkanalytics

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var observedAt = time.Date(2026, 9, 22, 10, 7, 30, 0, time.UTC)

func TestUnknownIsNotZero(t *testing.T) {
	facts := BuildSupplyFacts(AuthorityDispatch, "", []SupplyObservation{
		{Zone: "gh5:s1", ProductCode: "dispatch_parcel", Online: true, Occupied: Bool(false), Eligible: nil},
		{Zone: "gh5:s2", ProductCode: "DISPATCH_PARCEL", Online: true, Occupied: Bool(false), Eligible: Bool(false)},
		{Zone: "gh5:s3", ProductCode: "DISPATCH_PARCEL", Online: true, Occupied: Bool(false), Eligible: Bool(true)},
		{Zone: "gh5:s3", ProductCode: "DISPATCH_PARCEL", Online: false, Eligible: Bool(true)},
	}, observedAt, "p1")
	if len(facts) != 3 {
		t.Fatalf("facts %+v", facts)
	}
	byZone := map[string]SupplyFact{}
	for _, fact := range facts {
		byZone[fact.Zone] = fact
	}
	if byZone["gh5:s1"].EligibleCount != nil || byZone["gh5:s1"].State != SupplyUnknown {
		t.Fatal("unknown eligibility must stay unknown")
	}
	if byZone["gh5:s2"].EligibleCount == nil || *byZone["gh5:s2"].EligibleCount != 0 || byZone["gh5:s2"].State != SupplyLimited {
		t.Fatal("known zero eligibility must be zero and limited")
	}
	if byZone["gh5:s3"].OnlineCount != 1 || byZone["gh5:s3"].State != SupplyAvailable {
		t.Fatal("an offline provider is not supply")
	}
	raw, _ := json.Marshal(byZone["gh5:s1"])
	if !strings.Contains(string(raw), `"eligible_count":null`) {
		t.Fatalf("unknown must marshal as null: %s", raw)
	}
}

func TestSupplyFactsCarryNoIdentityOrCoordinates(t *testing.T) {
	fact := BuildSupplyFacts(AuthorityHauling, "", []SupplyObservation{
		{Zone: ZoneFor(6.524379, 3.379206), ProductCode: "HAULAGE", Online: true, Occupied: Bool(true), Eligible: Bool(true)},
	}, observedAt, "")[0]
	raw, _ := json.Marshal(fact)
	for _, leak := range []string{"6.52", "3.37", "provider", "vehicle", "truck_id", "customer"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("supply fact leaks %q: %s", leak, raw)
		}
	}
	if !strings.HasPrefix(fact.Zone, "gh5:") || len(fact.Zone) != 9 {
		t.Fatalf("zone %q is not a precision-5 cell", fact.Zone)
	}
	if ZoneFor(0, 0) != UnlocatedZone {
		t.Fatal("no fix must be unlocated, not a cell at null island")
	}
}

func TestReplicasPublishTheSameFact(t *testing.T) {
	observations := []SupplyObservation{{Zone: "gh5:a", ProductCode: "X", Online: true, Occupied: Bool(false), Eligible: Bool(true)}}
	a := BuildSupplyFacts(AuthorityTaxi, "m1", observations, observedAt, "")[0]
	b := BuildSupplyFacts(AuthorityTaxi, "m1", observations, observedAt.Add(2*time.Minute), "")[0]
	if a.EventID() != b.EventID() || a.Version() != b.Version() {
		t.Fatal("two observations in one bucket must be one fact")
	}
	c := BuildSupplyFacts(AuthorityTaxi, "m1", observations, observedAt.Add(6*time.Minute), "")[0]
	if c.Version() <= a.Version() || c.EntityID() != a.EntityID() {
		t.Fatal("the next bucket must supersede, under the same entity")
	}
	envelope, err := SupplyEnvelope("driver-taxi-service", c)
	if err != nil || envelope.Validate() != nil {
		t.Fatalf("envelope %+v err=%v", envelope, err)
	}
}

func TestAVanishedZoneIsAnObservedZero(t *testing.T) {
	previous := BuildSupplyFacts(AuthorityDispatch, "", []SupplyObservation{
		{Zone: "gh5:a", ProductCode: "P", Online: true, Occupied: Bool(false), Eligible: Bool(true)}}, observedAt, "")
	current := WithVanishedZones(previous, nil, observedAt.Add(5*time.Minute))
	if len(current) != 1 || current[0].OnlineCount != 0 || current[0].EligibleCount == nil || *current[0].EligibleCount != 0 {
		t.Fatalf("vanished zone %+v", current)
	}
	if again := WithVanishedZones(current, nil, observedAt.Add(10*time.Minute)); len(again) != 0 {
		t.Fatal("a zero is not repeated forever")
	}
}

func TestDemandBucketsCountAndSupersede(t *testing.T) {
	start := DemandBucketStart(observedAt)
	observations := []DemandObservation{
		{Zone: "tz:1", ProductCode: "TAXI_PASSENGER", CommittedAt: start.Add(time.Minute)},
		{Zone: "tz:1", ProductCode: "TAXI_PASSENGER", CommittedAt: start.Add(2 * time.Minute), Unmatched: true},
		{Zone: "tz:1", ProductCode: "TAXI_PASSENGER", CommittedAt: start.Add(-time.Minute)},
	}
	buckets := BuildDemandBuckets(AuthorityTaxi, observations, observedAt)
	if len(buckets) != 2 {
		t.Fatalf("buckets %+v", buckets)
	}
	var current DemandBucket
	for _, bucket := range buckets {
		if bucket.BucketStart.Equal(start) {
			current = bucket
		}
	}
	if current.DemandCount != 2 || current.UnmatchedCount != 1 || current.BucketMinutes != 15 {
		t.Fatalf("current bucket %+v", current)
	}
	later := BuildDemandBuckets(AuthorityTaxi, observations, observedAt.Add(time.Minute))
	if later[0].EntityID() != buckets[0].EntityID() || later[0].Version() <= buckets[0].Version() {
		t.Fatal("a later observation of a bucket must supersede it")
	}
	if !DemandWindowStart(observedAt).Equal(start.Add(-DemandBucketSize)) {
		t.Fatal("the window must include the bucket just closed")
	}
	envelope, err := DemandEnvelope("driver-taxi-service", current)
	if err != nil || envelope.Validate() != nil {
		t.Fatalf("envelope %+v err=%v", envelope, err)
	}
	if _, kind, ok := Classify(EventDemandBucket, "TAXI"); !ok || kind != KindDemandBucket {
		t.Fatal("a demand bucket must classify from its payload authority")
	}
}

func TestLifecycleProductsStayDistinguishable(t *testing.T) {
	cases := map[string]string{
		"taxi.booking.completed":       ProductTaxiPassenger,
		"taxi.goods.completed.v1":      ProductTaxiGoods,
		"hauling.booking.completed.v1": ProductHaulage,
		"trip.completed.v1":            ProductDispatchParcel,
		"commerce.mission.created.v1":  ProductDispatchCommerce,
	}
	for name, want := range cases {
		if got := ProductFor(name, "", nil); got != want {
			t.Errorf("%s -> %s, want %s", name, got, want)
		}
	}
	if ProductFor("taxi.recovery.resolved.v1", "taxi_goods_mission", nil) != ProductTaxiGoods {
		t.Error("a goods recovery must be a goods product")
	}
	if ProductFor("trip.completed.v1", "", map[string]any{"product_code": "express"}) != "EXPRESS" {
		t.Error("an explicit product code wins")
	}
	for _, name := range []string{"taxi.booking.created", "hauling.booking.committed.v1", "dispatch.booking.committed.v1"} {
		if _, kind, ok := Classify(name, ""); !ok || kind != KindMissionCommitted {
			t.Errorf("%s must classify as committed", name)
		}
	}
}
