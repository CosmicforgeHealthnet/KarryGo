package recovery

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/services/hauling-service/internal/database"
	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/geo"
)

// Recovery is almost entirely about what happens when two things race or when
// a write half-lands, so these tests run against a real database. An in-memory
// fake would happily let both racers win.
//
// They skip without HAULING_TEST_DATABASE_URL, matching the repository's
// convention for DB-backed tests.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("HAULING_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	migrateOnce.Do(func() { migrateErr = applyServiceMigrations(pool) })
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}
	return pool
}

var (
	migrateOnce sync.Once
	migrateErr  error
)

// applyServiceMigrations runs the service's real migration runner, so this
// package works against an EMPTY test database as well as one another package
// already migrated (the runner is tracked and replay-safe).
func applyServiceMigrations(pool *pgxpool.Pool) error {
	_, sourceFile, _, _ := runtime.Caller(0)
	serviceRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	previous, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(serviceRoot); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(previous) }()
	return database.ApplyMigrations(context.Background(), pool)
}

const (
	providerA = "aaaaaaaa-0000-4000-8000-000000000001"
	providerB = "aaaaaaaa-0000-4000-8000-000000000002"
	providerC = "aaaaaaaa-0000-4000-8000-000000000003"
	truckA    = "bbbbbbbb-0000-4000-8000-000000000001"
	truckB    = "bbbbbbbb-0000-4000-8000-000000000002"
	truckC    = "bbbbbbbb-0000-4000-8000-000000000003"
)

// seed builds a booking already assigned to providerA, plus two potential
// replacements. It returns the booking id.
const seededBookingID = "cccccccc-0000-4000-8000-000000000001"

func seed(t *testing.T, pool *pgxpool.Pool, status string) string {
	t.Helper()
	ctx := context.Background()
	bookingID := seededBookingID

	// Clean only THIS test's own fixed rows.
	//
	// A blanket DELETE FROM haulage_bookings would be both slower and wrong:
	// the suite shares a database with packages that hold foreign keys into
	// bookings (private-execution mappings, among others), so wiping the table
	// fails on their constraints and, worse, would destroy their fixtures if
	// it succeeded. Scoping to known ids makes this helper independent of
	// whatever else is in the database.
	for _, statement := range []string{
		`UPDATE haulage_bookings SET active_recovery_id=NULL WHERE id=$1`,
		`DELETE FROM hauling_payment_rebind_obligations WHERE booking_id=$1`,
		`DELETE FROM hauling_recovery_cases WHERE booking_id=$1`,
		`DELETE FROM booking_events WHERE booking_id=$1`,
		`DELETE FROM haulage_bookings WHERE id=$1`,
	} {
		if _, err := pool.Exec(ctx, statement, bookingID); err != nil {
			t.Fatalf("clean %q: %v", statement, err)
		}
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM hauling_domain_outbox WHERE aggregate_type='recovery_case'`); err != nil {
		t.Fatalf("clean outbox: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM trucks WHERE id = ANY($1::uuid[])`,
		[]string{truckA, truckB, truckC}); err != nil {
		t.Fatalf("clean trucks: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM truck_providers WHERE id = ANY($1::uuid[])`,
		[]string{providerA, providerB, providerC}); err != nil {
		t.Fatalf("clean providers: %v", err)
	}

	for i, id := range []string{providerA, providerB, providerC} {
		if _, err := pool.Exec(ctx, `
INSERT INTO truck_providers (id, phone, first_name, last_name, status)
VALUES ($1, $2, 'Test', 'Provider', 'active')`, id, "+23480111110"+string(rune('0'+i))); err != nil {
			t.Fatalf("seed provider: %v", err)
		}
	}
	// providerA's truck fits. providerB's is far too small. providerC's fits.
	for _, truck := range []struct {
		id, provider, plate string
		payloadGrams        int64
	}{
		{truckA, providerA, "REC-A", 20_000_000},
		{truckB, providerB, "REC-B", 100_000},
		{truckC, providerC, "REC-C", 20_000_000},
	} {
		if _, err := pool.Exec(ctx, `
INSERT INTO trucks (id, provider_id, truck_type, capacity_kg, plate_number, status, max_payload_grams)
VALUES ($1,$2,'flatbed',$3,$4,'active',$5)`,
			truck.id, truck.provider, int(truck.payloadGrams/1000), truck.plate, truck.payloadGrams); err != nil {
			t.Fatalf("seed truck: %v", err)
		}
	}

	if _, err := pool.Exec(ctx, `
INSERT INTO haulage_bookings (id, customer_id, provider_id, truck_id, pickup_address, pickup_lat, pickup_lng,
	dropoff_address, dropoff_lat, dropoff_lng, cargo_type, cargo_weight_kg, status, preferred_truck_type)
VALUES ($1,'dddddddd-0000-4000-8000-000000000001',$2,$3,'Surulere',6.50,3.35,'VI',6.43,3.42,'general',5000,$4,'flatbed')`,
		bookingID, providerA, truckA, status); err != nil {
		t.Fatalf("seed booking: %v", err)
	}
	return bookingID
}

// fakeCandidates offers providers in a fixed order.
type fakeCandidates struct {
	providers []NearbyProvider
	err       error
}

func (f fakeCandidates) NearbyProviders(context.Context, geo.Point, float64, int) ([]NearbyProvider, error) {
	return f.providers, f.err
}

// fakeCapability answers from the real trucks table shape but under test
// control, so a test can make a store outage happen on demand.
type fakeCapability struct {
	fits map[string][]cargofit.TruckCapability
	err  error
}

func (f fakeCapability) LoadProviderTruckFits(
	_ context.Context, providerIDs []string, _ cargofit.Requirement,
) (map[string][]cargofit.TruckCapability, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string][]cargofit.TruckCapability{}
	for _, id := range providerIDs {
		if trucks, ok := f.fits[id]; ok {
			out[id] = trucks
		}
	}
	return out, nil
}

func (f fakeCapability) ProductForRequirement(cargofit.Requirement) (string, bool) {
	return "HAUL_GENERAL", false
}

func fittingTruck(id, provider string) cargofit.TruckCapability {
	return cargofit.TruckCapability{
		TruckID: id, ProviderID: provider, TruckType: "flatbed",
		Active: true, Verified: true,
		MaxPayloadGrams: 20_000_000, MaxPayloadGramsKnown: true,
		CapabilityVersion: 2,
	}
}

func serviceFor(t *testing.T, pool *pgxpool.Pool, candidates CandidateSource, capability CapabilitySource) *Service {
	t.Helper()
	return New(Options{
		Repository: NewRepository(pool),
		Candidates: candidates,
		Capability: capability,
		Custody:    custodyops.NewRepository(pool, "HAULING"),
		Events:     NewOutboxSink(pool, "analytics-service", true, nil),
		Enabled:    true,
		Now:        func() time.Time { return time.Now().UTC() },
	})
}

func nearby(ids ...string) fakeCandidates {
	out := make([]NearbyProvider, 0, len(ids))
	for i, id := range ids {
		out = append(out, NearbyProvider{ProviderID: id, DistanceKm: float64(i + 1)})
	}
	return fakeCandidates{providers: out}
}
