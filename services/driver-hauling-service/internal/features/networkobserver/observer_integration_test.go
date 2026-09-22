package networkobserver

// Section 13 closure, Haulage: supply and demand observed as counts only, and
// the booking lifecycle recorded for analytics by migration 024.

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/database"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	previous, _ := os.Getwd()
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	if err = database.ApplyMigrations(context.Background(), pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

type onlineSet []availabilityrepositories.ProviderStatus

func (s onlineSet) GetOnlineProviders(context.Context) ([]availabilityrepositories.ProviderStatus, error) {
	return s, nil
}

func seedProvider(t *testing.T, pool *pgxpool.Pool, truckStatus string) (string, string) {
	t.Helper()
	providerID, truckID := uuid.NewString(), uuid.NewString()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO truck_providers(id,phone,email,status,onboarding_status,service_type)
		VALUES($1,$2,$3,'active','complete','hauling')`, providerID, "+234"+providerID[:10], providerID+"@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number,status)
		VALUES($1,$2,'flatbed',1000,$3,$4)`, truckID, providerID, "TEST-"+truckID[:8], truckStatus); err != nil {
		t.Fatal(err)
	}
	return providerID, truckID
}

// testLocation is a random point per run, so each run owns its zone and a
// rerun against a reused database does not collide with an earlier one's facts.
var testLat, testLng = 4.5 + rand.Float64()*8, 3.0 + rand.Float64()*10

func seedBooking(t *testing.T, pool *pgxpool.Pool, providerID *string, status string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `INSERT INTO haulage_bookings(id,customer_id,provider_id,pickup_address,
		pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg,status)
				VALUES($1,$2,$3,'12 Marina Road',$5,$6,'Ikeja',6.6,3.35,'general',100,$4)`,
		id, uuid.NewString(), providerID, status, testLat, testLng); err != nil {
		t.Fatalf("seed booking: %v", err)
	}
	return id
}

func facts(t *testing.T, pool *pgxpool.Pool, since time.Time) map[string][]string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT event_type, payload::text FROM hauling_domain_outbox
		WHERE target='analytics-service' AND occurred_at >= $1`, since)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var event, payload string
		if err = rows.Scan(&event, &payload); err != nil {
			t.Fatal(err)
		}
		out[event] = append(out[event], payload)
	}
	return out
}

func TestHaulagePublishesSupplyAndDemandAsCountsOnly(t *testing.T) {
	pool := testPool(t)
	since := time.Now().UTC().Add(-time.Second)
	free, freeTruck := seedProvider(t, pool, "active")
	busy, busyTruck := seedProvider(t, pool, "active")
	_, inactiveTruck := seedProvider(t, pool, "inactive")
	seedBooking(t, pool, &busy, "en_route_pickup")
	seedBooking(t, pool, nil, "unmatched")
	lat, lng := testLat, testLng
	observer := &Observer{DB: pool, Online: onlineSet{
		{ProviderID: free, TruckID: freeTruck, Lat: lat, Lng: lng},
		{ProviderID: busy, TruckID: busyTruck, Lat: lat, Lng: lng},
		{ProviderID: uuid.NewString(), TruckID: inactiveTruck, Lat: lat, Lng: lng},
		{ProviderID: uuid.NewString(), Lat: lat, Lng: lng}, // no truck on record
	}}
	if err := observer.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	published := facts(t, pool, since)
	zone := networkanalytics.ZoneFor(lat, lng)
	var supply string
	for _, payload := range published[networkanalytics.EventSupplySnapshot] {
		// Only this run's zone.
		if strings.Contains(payload, zone) {
			supply = payload
		}
	}
	// Four online; one on a job; eligibility unknown for the truckless one,
	// so the zone's eligible count is unknown -- not three.
	if !strings.Contains(supply, `"online_count": 4`) || !strings.Contains(supply, `"occupied_count": 1`) ||
		!strings.Contains(supply, `"eligible_count": null`) || !strings.Contains(supply, `"state": "UNKNOWN"`) {
		t.Fatalf("supply %s", supply)
	}
	for event, payloads := range published {
		for _, payload := range payloads {
			for _, leak := range []string{free, busy, freeTruck, busyTruck, "Marina", "TEST-",
				strconv.FormatFloat(testLat, 'f', 3, 64), strconv.FormatFloat(testLng, 'f', 3, 64)} {
				if strings.Contains(payload, leak) {
					t.Fatalf("%s leaked %q: %s", event, leak, payload)
				}
			}
		}
	}
	var demand string
	for _, payload := range published[networkanalytics.EventDemandBucket] {
		if strings.Contains(payload, zone) {
			demand = payload
		}
	}
	if !strings.Contains(demand, `"demand_count": 2`) || !strings.Contains(demand, `"unmatched_count": 1`) {
		t.Fatalf("demand %v", published[networkanalytics.EventDemandBucket])
	}
	before := len(published[networkanalytics.EventSupplySnapshot])
	if err := observer.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if after := len(facts(t, pool, since)[networkanalytics.EventSupplySnapshot]); after != before {
		t.Fatalf("re-observing one bucket added supply rows: %d -> %d", before, after)
	}
}

func TestHaulageBookingLifecycleReachesAnalytics(t *testing.T) {
	pool := testPool(t)
	since := time.Now().UTC().Add(-time.Second)
	provider, _ := seedProvider(t, pool, "active")
	booking := seedBooking(t, pool, nil, "pending_match")
	ctx := context.Background()
	for _, status := range []string{"accepted", "delivered", "completed"} {
		if _, err := pool.Exec(ctx, `UPDATE haulage_bookings SET status=$2, provider_id=$3 WHERE id=$1`,
			booking, status, provider); err != nil {
			t.Fatal(err)
		}
	}
	// A repeated status write is not a second outcome.
	if _, err := pool.Exec(ctx, `UPDATE haulage_bookings SET status='completed' WHERE id=$1`, booking); err != nil {
		t.Fatal(err)
	}
	published := facts(t, pool, since)
	for event, payloads := range published {
		var mine []string
		for _, payload := range payloads {
			if strings.Contains(payload, booking) {
				mine = append(mine, payload)
			}
		}
		published[event] = mine
	}
	for _, event := range []string{"hauling.booking.committed.v1", "hauling.booking.assigned.v1", "hauling.booking.completed.v1"} {
		if len(published[event]) != 1 {
			t.Fatalf("%s published %d times (%v)", event, len(published[event]), published)
		}
		if strings.Contains(published[event][0], provider) || strings.Contains(published[event][0], "Marina") {
			t.Fatalf("%s leaked identity: %s", event, published[event][0])
		}
		if _, kind, ok := networkanalytics.Classify(event, ""); !ok || kind == "" {
			t.Fatalf("%s is not classified", event)
		}
	}
}
