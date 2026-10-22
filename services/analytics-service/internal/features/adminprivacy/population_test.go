package adminprivacy

// Owner-declared unattested populations (analyticsadmin.UnattestedPopulations)
// and Dispatch's authoritative unmatched demand fact.

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"github.com/google/uuid"
)

func completeOrError(t *testing.T, f *reportFixture, q Query) (Snapshot, error) {
	t.Helper()
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	return completeSnapshot(context.Background(), tx, map[string]admincontext.Authority{f.key.authority.Owner: f.key.authority}, q.Metric, f.policyField(), q)
}

func (f *reportFixture) policyField() string {
	contract := f.policy.Document.Rules[0].SourceContract
	for i := len(contract) - 1; i >= 0; i-- {
		if contract[i] == ':' {
			return contract[i+1:]
		}
	}
	return ""
}

// notComplete proves that neither the source snapshot nor the disclosed
// release claims COMPLETE coverage.
func notComplete(t *testing.T, f *reportFixture) {
	t.Helper()
	if s, err := f.snapshot(t, f.q); err == nil && !s.Partial {
		t.Fatal("an unattested population produced a COMPLETE snapshot", s)
	}
	if release, err := f.service.Disclose(context.Background(), f.actor, f.q); err == nil && release.Coverage == "COMPLETE" {
		t.Fatal("an unattested population was released as COMPLETE", release)
	}
}

// publishHalves publishes the first half-window manifest (both=false, the
// window is not yet tiled) or then the second half (both=true, now tiled).
func publishHalves(t *testing.T, f *reportFixture, members []analyticsadmin.ManifestMember, both bool) {
	t.Helper()
	mid := f.q.Start.Add(30 * time.Minute)
	first, second := []analyticsadmin.ManifestMember{}, []analyticsadmin.ManifestMember{}
	for _, m := range members {
		if m.OccurredAt.Before(mid) {
			first = append(first, m)
		} else {
			second = append(second, m)
		}
	}
	if !both {
		f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, mid, 1, first)
		return
	}
	f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, mid, f.q.End, 1, second)
}

// Dispatch's unmatched demand fact is verified at ingestion, counted against
// its own booking, and the unmatched cell releases COMPLETE once the owner's
// signed manifests (built from per-booking signed contexts) tile the window.
func TestPostgresDispatchUnmatchedDemandReleasesCompleteUnderAttestedCoverage(t *testing.T) {
	const owner = "driver-dispatch-delivery-service"
	f := newReportFixture(t, "demand", owner, "DISPATCH", "DISPATCH_PARCEL", "source_entity_id")
	ctx := context.Background()
	members := []analyticsadmin.ManifestMember{}
	bookings := []string{}
	for i := 0; i < 6; i++ {
		booking := uuid.NewString()
		bookings = append(bookings, booking)
		members = append(members, ingestFact(t, f.db, owner, "dispatch.booking.committed.v1", "dispatch_booking", booking, 1, f.q.Start.Add(time.Duration(i*10+1)*time.Minute), map[string]any{"booking_id": booking, "service_tier": "standard", "product_code": "DISPATCH_PARCEL"}))
	}
	unmatched := ingestFact(t, f.db, owner, networkanalytics.EventDispatchBookingUnmatched, "dispatch_booking", bookings[5], 1, f.q.Start.Add(55*time.Minute), map[string]any{"booking_id": bookings[5], "service_tier": "standard", "product_code": "DISPATCH_PARCEL"})
	members = append(members, unmatched)

	var kind, authority, status string
	if err := f.db.QueryRow(ctx, `SELECT kind,authority,status FROM analytics_network_events WHERE event_id=$1`, unmatched.EventID).Scan(&kind, &authority, &status); err != nil {
		t.Fatal(err)
	}
	if kind != string(networkanalytics.KindMissionFailed) || authority != "DISPATCH" || status != "PROCESSED" {
		t.Fatal("unmatched fact not ingested as an authoritative Dispatch outcome", kind, authority, status)
	}
	tx, _ := f.db.Begin(ctx)
	reader := LineageReader{Owner: owner}
	unit, err := reader.ReadUnit(ctx, tx, "demand", "source_entity_id", unmatched.EventID)
	request, err2 := reader.ReadUnit(ctx, tx, "demand", "source_entity_id", members[5].EventID)
	tx.Rollback(ctx)
	if err != nil || err2 != nil || unit.SubjectHash != request.SubjectHash {
		t.Fatal("unmatched booking does not resolve to its own request subject", err, err2)
	}

	// Incomplete coverage: never COMPLETE.
	publishHalves(t, f, members, false)
	if _, err := completeOrError(t, f, f.q); !errors.Is(err, ErrCoverage) {
		t.Fatal("un-tiled window treated as complete", err)
	}
	notComplete(t, f)
	// Full attested coverage: the unmatched cell is computed and released.
	publishHalves(t, f, members, true)
	s, err := f.snapshot(t, f.q)
	if err != nil || s.Partial {
		t.Fatal(s, err)
	}
	if got := cellValue(t, s, "requests"); got == nil || *got != 6 {
		t.Fatal("requests", got)
	}
	if got := cellValue(t, s, "unmatched"); got == nil || *got != 1 {
		t.Fatal("unmatched", got)
	}
	release, err := f.service.Disclose(ctx, f.actor, f.q)
	if err != nil || release.Coverage != "COMPLETE" {
		t.Fatal(release, err)
	}
	for _, c := range release.Cells {
		if c.Key == "unmatched" && c.Value == nil && c.Suppression == "UNKNOWN" {
			t.Fatal("Dispatch unmatched cell still UNKNOWN", c)
		}
	}
}

// Every newly enrolled population releases COMPLETE once signed manifests
// tile the window, and is never COMPLETE before.
func TestPostgresEnrolledPopulationsReleaseCompleteOnlyWhenTiled(t *testing.T) {
	cases := []struct {
		name, report, owner, authority, product, field, event, entity, key string
		payload                                                            map[string]any
	}{
		{"dispatch commerce demand", "demand", "driver-dispatch-delivery-service", "DISPATCH", "DISPATCH_COMMERCE", "source_entity_id", "commerce.mission.created.v1", "COMMERCE_MISSION", "requests", map[string]any{}},
		{"dispatch commerce trip outcomes", "mission-outcomes", "driver-dispatch-delivery-service", "DISPATCH", "DISPATCH_COMMERCE", "source_entity_id", "trip.completed.v1", "TRIP", "completed", map[string]any{}},
		{"dispatch mission success over trips", "mission-success", "driver-dispatch-delivery-service", "DISPATCH", "DISPATCH_PARCEL", "provider_id", "trip.completed.v1", "TRIP", "completed", map[string]any{}},
		{"dispatch delivery-mission declines", "declines", "driver-dispatch-delivery-service", "DISPATCH", "DISPATCH_PARCEL", "provider_id", "dispatch.offer.declined.v1", "delivery_mission", "safety_declines", map[string]any{"reason_code": "UNSAFE_ROUTE"}},
		{"dispatch delivery-mission mismatches", "capability-mismatches", "driver-dispatch-delivery-service", "DISPATCH", "DISPATCH_PARCEL", "source_entity_id", "dispatch.capability_mismatch.v1", "delivery_mission", "mismatches", map[string]any{"reason_codes": "CAPACITY"}},
		{"taxi goods demand", "demand", "driver-taxi-service", "TAXI", "TAXI_GOODS", "source_entity_id", "taxi.goods.mission.committed.v1", "taxi_goods_mission", "requests", map[string]any{}},
		{"taxi goods outcomes", "mission-outcomes", "driver-taxi-service", "TAXI", "TAXI_GOODS", "source_entity_id", "taxi.goods.completed.v1", "taxi_goods_mission", "completed", map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newReportFixture(t, tc.report, tc.owner, tc.authority, tc.product, tc.field)
			members := []analyticsadmin.ManifestMember{}
			for i := 0; i < 6; i++ {
				payload := map[string]any{"provider_id": provider(i), "product_code": tc.product}
				for k, v := range tc.payload {
					payload[k] = v
				}
				members = append(members, ingestFact(t, f.db, tc.owner, tc.event, tc.entity, uuid.NewString(), 1, f.q.Start.Add(time.Duration(i*10+1)*time.Minute), payload))
			}
			publishHalves(t, f, members, false)
			if _, err := completeOrError(t, f, f.q); !errors.Is(err, ErrCoverage) {
				t.Fatal("un-tiled window treated as complete", err)
			}
			notComplete(t, f)
			publishHalves(t, f, members, true)
			s, err := f.snapshot(t, f.q)
			if err != nil || s.Partial {
				t.Fatal(s, err)
			}
			if got := cellValue(t, s, tc.key); got == nil || *got != 6 {
				t.Fatal(tc.key, got)
			}
		})
	}
}

// Anything still declared unattested is refused COMPLETE even with an exactly
// tiling signed manifest.
func TestPostgresDeclaredUnattestedPopulationStillRefusesComplete(t *testing.T) {
	restore := unattestedPopulations
	unattestedPopulations = func() []analyticsadmin.UnattestedPopulation {
		return []analyticsadmin.UnattestedPopulation{{Owner: "driver-hauling-service", Authority: "HAULING", ProductCode: "HAULAGE", EntityTypes: []string{"haulage_booking"}, Reason: "test declaration"}}
	}
	defer func() { unattestedPopulations = restore }()
	f := newReportFixture(t, "demand", "driver-hauling-service", "HAULING", "HAULAGE", "source_entity_id")
	members := []analyticsadmin.ManifestMember{}
	for i := 0; i < 6; i++ {
		members = append(members, ingestFact(t, f.db, "driver-hauling-service", "hauling.booking.committed.v1", "haulage_booking", uuid.NewString(), 1, f.q.Start.Add(time.Duration(i+1)*time.Minute), map[string]any{"product_code": "HAULAGE"}))
	}
	f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, members)
	if _, err := completeOrError(t, f, f.q); !errors.Is(err, ErrUnattestedPopulation) {
		t.Fatal("declared population not refused", err)
	}
	notComplete(t, f)
	// An empty window proves nothing about a declared population either.
	g := newReportFixture(t, "demand", "driver-hauling-service", "HAULING", "HAULAGE", "source_entity_id")
	g.publish(t, analyticsadmin.FamilyNetworkEvents, g.q.Scope, g.q.Start, g.q.End, 1, nil)
	if _, err := completeOrError(t, g, g.q); !errors.Is(err, ErrUnattestedPopulation) {
		t.Fatal("declared population complete on an empty window", err)
	}
}

// The gate is exact: attested populations with an authoritative unmatched
// outcome still release COMPLETE with the unmatched cell computed.
func TestPostgresAttestedUnmatchedDemandReleasesComplete(t *testing.T) {
	cases := []struct{ owner, authority, product, committed, unmatched, entity string }{
		{"driver-hauling-service", "HAULING", "HAULAGE", "hauling.booking.committed.v1", "hauling.booking.unmatched.v1", "haulage_booking"},
		{"driver-taxi-service", "TAXI", "TAXI_PASSENGER", "taxi.booking.created", "taxi.booking.unmatched", "booking"},
	}
	for _, tc := range cases {
		t.Run(tc.authority, func(t *testing.T) {
			f := newReportFixture(t, "demand", tc.owner, tc.authority, tc.product, "source_entity_id")
			members := []analyticsadmin.ManifestMember{}
			for i := 0; i < 7; i++ {
				booking := uuid.NewString()
				payload := map[string]any{"product_code": tc.product}
				members = append(members, ingestFact(t, f.db, tc.owner, tc.committed, tc.entity, booking, 1, f.q.Start.Add(time.Duration(i+1)*time.Minute), payload))
				if i < 2 {
					members = append(members, ingestFact(t, f.db, tc.owner, tc.unmatched, tc.entity, booking, 2, f.q.Start.Add(time.Duration(i+20)*time.Minute), payload))
				}
			}
			f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, members)
			s, err := f.snapshot(t, f.q)
			if err != nil || s.Partial {
				t.Fatal(s, err)
			}
			if got := cellValue(t, s, "requests"); got == nil || *got != 7 {
				t.Fatal("requests", got)
			}
			if got := cellValue(t, s, "unmatched"); got == nil || *got != 2 {
				t.Fatal("unmatched", got)
			}
		})
	}
}

// dispatchMissionFacts ingests, for n copies of each Dispatch flow, exactly the
// facts migration 000076 records (one per mission and kind; see the Dispatch
// test TestPostgresDispatchLifecycleFactsOncePerMissionAndKind).
func dispatchMissionFacts(t *testing.T, f *reportFixture, product string, n int) []analyticsadmin.ManifestMember {
	t.Helper()
	const owner = "driver-dispatch-delivery-service"
	members := []analyticsadmin.ManifestMember{}
	at := f.q.Start
	add := func(event, entity, id string, version int64) {
		at = at.Add(time.Second)
		members = append(members, ingestFact(t, f.db, owner, event, entity, id, version, at, map[string]any{"provider_id": provider(len(members) % 7), "product_code": product}))
	}
	for i := 0; i < n; i++ {
		if product == "DISPATCH_COMMERCE" {
			mission := "dcm_" + uuid.NewString()
			add("commerce.mission.created.v1", "COMMERCE_MISSION", mission, 1)
			add("dispatch.booking.assigned.v1", "COMMERCE_MISSION", mission, 1) // first trip of the mission; reassignment adds none
			add("trip.cancelled.v1", "TRIP", uuid.NewString(), 3)               // the replaced execution
			add("trip.completed.v1", "TRIP", uuid.NewString(), 4)
			continue
		}
		parcel, partner, unmatched, cancelled, late := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		add("dispatch.booking.committed.v1", "dispatch_booking", parcel, 1)
		add("dispatch.booking.assigned.v1", "dispatch_booking", parcel, 1)
		add("trip.completed.v1", "TRIP", uuid.NewString(), 5)
		add("dispatch.booking.committed.v1", "partner_request", partner, 1)
		add("dispatch.booking.assigned.v1", "partner_request", partner, 1)
		add("trip.completed.v1", "TRIP", uuid.NewString(), 6)
		add("dispatch.booking.committed.v1", "dispatch_booking", unmatched, 1)
		add(networkanalytics.EventDispatchBookingUnmatched, "dispatch_booking", unmatched, 1)
		add("dispatch.booking.committed.v1", "dispatch_booking", cancelled, 1)
		add(networkanalytics.EventDispatchBookingCancelled, "dispatch_booking", cancelled, 1)
		add("dispatch.booking.committed.v1", "dispatch_booking", late, 1)
		add("dispatch.booking.assigned.v1", "dispatch_booking", late, 1)
		add("trip.cancelled.v1", "TRIP", uuid.NewString(), 2)
	}
	return members
}

func releasedCells(t *testing.T, f *reportFixture, want map[string]int64, mustRelease bool) {
	t.Helper()
	s, err := f.snapshot(t, f.q)
	if err != nil || s.Partial {
		t.Fatal(s, err)
	}
	for key, value := range want {
		if got := cellValue(t, s, key); got == nil || *got != value {
			t.Fatal(f.q.Metric, key, got, value)
		}
	}
	release, err := f.service.Disclose(context.Background(), f.actor, f.q)
	if err != nil || release.Coverage != "COMPLETE" {
		t.Fatal(release, err)
	}
	released := 0
	for _, c := range release.Cells {
		value, ok := want[c.Key]
		if !ok {
			continue
		}
		if c.Suppression == "UNKNOWN" {
			t.Fatal("lifecycle cell UNKNOWN under complete coverage", c)
		}
		if c.Value == nil {
			continue
		}
		// Released values are the computed counts after the policy's rounding.
		if *c.Value != value-value%10 {
			t.Fatal("released", c.Key, *c.Value, "from", value)
		}
		released++
	}
	if mustRelease && released != len(want) {
		t.Fatal("expected every cell released", release.Cells)
	}
}

// Under full attested coverage each Dispatch mission counts once per kind in
// the released mission-outcomes and demand cells: parcel bookings, partner
// requests, unmatched and cancelled bookings, and commerce missions (with
// reassignment).
func TestPostgresDispatchMissionOutcomesCountEachMissionOncePerKind(t *testing.T) {
	const owner = "driver-dispatch-delivery-service"
	for _, tc := range []struct {
		report, product string
		want            map[string]int64
	}{
		{"mission-outcomes", "DISPATCH_PARCEL", map[string]int64{"committed": 50, "assigned": 30, "completed": 20, "failed": 10, "cancelled": 20}},
		{"demand", "DISPATCH_PARCEL", map[string]int64{"requests": 50, "unmatched": 10}},
		{"mission-success", "DISPATCH_PARCEL", map[string]int64{"completed": 20, "failed": 10, "cancelled": 20}},
		{"mission-outcomes", "DISPATCH_COMMERCE", map[string]int64{"committed": 10, "assigned": 10, "completed": 10, "failed": 0, "cancelled": 10}},
	} {
		t.Run(tc.report+"/"+tc.product, func(t *testing.T) {
			f := newReportFixture(t, tc.report, owner, "DISPATCH", tc.product, "source_entity_id")
			members := dispatchMissionFacts(t, f, tc.product, 10)
			f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, members)
			// Dispatch records no recovery-resolution or guarantee facts, so
			// those mission-outcomes cells are structurally zero; the
			// policy's small-cell rule treats a zero cohort as small and
			// complementarily suppresses the rest. The computed snapshot
			// cells above are what such a release is built from.
			releasedCells(t, f, tc.want, tc.report == "demand")
		})
	}
}

// dispatchRecoveryFacts ingests the Dispatch recovery facts for n incident
// recoveries resolved after 60s, n/2 incident recoveries that failed, and n
// delivery recoveries resolved after 120s (migration 000077; starts from the
// existing selected/replacement events and the new incident start).
func dispatchRecoveryFacts(t *testing.T, f *reportFixture, n int) []analyticsadmin.ManifestMember {
	t.Helper()
	const owner = "driver-dispatch-delivery-service"
	members := []analyticsadmin.ManifestMember{}
	add := func(event, entity, id string, version int64, at time.Time, payload map[string]any) {
		payload["product_code"] = "DISPATCH_PARCEL"
		payload["provider_id"] = provider(len(members) % 7)
		members = append(members, ingestFact(t, f.db, owner, event, entity, id, version, at, payload))
	}
	base := f.q.Start.Add(time.Minute)
	for i := 0; i < n; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		incident, trip := uuid.NewString(), uuid.NewString()
		add("dispatch.incident_recovery.started.v1", "delivery_incident", incident, 1, at, map[string]any{"incident_id": incident})
		add("dispatch.provider_replacement.started.v1", "trip", trip, 2, at, map[string]any{"incident_id": incident})
		add("dispatch.incident_recovery.resolved.v1", "delivery_incident", incident, 1, at.Add(60*time.Second), map[string]any{"incident_id": incident})
		action, actionTrip := uuid.NewString(), uuid.NewString()
		add("dispatch.delivery_recovery.selected.v1", "trip", actionTrip, 3, at, map[string]any{"recovery_action_id": action})
		add("dispatch.delivery_recovery.resolved.v1", "delivery_recovery_action", action, 1, at.Add(120*time.Second), map[string]any{"recovery_action_id": action})
		if i%2 == 0 {
			failed, failedTrip := uuid.NewString(), uuid.NewString()
			add("dispatch.incident_recovery.started.v1", "delivery_incident", failed, 1, at, map[string]any{"incident_id": failed})
			add("dispatch.provider_replacement.started.v1", "trip", failedTrip, 2, at, map[string]any{"incident_id": failed})
			add("dispatch.incident_recovery.failed.v1", "delivery_incident", failed, 1, at.Add(30*time.Second), map[string]any{"incident_id": failed})
		}
	}
	return members
}

// Dispatch recovery outcomes count once per recovery in mission-outcomes and
// the recovery report, and recovery-resolution times each resolved recovery
// from its own attested start (incident or delivery recovery), failed ones
// excluded.
func TestPostgresDispatchRecoveryOutcomesAndResolutionDurations(t *testing.T) {
	const owner = "driver-dispatch-delivery-service"
	for _, tc := range []struct {
		report string
		want   map[string]int64
	}{
		{"mission-outcomes", map[string]int64{"recovery_resolved": 20, "recovery_failed": 5, "guarantee_breaches": 0}},
		{"recovery", map[string]int64{"started": 25, "replaced": 15, "resolved": 20, "failed": 5}},
		{"recovery-resolution", map[string]int64{"resolved_with_duration": 20, "resolution_seconds_mean": 90, "resolution_seconds_median": 90}},
	} {
		t.Run(tc.report, func(t *testing.T) {
			f := newReportFixture(t, tc.report, owner, "DISPATCH", "DISPATCH_PARCEL", "source_entity_id")
			members := dispatchRecoveryFacts(t, f, 10)
			f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, members)
			// mission-outcomes keeps a structural zero (Dispatch has no
			// guarantee concept), so its release stays small-cell suppressed;
			// the recovery reports have no zero cell and release every value.
			releasedCells(t, f, tc.want, tc.report != "mission-outcomes")
		})
	}
}
