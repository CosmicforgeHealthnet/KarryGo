package networkobserver

// Section 13 closure, Taxi: supply and demand observed as counts only, with
// UNKNOWN kept distinct from zero, in Taxi's own markets and zones.

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/taxi-service/internal/database"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

var (
	migrateOnce sync.Once
	migrateErr  error
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrateOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
		previous, _ := os.Getwd()
		if migrateErr = os.Chdir(root); migrateErr != nil {
			return
		}
		defer func() { _ = os.Chdir(previous) }()
		migrateErr = database.ApplyMigrations(context.Background(), pool)
	})
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}
	return pool
}

type onlineSet []availabilityrepositories.ProviderStatus

func (s onlineSet) GetOnlineProviders(context.Context) ([]availabilityrepositories.ProviderStatus, error) {
	return s, nil
}

func TestTaxiPublishesSupplyAndDemandAsCountsOnly(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	market, zone := uuid.NewString(), uuid.NewString()
	code := "TEST-" + market[:8]
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("%v: %s", err, query)
		}
	}
	exec(`INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Test Market', $2)`, market, code)
	exec(`INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Test Zone', $3, 6.5, 3.4, 10, '{economy}')`, zone, market, code)

	// Each run owns its cells, so a reused database cannot collide.
	knownLat, knownLng := 4.5+rand.Float64()*8, 3.0+rand.Float64()*10
	unknownLat, unknownLng := knownLat+1, knownLng+1
	var online onlineSet
	var ids []string
	for _, state := range []string{"AVAILABLE", "ON_TRIP", "PAUSED", "OFFLINE", ""} {
		id := uuid.NewString()
		ids = append(ids, id)
		exec(`INSERT INTO taxi_providers (id, phone) VALUES ($1, $2)`, id, "+234"+id[:10])
		lat, lng := knownLat, knownLng
		if state == "" {
			lat, lng = unknownLat, unknownLng // no operational state on record
		} else {
			exec(`INSERT INTO provider_operational_state (provider_id, state, version) VALUES ($1, $2, 1)`, id, state)
		}
		online = append(online, availabilityrepositories.ProviderStatus{ProviderID: id, VehicleID: uuid.NewString(),
			MarketID: market, VehicleClass: "economy", Lat: lat, Lng: lng})
	}
	for _, status := range []string{"pending_match", "unmatched"} {
		exec(`INSERT INTO taxi_bookings (id, customer_id, market_id, operating_zone_id, vehicle_class, status)
			VALUES ($1, $2, $3, $4, 'economy', $5)`, uuid.NewString(), uuid.NewString(), market, zone, status)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM taxi_bookings WHERE market_id=$1::uuid`, market)
		_, _ = pool.Exec(c, `DELETE FROM taxi_providers WHERE id = ANY($1::uuid[])`, ids)
		_, _ = pool.Exec(c, `DELETE FROM operating_zones WHERE id=$1::uuid`, zone)
		_, _ = pool.Exec(c, `DELETE FROM markets WHERE id=$1::uuid`, market)
	})

	observer := &Observer{DB: pool, Online: online}
	if err := observer.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	published := map[string][]string{}
	rows, err := pool.Query(ctx, `SELECT event_type, payload::text FROM taxi_outbox
		WHERE target='analytics-service' AND payload::text LIKE '%'||$1||'%'`, market)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var event, payload string
		if err = rows.Scan(&event, &payload); err != nil {
			t.Fatal(err)
		}
		published[event] = append(published[event], payload)
	}
	rows.Close()

	find := func(event, zoneKey string) string {
		for _, payload := range published[event] {
			if strings.Contains(payload, `"`+zoneKey+`"`) {
				return payload
			}
		}
		t.Fatalf("no %s for %s in %v", event, zoneKey, published)
		return ""
	}
	// OFFLINE is not online; ON_TRIP is occupied; PAUSED may not be offered.
	known := find(networkanalytics.EventSupplySnapshot, networkanalytics.ZoneFor(knownLat, knownLng))
	for _, want := range []string{`"online_count": 3`, `"occupied_count": 1`, `"eligible_count": 2`, `"product_code": "TAXI_ECONOMY"`} {
		if !strings.Contains(known, want) {
			t.Fatalf("known supply lacks %s: %s", want, known)
		}
	}
	// No recorded state: unknown, never zero.
	unknown := find(networkanalytics.EventSupplySnapshot, networkanalytics.ZoneFor(unknownLat, unknownLng))
	for _, want := range []string{`"online_count": 1`, `"occupied_count": null`, `"eligible_count": null`, `"state": "UNKNOWN"`} {
		if !strings.Contains(unknown, want) {
			t.Fatalf("unknown supply lacks %s: %s", want, unknown)
		}
	}
	demand := find(networkanalytics.EventDemandBucket, "tz:"+zone)
	for _, want := range []string{`"demand_count": 2`, `"unmatched_count": 1`, `"product_code": "TAXI_PASSENGER"`} {
		if !strings.Contains(demand, want) {
			t.Fatalf("demand lacks %s: %s", want, demand)
		}
	}
	for event, payloads := range published {
		for _, payload := range payloads {
			leaks := []string{strconv.FormatFloat(knownLat, 'f', 3, 64), strconv.FormatFloat(knownLng, 'f', 3, 64)}
			for _, status := range online {
				leaks = append(leaks, status.ProviderID, status.VehicleID)
			}
			for _, leak := range leaks {
				if strings.Contains(payload, leak) {
					t.Fatalf("%s leaked %q: %s", event, leak, payload)
				}
			}
		}
	}

	var before, after int
	countSupply := func() int {
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM taxi_outbox WHERE target='analytics-service'
			AND event_type=$1 AND payload::text LIKE '%'||$2||'%'`, networkanalytics.EventSupplySnapshot, market).Scan(&n)
		return n
	}
	before = countSupply()
	if err = observer.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if after = countSupply(); after != before {
		t.Fatalf("re-observing one bucket added supply rows: %d -> %d", before, after)
	}
}
