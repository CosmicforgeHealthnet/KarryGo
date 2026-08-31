package migrations_test

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/assignment"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/deliveryrecovery"
	planning "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/missionplanning"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLifecycleDeliveryMigrationShape(t *testing.T) {
	raw, err := os.ReadFile("000051_lifecycle_delivery_replacement_recovery.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"AFTER INSERT OR UPDATE OF status ON dispatch_bookings", "AFTER INSERT OR UPDATE OF status ON trips", "dispatch_route_alert_analytics_intent", "dispatch_vendor_lifecycle_snapshot", "recovery_lease_expires_at", "dispatch_assignment_one_replacement_winner", "recovery_deadline", "ON CONFLICT DO NOTHING"} {
		if !strings.Contains(string(raw), want) {
			t.Fatal("missing", want)
		}
	}
	if strings.Contains(string(raw), "CREATE TABLE") {
		t.Fatal("closure duplicated persistence")
	}
}

func TestLifecycleNotificationIntentAtomicRollbackAndDedupPostgres(t *testing.T) {
	_, pool, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE trips SET status='in_progress' WHERE id=$1`, f.trip); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_marketplace_outbox WHERE event_type='package.picked_up' AND aggregate_type='booking_notification'`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	_ = tx.Rollback(ctx)
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_marketplace_outbox WHERE event_type='package.picked_up'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("intent escaped rollback", count, err)
	}
	for i := 0; i < 2; i++ {
		if _, err = pool.Exec(ctx, `UPDATE trips SET status='in_progress' WHERE id=$1`, f.trip); err != nil {
			t.Fatal(err)
		}
	}
	var raw []byte
	if err = pool.QueryRow(ctx, `SELECT payload FROM dispatch_marketplace_outbox WHERE event_type='package.picked_up'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	if len(p) != 3 {
		t.Fatal("unexpected intent fields", p)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_marketplace_outbox WHERE event_type='package.picked_up'`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestIncidentRecoveryClaimReplayAcceptanceAndNoSecondTripPostgres(t *testing.T) {
	_, pool, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx := context.Background()
	group, mission := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_delivery_groups(id,source_type,source_ref,state) VALUES($1,'CUSTOMER',$1::text,'ACTIVE')`, group); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref,booking_id,trip_id,state) VALUES($1,$2,'CUSTOMER',$1::text,$3,$4,'ASSIGNED')`, mission, group, f.booking, f.trip); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bikes(provider_id,bike_type,brand,model,year,color,plate_number,verification_status) VALUES($1,'motorcycle','test','test',2025,'black',$1::text,'verified')`, f.providerB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_availability(provider_id,status,verified_to_go_online) VALUES($1,'online',true)`, f.providerB); err != nil {
		t.Fatal(err)
	}
	repo := deliveryrecovery.NewRepository(pool)
	incident, err := repo.ReportIncident(ctx, deliveryrecovery.IncidentCommand{TripID: f.trip, ProviderID: f.providerA, ReasonCode: "BREAKDOWN", PreferredStrategy: "SEQUENTIAL_NEAREST", IdempotencyKey: "incident"}, time.Now(), "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	counts := make(chan int, 2)
	errs := make(chan error, 2)
	for _, owner := range []string{"a", "b"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			items, e := repo.ClaimIncidentRecovery(ctx, time.Now(), owner, time.Minute, 10)
			counts <- len(items)
			errs <- e
		}(owner)
	}
	wg.Wait()
	close(counts)
	close(errs)
	n := 0
	for c := range counts {
		n += c
	}
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if n != 1 {
		t.Fatal("claim winners", n)
	}
	ar := assignment.NewRepository(pool)
	scope, err := ar.ReadReplacement(ctx, incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	offerErrors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := ar.OfferReplacement(ctx, scope, []assignment.Candidate{{ProviderID: f.providerB, Rank: 1}}, time.Now(), time.Minute)
			offerErrors <- e
		}()
	}
	wg.Wait()
	close(offerErrors)
	for e := range offerErrors {
		if e != nil {
			t.Fatal(e)
		}
	}
	var offer string
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_assignment_offers WHERE incident_id=$1`, incident.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err = pool.QueryRow(ctx, `SELECT id::text FROM dispatch_assignment_offers WHERE incident_id=$1`, incident.ID).Scan(&offer); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		result, e := ar.Accept(ctx, offer, f.providerB, assignment.Decision{IdempotencyKey: "accept", ExpectedVersion: 1}, time.Now())
		if e != nil || result.TripID != f.trip || result.Status != "REPLACEMENT_READY" {
			t.Fatal(result, e)
		}
	}
	var provider, result string
	if err = pool.QueryRow(ctx, `SELECT t.provider_id::text,i.recovery_result FROM trips t JOIN dispatch_delivery_incidents i ON i.trip_id=t.id WHERE i.id=$1`, incident.ID).Scan(&provider, &result); err != nil || provider != f.providerA || result != "READY" {
		t.Fatal("custody moved", provider, result, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM trips WHERE booking_id=$1`, f.booking).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate Trip", count, err)
	}
}

func TestLifecycleMigrationApplyRollbackReapplyPostgres(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	for _, path := range []string{"000051_lifecycle_delivery_replacement_recovery.down.sql", "000051_lifecycle_delivery_replacement_recovery.up.sql"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(context.Background(), string(raw)); err != nil {
			t.Fatal(path, err)
		}
	}
}

func TestReplacementCancelledOrExpiredNeverCreatesOfferPostgres(t *testing.T) {
	for _, scenario := range []string{"cancelled", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			_, pool, f, cleanup := seedHandoverFixture(t)
			defer cleanup()
			ctx := context.Background()
			group, mission := uuid.NewString(), uuid.NewString()
			if _, err := pool.Exec(ctx, `INSERT INTO dispatch_delivery_groups(id,source_type,source_ref,state) VALUES($1,'CUSTOMER',$1::text,'ACTIVE')`, group); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref,trip_id,state) VALUES($1,$2,'CUSTOMER',$1::text,$3,'ASSIGNED')`, mission, group, f.trip); err != nil {
				t.Fatal(err)
			}
			repo := deliveryrecovery.NewRepository(pool)
			incident, err := repo.ReportIncident(ctx, deliveryrecovery.IncidentCommand{TripID: f.trip, ProviderID: f.providerA, ReasonCode: "BREAKDOWN", PreferredStrategy: "SEQUENTIAL_NEAREST", IdempotencyKey: "incident"}, time.Now(), "fp")
			if err != nil {
				t.Fatal(err)
			}
			want := "CANCELLED"
			if scenario == "cancelled" {
				_, err = pool.Exec(ctx, `UPDATE trips SET status='cancelled' WHERE id=$1`, f.trip)
			} else {
				want = "MANUAL_REVIEW"
				_, err = pool.Exec(ctx, `UPDATE dispatch_delivery_incidents SET recovery_deadline=now()-interval '1 second' WHERE id=$1`, incident.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			items, err := repo.ClaimIncidentRecovery(ctx, time.Now(), "worker", time.Minute, 10)
			if err != nil || len(items) != 0 {
				t.Fatal(items, err)
			}
			var state string
			var count int
			if err = pool.QueryRow(ctx, `SELECT recovery_result,(SELECT count(*) FROM dispatch_assignment_offers WHERE incident_id=$1) FROM dispatch_delivery_incidents WHERE id=$1`, incident.ID).Scan(&state, &count); err != nil || state != want || count != 0 {
				t.Fatal(state, count, err)
			}
		})
	}
}

func TestVendorLifecycleProjectionIsTransactionBoundPostgres(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	repo := planning.NewRepository(pool)
	svc := planning.New(repo, planning.NewPlanner(), true).WithRouteAuthority(routeAuthorityStub{})
	in := executionPlanRequest()
	plan, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err = pool.QueryRow(ctx, `SELECT payload->'delivery_projection' FROM dispatch_marketplace_outbox WHERE event_type='dispatch.delivery_plan.created.v1' AND aggregate_id=$1`, plan.GroupRef).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	p, err := dispatchcommerce.DecodeLifecycleProjection(raw)
	if err != nil || p.SourceRef != in.SourceRef || len(p.Missions) != 1 || p.Missions[0].MissionRef != plan.Missions[0].MissionRef {
		t.Fatal(p, err)
	}
	if strings.Contains(string(raw), "latitude") || strings.Contains(string(raw), "receiver") {
		t.Fatal("private projection")
	}
}
