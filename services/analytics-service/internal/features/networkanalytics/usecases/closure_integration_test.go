package usecases

// Section 13 closure: supply by product, demand buckets and forecast, and
// mission outcomes by product -- all fed by the facts the authorities publish.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

func (h *harness) publish(t *testing.T, envelope sharedevents.Envelope, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return h.ingest(t, envelope)
}

func supplyFrom(authority, service string, at time.Time, observations []networkanalytics.SupplyObservation) []networkanalytics.SupplyFact {
	return networkanalytics.BuildSupplyFacts(authority, "", observations, at, "policy-1")
}

func free(zone, product string) networkanalytics.SupplyObservation {
	return networkanalytics.SupplyObservation{Zone: zone, ProductCode: product, Online: true,
		Occupied: networkanalytics.Bool(false), Eligible: networkanalytics.Bool(true)}
}

// Each authority's published supply lands in analytics under its own name.
func TestEveryAuthoritysSupplyPopulatesAnalytics(t *testing.T) {
	h := newHarness(t)
	sources := map[string]string{
		networkanalytics.AuthorityDispatch: "driver-dispatch-delivery-service",
		networkanalytics.AuthorityTaxi:     "driver-taxi-service",
		networkanalytics.AuthorityHauling:  "driver-hauling-service",
	}
	for authority, service := range sources {
		for _, fact := range supplyFrom(authority, service, h.now, []networkanalytics.SupplyObservation{
			free("gh5:s0000", "P1"), free("gh5:s0000", "P1")}) {
			envelope, err := networkanalytics.SupplyEnvelope(service, fact)
			if status := h.publish(t, envelope, err); status != "PROCESSED" {
				t.Fatalf("%s supply status %s", authority, status)
			}
		}
		answer, err := h.service.SupplySnapshotFor(context.Background(), authority, "gh5:s0000", "p1")
		if err != nil || !answer.Available || answer.OnlineCount != 2 || answer.EligibleCount == nil ||
			*answer.EligibleCount != 2 || answer.SupplyState != networkanalytics.SupplyAvailable {
			t.Fatalf("%s supply %+v err=%v", authority, answer, err)
		}
	}
}

func TestAStaleSupplyFactCannotReplaceANewerOne(t *testing.T) {
	h := newHarness(t)
	service := "driver-dispatch-delivery-service"
	newer := supplyFrom("DISPATCH", service, h.now, []networkanalytics.SupplyObservation{
		free("gh5:z1", "P"), free("gh5:z1", "P"), free("gh5:z1", "P")})[0]
	older := supplyFrom("DISPATCH", service, h.now.Add(-10*time.Minute), []networkanalytics.SupplyObservation{
		free("gh5:z1", "P")})[0]
	envelope, err := networkanalytics.SupplyEnvelope(service, newer)
	h.publish(t, envelope, err)
	envelope, err = networkanalytics.SupplyEnvelope(service, older)
	if status := h.publish(t, envelope, err); status != "STALE" {
		t.Fatalf("an older bucket must be stale, got %s", status)
	}
	answer, _ := h.service.SupplySnapshotFor(context.Background(), "DISPATCH", "gh5:z1", "")
	if answer.OnlineCount != 3 {
		t.Fatalf("a stale fact replaced a newer one: online=%d", answer.OnlineCount)
	}
}

func TestUnknownSupplyIsNotZeroSupply(t *testing.T) {
	h := newHarness(t)
	service := "driver-hauling-service"
	unknown := supplyFrom("HAULING", service, h.now, []networkanalytics.SupplyObservation{
		{Zone: "gh5:u1", ProductCode: "HAULAGE", Online: true, Occupied: nil, Eligible: nil}})[0]
	zero := supplyFrom("HAULING", service, h.now, []networkanalytics.SupplyObservation{
		{Zone: "gh5:u2", ProductCode: "HAULAGE", Online: true, Occupied: networkanalytics.Bool(true),
			Eligible: networkanalytics.Bool(false)}})[0]
	for _, fact := range []networkanalytics.SupplyFact{unknown, zero} {
		envelope, err := networkanalytics.SupplyEnvelope(service, fact)
		h.publish(t, envelope, err)
	}
	u, _ := h.service.SupplySnapshotFor(context.Background(), "HAULING", "gh5:u1", "")
	z, _ := h.service.SupplySnapshotFor(context.Background(), "HAULING", "gh5:u2", "")
	if u.EligibleCount != nil || u.OccupiedCount != nil || u.SupplyState != networkanalytics.SupplyUnknown || u.Available {
		t.Fatalf("unknown supply was read as known: %+v", u)
	}
	if z.EligibleCount == nil || *z.EligibleCount != 0 || !z.Available || z.Score != 0 {
		t.Fatalf("known zero supply was not zero: %+v", z)
	}
	var nulls int
	if err := h.pool.QueryRow(context.Background(), `SELECT count(*) FROM supply_product_snapshots
		WHERE zone='gh5:u1' AND eligible_count IS NULL AND occupied_count IS NULL`).Scan(&nulls); err != nil || nulls != 1 {
		t.Fatalf("unknown counts must be stored as NULL (%d, %v)", nulls, err)
	}
}

// A provider or vehicle id placed in a supply payload never reaches storage.
func TestSupplyAnalyticsHoldNoIdentityOrCoordinates(t *testing.T) {
	h := newHarness(t)
	fact := supplyFrom("TAXI", "driver-taxi-service", h.now, []networkanalytics.SupplyObservation{free("gh5:p1", "TAXI_PASSENGER")})[0]
	envelope, err := networkanalytics.SupplyEnvelope("driver-taxi-service", fact)
	if err != nil {
		t.Fatal(err)
	}
	// A careless producer adding identities and a position to the payload.
	envelope.Payload = []byte(`{"authority":"TAXI","zone":"gh5:p1","product_code":"TAXI_PASSENGER","online_count":1,
		"eligible_count":1,"occupied_count":0,"state":"AVAILABLE","bucket_start":"` +
		fact.BucketStart.Format(time.RFC3339) + `","provider_name":"Ada","driver_lat":6.5,"driver_lng":3.3,
		"vehicle_plate_note":"x"}`)
	h.ingest(t, envelope)
	var stored string
	if err = h.pool.QueryRow(context.Background(), `SELECT payload::text FROM analytics_network_events
		WHERE event_name=$1`, networkanalytics.EventSupplySnapshot).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"Ada", "6.5", "3.3", "driver_lat", "provider_name"} {
		if contains(stored, leak) {
			t.Fatalf("supply analytics stored %q: %s", leak, stored)
		}
	}
	var columns int
	if err = h.pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns
		WHERE table_name='supply_product_snapshots' AND (column_name LIKE '%provider%' OR column_name LIKE '%vehicle%'
		OR column_name LIKE '%lat%' OR column_name LIKE '%lng%')`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("the supply read model has identity or coordinate columns (%d)", columns)
	}
}

func demandFrom(at time.Time, count int, unmatched int) networkanalytics.DemandBucket {
	var observations []networkanalytics.DemandObservation
	for i := 0; i < count; i++ {
		observations = append(observations, networkanalytics.DemandObservation{Zone: "tz:z1",
			ProductCode: "TAXI_PASSENGER", CommittedAt: networkanalytics.DemandBucketStart(at).Add(time.Minute),
			Unmatched: i < unmatched})
	}
	return networkanalytics.BuildDemandBuckets("TAXI", observations, at)[0]
}

func TestDemandBucketsPopulateCountOnceAndNeverRegress(t *testing.T) {
	h := newHarness(t)
	// Well inside one bucket, so the minute either side stays in it.
	h.now = networkanalytics.DemandBucketStart(h.now).Add(5 * time.Minute)
	service := "driver-taxi-service"
	first := demandFrom(h.now, 3, 1)
	envelope, err := networkanalytics.DemandEnvelope(service, first)
	if status := h.publish(t, envelope, err); status != "PROCESSED" {
		t.Fatalf("demand status %s", status)
	}
	if status := h.publish(t, envelope, err); status != "DUPLICATE" {
		t.Fatalf("a replayed bucket must be a duplicate, got %s", status)
	}
	grown := demandFrom(h.now.Add(time.Minute), 5, 1)
	envelope, err = networkanalytics.DemandEnvelope(service, grown)
	h.publish(t, envelope, err)
	stale := demandFrom(h.now.Add(-time.Minute), 2, 0)
	stale.BucketStart = first.BucketStart
	envelope, err = networkanalytics.DemandEnvelope(service, stale)
	if status := h.publish(t, envelope, err); status != "STALE" {
		t.Fatalf("an older observation must be stale, got %s", status)
	}
	var count, unmatched int64
	if err = h.pool.QueryRow(context.Background(), `SELECT demand_count, unmatched_count FROM demand_buckets`).
		Scan(&count, &unmatched); err != nil {
		t.Fatal(err)
	}
	if count != 5 || unmatched != 1 {
		t.Fatalf("bucket count=%d unmatched=%d, want 5/1", count, unmatched)
	}
}

func TestTheDemandForecastIsDeterministicAndHonestAboutThinData(t *testing.T) {
	h := newHarness(t)
	h.now = networkanalytics.DemandBucketStart(h.now).Add(5 * time.Minute)
	service := "driver-taxi-service"
	publish := func(at time.Time, count int) {
		envelope, err := networkanalytics.DemandEnvelope(service, demandFrom(at, count, 0))
		h.publish(t, envelope, err)
	}
	publish(h.now, 4)                   // the current bucket so far
	publish(h.now.AddDate(0, 0, -7), 6) // one week ago
	publish(h.now.AddDate(0, 0, -14), 8)
	thin, err := h.service.DemandForecast(context.Background(), "TAXI", "tz:z1", "TAXI_PASSENGER", h.now)
	if err != nil {
		t.Fatal(err)
	}
	if thin.Available || thin.SampleSize != 2 || thin.Score != networkanalytics.NeutralScore {
		t.Fatalf("two weeks of history must be low confidence: %+v", thin)
	}
	publish(h.now.AddDate(0, 0, -21), 10)
	answer, err := h.service.DemandForecast(context.Background(), "TAXI", "tz:z1", "TAXI_PASSENGER", h.now)
	if err != nil {
		t.Fatal(err)
	}
	if !answer.Available || answer.Expected != 8 || answer.CurrentCount == nil || *answer.CurrentCount != 4 ||
		answer.DataAsOf.IsZero() || answer.Method != networkanalytics.DemandForecastMethod || answer.ModelVersion == "" {
		t.Fatalf("forecast %+v", answer)
	}
	empty, _ := h.service.DemandForecast(context.Background(), "TAXI", "tz:none", "TAXI_PASSENGER", h.now)
	if empty.Available || empty.Expected != 0 || empty.CurrentCount != nil {
		t.Fatalf("no history must be neutral, not zero demand: %+v", empty)
	}
}

// Each execution authority, and each Taxi product, feeds mission outcomes.
func TestEveryAuthorityFeedsMissionOutcomes(t *testing.T) {
	h := newHarness(t)
	facts := []struct{ service, name, entity string }{
		{"driver-taxi-service", "taxi.booking.created", "booking"},
		{"driver-taxi-service", "taxi.booking.accepted", "booking"},
		{"driver-taxi-service", "taxi.booking.completed", "booking"},
		{"driver-taxi-service", "taxi.goods.mission.committed.v1", "taxi_goods_mission"},
		{"driver-taxi-service", "taxi.goods.completed.v1", "taxi_goods_mission"},
		{"driver-dispatch-delivery-service", "dispatch.booking.committed.v1", "dispatch_booking"},
		{"driver-dispatch-delivery-service", "trip.created.v1", "trip"},
		{"driver-dispatch-delivery-service", "trip.completed.v1", "trip"},
		{"driver-hauling-service", "hauling.booking.committed.v1", "haulage_booking"},
		{"driver-hauling-service", "hauling.booking.assigned.v1", "haulage_booking"},
		{"driver-hauling-service", "hauling.booking.completed.v1", "haulage_booking"},
		{"driver-hauling-service", "hauling.recovery.resolved", "recovery_case"},
		{"driver-hauling-service", "hauling.recovery.failed", "recovery_case"},
		{"driver-taxi-service", "taxi.recovery.resolved.v1", "taxi_goods_mission"},
	}
	for i, fact := range facts {
		h.ingest(t, envelope(fact.service, fact.name, fact.entity, "e"+string(rune('a'+i)), 1, h.now, map[string]any{}))
	}
	days, err := h.service.MissionOutcomes(context.Background(), "", "", 7)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]int64{}
	for _, day := range days {
		got[day.Authority+"/"+day.ProductCode] = map[string]int64{"committed": day.Committed, "assigned": day.Assigned,
			"completed": day.Completed, "recovered": day.Recovered, "recovery_failed": day.RecoveryFailed}
	}
	want := map[string]map[string]int64{
		"TAXI/TAXI_PASSENGER":      {"committed": 1, "assigned": 1, "completed": 1},
		"TAXI/TAXI_GOODS":          {"committed": 1, "completed": 1, "recovered": 1},
		"DISPATCH/DISPATCH_PARCEL": {"committed": 1, "assigned": 1, "completed": 1},
		"HAULING/HAULAGE":          {"committed": 1, "assigned": 1, "completed": 1, "recovered": 1, "recovery_failed": 1},
	}
	for key, counts := range want {
		for column, value := range counts {
			if got[key][column] != value {
				t.Errorf("%s %s = %d, want %d (all: %v)", key, column, got[key][column], value, got[key])
			}
		}
	}
}

func TestMigration004RollsBackAndReapplies(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	root := filepath.Join("..", "..", "..", "..", "migrations")
	down, err := os.ReadFile(filepath.Join(root, "down", "004_network_supply_demand_outcomes.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	var tables int
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name IN
		('supply_product_snapshots','demand_buckets','mission_outcomes_daily') AND table_schema = current_schema()`).Scan(&tables)
	if tables != 0 {
		t.Fatalf("down left %d tables", tables)
	}
	var kept int
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name='zone_supply_snapshots'
		AND table_schema = current_schema()`).Scan(&kept)
	if kept != 1 {
		t.Fatal("rolling back 004 removed a table 003 owns")
	}
	up, err := os.ReadFile(filepath.Join(root, "004_network_supply_demand_outcomes.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		if _, err = h.pool.Exec(ctx, string(up)); err != nil {
			t.Fatalf("reapply %d: %v", pass, err)
		}
	}
	if _, err = h.pool.Exec(ctx, `INSERT INTO supply_product_snapshots (authority, zone, product_code, online_count,
		eligible_count, supply_state, observed_at, bucket_start, source_version) VALUES
		('TAXI','z','P',1,2,'AVAILABLE',now(),now(),1)`); err == nil {
		t.Fatal("eligible above online must be refused by the constraint")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
