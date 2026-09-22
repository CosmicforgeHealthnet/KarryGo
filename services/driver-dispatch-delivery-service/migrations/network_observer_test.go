package migrations_test

// Section 13 closure, Dispatch: supply and demand are observed from Dispatch's
// own state and published as counts only; lifecycle facts reach analytics.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/networkobserver"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

type fixedLocator map[string]availability.Location

func (l fixedLocator) GetLocation(_ context.Context, id string) (availability.Location, bool, error) {
	location, ok := l[id]
	return location, ok, nil
}

func seedRider(t *testing.T, pool *pgxpool.Pool, status string, verified bool) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO providers (id, phone, verification_status, is_active, onboarding_complete, operation_type)
		VALUES (gen_random_uuid(), $1, 'verified', true, true, 'fleet') RETURNING id::text`,
		"+23480"+strings.ReplaceAll(uuid.NewString(), "-", "")[:8]).Scan(&id); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_availability (provider_id, status, verified_to_go_online)
		VALUES ($1::uuid, $2, $3)`, id, status, verified); err != nil {
		t.Fatalf("seed availability: %v", err)
	}
	return id
}

func seedBooking(t *testing.T, pool *pgxpool.Pool, status string) {
	t.Helper()
	ctx := context.Background()
	customer := uuid.NewString()
	quote := "quote_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_serviceability_quotes(
		id,customer_id,idempotency_key,request_fingerprint,
		pickup_latitude,pickup_longitude,dropoff_latitude,dropoff_longitude,
		vehicle_class,decision,policy_version,package_declaration,package_admission,
		fare_estimate_kobo,currency,expires_at)
		VALUES($1,$2,'quote-key','quote-fingerprint',6.5,3.4,6.6,3.5,
		'MOTORCYCLE','SERVICEABLE','route-policy-v1','{}','{"allowed":true}',
		1000,'NGN',now()+interval '10 minutes')`, quote, customer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_bookings(
		customer_id,status,pickup_address,pickup_lat,pickup_lng,
		dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,
		serviceability_quote_id,serviceability_quote_version,serviceability_quote_fingerprint,
		create_idempotency_key,create_request_fingerprint)
		VALUES($1,$2,'12 Marina Road',6.5243,3.3792,'dropoff',6.6,3.5,1000,'Ada Receiver','+2348000000000',
		$3,1,'quote-fingerprint',$4,'fp')`, customer, status, quote, "key-"+uuid.NewString()); err != nil {
		t.Fatalf("seed booking: %v", err)
	}
}

type outboxFact struct {
	event, aggregateType string
	payload              map[string]any
	raw                  string
}

func analyticsFacts(t *testing.T, pool *pgxpool.Pool) []outboxFact {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT event_type, aggregate_type, payload::text
		FROM dispatch_marketplace_outbox WHERE target_service='analytics-service' ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var facts []outboxFact
	for rows.Next() {
		var fact outboxFact
		if err = rows.Scan(&fact.event, &fact.aggregateType, &fact.raw); err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal([]byte(fact.raw), &fact.payload)
		facts = append(facts, fact)
	}
	return facts
}

func TestDispatchPublishesSupplyAndDemandAsCountsOnly(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	free := seedRider(t, pool, "online", true)
	busy := seedRider(t, pool, "busy", true)
	unverified := seedRider(t, pool, "online", false)
	seedRider(t, pool, "offline", true)
	seedBooking(t, pool, "searching")
	seedBooking(t, pool, "unmatched")
	lagos := availability.Location{Lat: 6.5244, Lng: 3.3792}
	clock := time.Now().UTC()
	observer := &networkobserver.Observer{DB: pool, Now: func() time.Time { return clock },
		Locator: fixedLocator{free: lagos, busy: lagos}}

	if err := observer.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	facts := analyticsFacts(t, pool)
	var supply, demand []outboxFact
	for _, fact := range facts {
		switch fact.event {
		case networkanalytics.EventSupplySnapshot:
			supply = append(supply, fact)
		case networkanalytics.EventDemandBucket:
			demand = append(demand, fact)
		}
		for _, id := range []string{free, busy, unverified} {
			if strings.Contains(fact.raw, id) {
				t.Fatalf("a rider id reached analytics: %s", fact.raw)
			}
		}
		for _, leak := range []string{"6.524", "3.379", "Marina", "Ada", "+234"} {
			if fact.event != "dispatch.booking.committed.v1" && strings.Contains(fact.raw, leak) {
				t.Fatalf("%s leaked %q: %s", fact.event, leak, fact.raw)
			}
		}
	}
	zone := networkanalytics.ZoneFor(lagos.Lat, lagos.Lng)
	var located, unlocated map[string]any
	for _, fact := range supply {
		switch fact.payload["zone"] {
		case zone:
			located = fact.payload
		case networkanalytics.UnlocatedZone:
			unlocated = fact.payload
		}
	}
	if located == nil || located["online_count"] != float64(2) || located["eligible_count"] != float64(2) ||
		located["occupied_count"] != float64(1) || located["state"] != networkanalytics.SupplyAvailable {
		t.Fatalf("located supply %v", located)
	}
	if unlocated == nil || unlocated["online_count"] != float64(1) || unlocated["eligible_count"] != float64(0) {
		t.Fatalf("the unverified rider without a fix must be counted, unlocated and ineligible: %v", unlocated)
	}
	if len(demand) != 1 || demand[0].payload["demand_count"] != float64(2) ||
		demand[0].payload["unmatched_count"] != float64(1) || demand[0].payload["zone"] != zone {
		t.Fatalf("demand %v", demand)
	}

	// A second observation in the same bucket is the same supply fact.
	if err := observer.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var supplyRows int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM dispatch_marketplace_outbox
		WHERE event_type=$1`, networkanalytics.EventSupplySnapshot).Scan(&supplyRows)
	if supplyRows != len(supply) {
		t.Fatalf("re-observing one bucket added supply rows: %d -> %d", len(supply), supplyRows)
	}

	// The last rider in the zone goes offline: the zone reads zero, not stale.
	if _, err := pool.Exec(context.Background(), `UPDATE provider_availability SET status='offline'
		WHERE provider_id = ANY($1::uuid[])`, []string{free, busy}); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(networkanalytics.SupplyBucket)
	if err := observer.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var zeroed int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM dispatch_marketplace_outbox
		WHERE event_type=$1 AND payload->>'zone'=$2 AND (payload->>'online_count')::int = 0`,
		networkanalytics.EventSupplySnapshot, zone).Scan(&zeroed)
	if zeroed != 1 {
		t.Fatalf("a vanished zone must be published as zero (%d)", zeroed)
	}
}

// Migration 000064: committed and assigned bookings, and the real 'TRIP'
// aggregate, reach analytics.
func TestDispatchLifecycleReachesAnalytics(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	seedBooking(t, pool, "searching")
	if _, err := pool.Exec(ctx, `UPDATE dispatch_bookings SET status='accepted'`); err != nil {
		t.Fatal(err)
	}
	trip := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox
		(event_type, aggregate_type, aggregate_id, aggregate_version, payload, payload_hash, target_service)
		VALUES ('trip.completed.v1','TRIP',$1,3,'{"providerId":"r1"}'::jsonb,'h','fleet-partner-service')`, trip); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, fact := range analyticsFacts(t, pool) {
		counts[fact.event]++
		if fact.event == "dispatch.booking.committed.v1" {
			for _, leak := range []string{"Marina", "Ada", "6.52", "+234"} {
				if strings.Contains(fact.raw, leak) {
					t.Fatalf("the committed fact leaked %q: %s", leak, fact.raw)
				}
			}
		}
	}
	if counts["dispatch.booking.committed.v1"] != 1 || counts["dispatch.booking.assigned.v1"] != 1 ||
		counts["trip.completed.v1"] != 1 {
		t.Fatalf("lifecycle facts %v", counts)
	}
	down, err := os.ReadFile("000064_network_analytics_lifecycle.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	up, err := os.ReadFile("000064_network_analytics_lifecycle.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}
