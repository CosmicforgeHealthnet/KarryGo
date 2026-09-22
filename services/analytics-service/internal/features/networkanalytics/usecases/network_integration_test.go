package usecases

// Section 13: PostgreSQL-backed certification of network analytics.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/models"
	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/repositories"
	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

type harness struct {
	pool    *pgxpool.Pool
	service *Service
	now     time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dsn := os.Getenv("ANALYTICS_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("ANALYTICS_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("analytics test database unavailable")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "network_analytics_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "003_network_analytics.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	h := &harness{pool: pool, service: NewService(repositories.NewRepository(pool)),
		now: time.Now().UTC().Truncate(time.Second)}
	h.service.now = func() time.Time { return h.now }
	return h
}

func envelope(source, name, entityType, entityID string, version int64, at time.Time,
	payload map[string]any) sharedevents.Envelope {
	raw, _ := json.Marshal(payload)
	return sharedevents.Envelope{EventID: uuid.NewString(), EventName: name, SchemaVersion: 1,
		SourceService: source, SourceEntityType: entityType, SourceEntityID: entityID,
		SourceVersion: version, OccurredAt: at, Payload: raw}
}

func (h *harness) ingest(t *testing.T, e sharedevents.Envelope) string {
	t.Helper()
	status, err := h.service.Ingest(context.Background(), e.SourceService, e)
	if err != nil {
		t.Fatalf("ingest %s: %v", e.EventName, err)
	}
	return status
}

func (h *harness) count(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	var value int64
	if err := h.pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return value
}

const hauling = "driver-hauling-service"

func TestRecoveryLifecycleIsMeasuredAndTimed(t *testing.T) {
	h := newHarness(t)
	start := h.now.Add(-10 * time.Minute)
	caseID := "case-1"
	base := map[string]any{"recovery_case_id": caseID, "original_provider_id": "p-old"}
	h.ingest(t, envelope(hauling, "hauling.recovery.started", "recovery_case", caseID, 1, start, base))
	replaced := map[string]any{"recovery_case_id": caseID, "replacement_provider_id": "p-new"}
	h.ingest(t, envelope(hauling, "hauling.recovery.provider_replaced", "recovery_case", caseID, 2, start.Add(2*time.Minute), replaced))
	h.ingest(t, envelope(hauling, "hauling.recovery.resolved", "recovery_case", caseID, 3, start.Add(4*time.Minute), base))

	days, err := h.service.Recovery(context.Background(), networkanalytics.AuthorityHauling, 7)
	if err != nil || len(days) != 1 {
		t.Fatalf("recovery days %+v err=%v", days, err)
	}
	day := days[0]
	if day.Started != 1 || day.Replaced != 1 || day.Resolved != 1 || day.AverageResolutionSeconds != 240 {
		t.Fatalf("unexpected recovery performance %+v", day)
	}
	if h.count(t, `SELECT recoveries_as_original FROM provider_quality_snapshots WHERE provider_id='p-old'`) != 1 ||
		h.count(t, `SELECT recoveries_as_replacement FROM provider_quality_snapshots WHERE provider_id='p-new'`) != 1 {
		t.Fatal("recovery was not attributed to the providers involved")
	}
}

func TestDuplicatesAreCountedOnce(t *testing.T) {
	h := newHarness(t)
	event := envelope(hauling, "hauling.recovery.started", "recovery_case", "c1", 1, h.now, map[string]any{})
	if h.ingest(t, event) != models.StatusProcessed {
		t.Fatal("first delivery must be processed")
	}
	if h.ingest(t, event) != models.StatusDuplicate {
		t.Fatal("the same event id must be a duplicate")
	}
	// A producer that re-publishes the same fact under a new event id.
	republished := event
	republished.EventID = uuid.NewString()
	if h.ingest(t, republished) != models.StatusDuplicate {
		t.Fatal("the same fact under a new id must be a duplicate")
	}
	if got := h.count(t, `SELECT started FROM recovery_performance`); got != 1 {
		t.Fatalf("started=%d, want 1", got)
	}
}

func TestConcurrentDeliveriesProjectOnce(t *testing.T) {
	h := newHarness(t)
	event := envelope("driver-dispatch-delivery-service", "trip.completed.v1", "trip", "t1", 4, h.now,
		map[string]any{"provider_id": "rider-1"})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = h.service.Ingest(context.Background(), event.SourceService, event)
		}()
	}
	wg.Wait()
	if got := h.count(t, `SELECT completed FROM mission_success_daily`); got != 1 {
		t.Fatalf("completed=%d after concurrent deliveries, want 1", got)
	}
}

func TestAStaleSnapshotNeverOverwritesANewerOne(t *testing.T) {
	h := newHarness(t)
	snapshot := func(version int64, at time.Time, available int) sharedevents.Envelope {
		return envelope(hauling, networkanalytics.EventSupplySnapshot, "supply_zone", "zone-7", version, at,
			map[string]any{"authority": "HAULING", "zone": "zone-7", "available_providers": available, "busy_providers": 2})
	}
	if h.ingest(t, snapshot(2, h.now.Add(-time.Minute), 9)) != models.StatusProcessed {
		t.Fatal("newer snapshot must be processed")
	}
	if status := h.ingest(t, snapshot(1, h.now.Add(-5*time.Minute), 1)); status != models.StatusStale {
		t.Fatalf("older snapshot status=%s, want STALE", status)
	}
	answer, err := h.service.SupplySnapshot(context.Background(), "HAULING", "zone-7")
	if err != nil || answer.AvailableProviders != 9 || !answer.Available {
		t.Fatalf("supply %+v err=%v", answer, err)
	}
	if h.count(t, `SELECT count(*) FROM analytics_network_events WHERE status='STALE'`) != 1 {
		t.Fatal("the stale event must still be kept for audit")
	}
}

func TestAnOldSupplySnapshotIsNotPresentedAsCurrent(t *testing.T) {
	h := newHarness(t)
	h.ingest(t, envelope(hauling, networkanalytics.EventSupplySnapshot, "supply_zone", "zone-1", 1,
		h.now.Add(-time.Hour), map[string]any{"authority": "HAULING", "zone": "zone-1", "available_providers": 5}))
	answer, err := h.service.SupplySnapshot(context.Background(), "HAULING", "zone-1")
	if err != nil || answer.Available || answer.Score != networkanalytics.NeutralScore {
		t.Fatalf("an hour-old snapshot must read as neutral: %+v err=%v", answer, err)
	}
}

func TestUnknownEventsAreKeptButNotProjected(t *testing.T) {
	h := newHarness(t)
	if status := h.ingest(t, envelope(hauling, "hauling.something_new", "booking", "b1", 1, h.now,
		map[string]any{})); status != models.StatusIgnored {
		t.Fatalf("status=%s, want IGNORED", status)
	}
	for _, table := range []string{"recovery_performance", "mission_success_daily", "provider_quality_snapshots"} {
		if h.count(t, "SELECT count(*) FROM "+table) != 0 {
			t.Fatalf("an unknown event was projected into %s", table)
		}
	}
}

func TestPeopleLocationsAndMoneyAreNeverStored(t *testing.T) {
	h := newHarness(t)
	h.ingest(t, envelope(hauling, "hauling.recovery.started", "recovery_case", "c-privacy", 1, h.now, map[string]any{
		"original_provider_id": "p1", "pickup_lat": 6.45, "pickup_lng": 3.39, "dropoff_address": "12 Marina",
		"receiver_phone": "+2348000000000", "customer_email": "a@b.c", "fare_amount_kobo": 500000,
		"provider_name": "Ada", "notes": "gate code 1234",
	}))
	var stored string
	if err := h.pool.QueryRow(context.Background(), `SELECT payload::text FROM analytics_network_events`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"6.45", "Marina", "+234", "a@b.c", "500000", "Ada", "1234"} {
		if strings.Contains(stored, leaked) {
			t.Fatalf("stored payload leaked %q: %s", leaked, stored)
		}
	}
	if !strings.Contains(stored, "p1") {
		t.Fatal("the provider reference a metric needs was dropped")
	}
}

func TestTheSourceMustBeTheCaller(t *testing.T) {
	h := newHarness(t)
	forged := envelope(hauling, "hauling.recovery.started", "recovery_case", "c1", 1, h.now, map[string]any{})
	if _, err := h.service.Ingest(context.Background(), "vendor-service", forged); !errors.Is(err, ErrCallerMismatch) {
		t.Fatalf("a service must not report another's events, got %v", err)
	}
	// The provisioned alias of the same service is the same service.
	if _, err := h.service.Ingest(context.Background(), "hauling-service", forged); err != nil {
		t.Fatalf("alias rejected: %v", err)
	}
}

func TestMismatchReasonsAreCountedWhateverTheShape(t *testing.T) {
	h := newHarness(t)
	h.ingest(t, envelope("driver-dispatch-delivery-service", "dispatch.capability_mismatch.v1", "delivery_mission",
		"m1", 1, h.now, map[string]any{"reason_codes": "PARCEL_TOO_HEAVY, PARCEL_TOO_LARGE"}))
	h.ingest(t, envelope(hauling, "hauling.capability_mismatch", "recovery_case", "c1", 1, h.now,
		map[string]any{"reason_codes": []any{"PARCEL_TOO_HEAVY"}}))
	days, err := h.service.Mismatches(context.Background(), "", 7)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, day := range days {
		got[day.Authority+"/"+day.ReasonCode] += day.Mismatches
	}
	want := map[string]int64{"DISPATCH/PARCEL_TOO_HEAVY": 1, "DISPATCH/PARCEL_TOO_LARGE": 1, "HAULING/PARCEL_TOO_HEAVY": 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("mismatches %v, want %v", got, want)
	}
}

func TestBranchReadinessNeedsEnoughEvidence(t *testing.T) {
	h := newHarness(t)
	ready := func(order string, onTime bool) {
		h.ingest(t, envelope("vendor-service", networkanalytics.EventBranchReadiness, "vendor_order", order, 3, h.now,
			map[string]any{"business_id": "biz", "branch_id": "branch-1", "on_time": onTime,
				"lateness_seconds": map[bool]int{true: 0, false: 300}[onTime], "prep_seconds": 600}))
	}
	for i := 0; i < 4; i++ {
		ready(fmt.Sprintf("o%d", i), i != 0)
	}
	thin, err := h.service.BranchReadiness(context.Background(), "branch-1", 30)
	if err != nil || thin.Available || thin.Score != networkanalytics.NeutralScore || thin.SampleSize != 4 {
		t.Fatalf("four orders must be too few to score: %+v err=%v", thin, err)
	}
	ready("o4", true)
	answer, err := h.service.BranchReadiness(context.Background(), "branch-1", 30)
	if err != nil || !answer.Available || answer.Score != 0.8 || answer.AverageLatenessSecs != 300 ||
		answer.AveragePrepSecs != 600 {
		t.Fatalf("readiness %+v err=%v", answer, err)
	}
	unknown, _ := h.service.BranchReadiness(context.Background(), "branch-none", 30)
	if unknown.Available || unknown.Score != networkanalytics.NeutralScore {
		t.Fatalf("a branch with no data must read neutral: %+v", unknown)
	}
}

func TestProviderQualityIsOperationalAndThresholded(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 4; i++ {
		h.ingest(t, envelope("driver-dispatch-delivery-service", "trip.completed.v1", "trip", fmt.Sprintf("t%d", i), 1,
			h.now, map[string]any{"provider_id": "rider-9"}))
	}
	// Dispatch's own payloads are camelCase; the read model must still find
	// the provider.
	h.ingest(t, envelope("driver-dispatch-delivery-service", "dispatch.delivery_recovery.selected.v1", "trip", "t9", 1,
		h.now, map[string]any{"providerId": "rider-9", "pickupAddress": "must not be stored"}))
	answer, err := h.service.ProviderQuality(context.Background(), "DISPATCH", "rider-9", 30)
	if err != nil || !answer.Available || answer.SampleSize != 5 || answer.CompletionRate != 1 ||
		answer.RecoveryInvolvement != 0.2 || answer.Score != 0.8 {
		t.Fatalf("provider quality %+v err=%v", answer, err)
	}
	none, _ := h.service.ProviderQuality(context.Background(), "DISPATCH", "rider-unknown", 30)
	if none.Available || none.Score != networkanalytics.NeutralScore {
		t.Fatalf("an unknown provider must read neutral: %+v", none)
	}
}

func TestMissionSuccessAndGuaranteeBreaches(t *testing.T) {
	h := newHarness(t)
	taxi := "driver-taxi-service"
	h.ingest(t, envelope(taxi, "taxi.goods.completed.v1", "taxi_goods_mission", "g1", 5, h.now, map[string]any{}))
	h.ingest(t, envelope(taxi, "taxi.goods.mission.cancelled.v1", "taxi_goods_mission", "g2", 2, h.now, map[string]any{}))
	h.ingest(t, envelope(taxi, "taxi.guarantee.breached", "booking", "b1", 1, h.now, map[string]any{}))
	days, err := h.service.MissionSuccess(context.Background(), "TAXI", 7)
	if err != nil || len(days) != 1 {
		t.Fatalf("days %+v err=%v", days, err)
	}
	if day := days[0]; day.Completed != 1 || day.Cancelled != 1 || day.GuaranteeBreaches != 1 || day.SuccessRate != 1 {
		t.Fatalf("mission success %+v", day)
	}
}

func TestDemandProjection(t *testing.T) {
	h := newHarness(t)
	h.ingest(t, envelope(hauling, networkanalytics.EventDemandObserved, "demand_zone", "zone-2", 1, h.now,
		map[string]any{"authority": "HAULING", "zone": "zone-2", "requests": 3}))
	h.ingest(t, envelope(hauling, networkanalytics.EventDemandObserved, "demand_zone", "zone-2", 2, h.now,
		map[string]any{"authority": "HAULING", "zone": "zone-2", "unmatched": true}))
	days, err := h.service.Demand(context.Background(), "HAULING", 7)
	if err != nil || len(days) != 1 || days[0].Requests != 4 || days[0].Unmatched != 1 {
		t.Fatalf("demand %+v err=%v", days, err)
	}
}

// E2E 3: a safety decline is counted apart and never scores against the
// provider who made it.
func TestASafetyDeclineIsDistinctFromAPerformanceDecline(t *testing.T) {
	h := newHarness(t)
	dispatch := "driver-dispatch-delivery-service"
	for i := 0; i < 5; i++ {
		h.ingest(t, envelope(dispatch, "trip.completed.v1", "trip", fmt.Sprintf("t%d", i), 1, h.now,
			map[string]any{"provider_id": "rider-s"}))
	}
	h.ingest(t, envelope(dispatch, "dispatch.offer.declined.v1", "delivery_mission", "m1", 2, h.now,
		map[string]any{"provider_id": "rider-s", "reason": "UNSAFE_VEHICLE"}))
	h.ingest(t, envelope(dispatch, "dispatch.offer.declined.v1", "delivery_mission", "m2", 2, h.now,
		map[string]any{"provider_id": "rider-s", "reason": "SHIFT_ENDING"}))

	quality, err := h.service.ProviderQuality(context.Background(), "DISPATCH", "rider-s", 30)
	if err != nil {
		t.Fatal(err)
	}
	if quality.SafetyDeclines != 1 || quality.Declines != 1 {
		t.Fatalf("safety=%d ordinary=%d, want 1/1", quality.SafetyDeclines, quality.Declines)
	}
	if quality.Score != 1 {
		t.Fatalf("a decline changed the provider's score to %v", quality.Score)
	}
	days, err := h.service.Declines(context.Background(), "DISPATCH", 7)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, day := range days {
		got[day.ReasonCode] = day.Safety
	}
	if safety, ok := got["UNSAFE_VEHICLE"]; !ok || !safety {
		t.Fatalf("UNSAFE_VEHICLE must be recorded as a safety decline: %v", got)
	}
	if safety, ok := got["SHIFT_ENDING"]; !ok || safety {
		t.Fatalf("SHIFT_ENDING must be recorded as an ordinary decline: %v", got)
	}
}

// E2E 1: the readiness profile carries a version that moves with the data.
func TestTheReadinessProfileVersionAdvances(t *testing.T) {
	h := newHarness(t)
	ready := func(order string) {
		h.ingest(t, envelope("vendor-service", networkanalytics.EventBranchReadiness, "vendor_order", order, 3, h.now,
			map[string]any{"branch_id": "br-v", "on_time": true}))
	}
	ready("o1")
	first, _ := h.service.BranchReadiness(context.Background(), "br-v", 30)
	ready("o1") // a replayed fact
	replayed, _ := h.service.BranchReadiness(context.Background(), "br-v", 30)
	ready("o2")
	second, _ := h.service.BranchReadiness(context.Background(), "br-v", 30)
	if first.ProfileVersion != 1 || replayed.ProfileVersion != 1 || second.ProfileVersion != 2 || second.DataAsOf.IsZero() {
		t.Fatalf("versions %d/%d/%d data_as_of=%v", first.ProfileVersion, replayed.ProfileVersion,
			second.ProfileVersion, second.DataAsOf)
	}
}
