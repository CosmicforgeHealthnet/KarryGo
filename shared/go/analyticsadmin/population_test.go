package analyticsadmin

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

func lineageKinds(names ...string) []networkanalytics.Kind { return kindsOf(names...) }

// Every population is anchored to an enrolled kind, so the declaration is
// empty; the rule it drives is still exact for any declared population.
func TestUnattestedPopulationDeclarationIsClosedAndExact(t *testing.T) {
	if got := UnattestedPopulations(); len(got) != 0 {
		t.Fatal("a population is declared unattested although its kind is enrolled", got)
	}
	if UnattestedEntityType("driver-dispatch-delivery-service", "dispatch_booking") || UnattestedEntityType("driver-taxi-service", "taxi_goods_mission") {
		t.Fatal("enrolled population treated as unattested")
	}
	declared := []UnattestedPopulation{
		{Owner: "driver-dispatch-delivery-service", Authority: "DISPATCH", ProductCode: "DISPATCH_PARCEL", EntityTypes: []string{"dispatch_booking"}, Kinds: lineageKinds("dispatch.booking.committed.v1", networkanalytics.EventDispatchBookingUnmatched), Reason: "test"},
		{Owner: "driver-taxi-service", Authority: "TAXI", ProductCode: "TAXI_GOODS", EntityTypes: []string{"taxi_goods_mission"}, Reason: "test"},
	}
	demand := lineageKinds("dispatch.booking.committed.v1")
	recovery := lineageKinds("dispatch.delivery_recovery.selected.v1")
	cases := []struct {
		authority, product string
		kinds              []networkanalytics.Kind
		want               bool
	}{
		{"DISPATCH", "DISPATCH_PARCEL", demand, false},
		{"DISPATCH", "DISPATCH_PARCEL", recovery, true},
		{"DISPATCH", "DISPATCH_COMMERCE", demand, true},
		{"TAXI", "TAXI_GOODS", recovery, false},
		{"TAXI", "TAXI_PASSENGER", demand, true},
	}
	for _, tc := range cases {
		if got := PopulationAttestedUnder(declared, tc.authority, tc.product, tc.kinds); got != tc.want {
			t.Error(tc.authority, tc.product, tc.kinds, got)
		}
	}
	if !PopulationAttested("DISPATCH", "DISPATCH_PARCEL", demand) || !PopulationAttested("TAXI", "TAXI_GOODS", recovery) {
		t.Fatal("enrolled populations refused")
	}
}

// A producer counts declared unattested facts explicitly and attests none of
// them; an unanchored fact that is NOT declared stops the run rather than
// silently leaving a population out of every scope.
func TestPostgresProducerCountsDeclaredUnattestedAndRefusesUndeclared(t *testing.T) {
	db := producerDB(t)
	ctx := context.Background()
	owner := "driver-dispatch-delivery-service"
	restore := declaredPopulations
	declaredPopulations = func() []UnattestedPopulation {
		return []UnattestedPopulation{{Owner: owner, Authority: "DISPATCH", ProductCode: "DISPATCH_PARCEL", EntityTypes: []string{"dispatch_booking"}, Reason: "test"}}
	}
	defer func() { declaredPopulations = restore }()
	signer, authority := testSigner(t, owner)
	now := time.Now().UTC().Truncate(time.Hour).Add(30 * time.Minute)
	window := now.Truncate(time.Hour).Add(-time.Hour)
	members := testMembers(window, 4)
	facts := staticFacts{}
	for i, m := range members {
		if i < 2 {
			m.EventName, m.EntityType = "trip.completed.v1", "TRIP"
			facts = append(facts, ProducerFact{Member: m, Family: FamilyNetworkEvents, AnchorKind: "missions", AnchorRef: "anchored"})
			continue
		}
		m.EventName, m.EntityType = networkanalytics.EventDispatchBookingUnmatched, "dispatch_booking"
		facts = append(facts, ProducerFact{Member: m, Family: FamilyNetworkEvents})
	}
	publisher := &capturePublisher{}
	p := &Producer{Owner: owner, Environment: adminpolicy.TestOnly, DB: db, Source: facts, Resolver: staticResolver{"anchored": {testScope}}, AnchorKinds: []string{"missions"}, Signer: signer, Publisher: publisher, Period: time.Hour, Lookback: 2 * time.Hour, Settle: 10 * time.Minute, TTL: 48 * time.Hour, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", Now: func() time.Time { return now }}
	result, err := p.RunOnce(ctx)
	if err != nil || result.Unattested != 2 || result.Unscoped != 0 {
		t.Fatal(result, err)
	}
	for _, body := range publisher.bodies {
		v, err := VerifyManifest(authority, body, now)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range v.Members {
			if m.EntityType != "TRIP" {
				t.Fatal("unattested fact attested into a scope", m)
			}
		}
		if v.Start.Equal(window) && len(v.Members) != 2 {
			t.Fatal("anchored facts missing", v.Members)
		}
	}
	undeclared := testMembers(window, 1)[0]
	undeclared.EntityType = "route_alert"
	p.Source = append(facts, ProducerFact{Member: undeclared, Family: FamilyNetworkEvents})
	if _, err := p.RunOnce(ctx); !errors.Is(err, ErrInvalid) {
		t.Fatal("undeclared unanchored fact silently dropped", err)
	}
}
