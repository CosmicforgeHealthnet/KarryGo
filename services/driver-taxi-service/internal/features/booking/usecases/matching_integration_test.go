package bookingusecases

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	marketrepositories "cosmicforge/logistics/services/taxi-service/internal/features/markets/repositories"
	providerauthrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/repositories"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
)

// This file tests findEligibleCandidates -- the synchronous, deterministic
// hard-filter logic -- against REAL Postgres and REAL Redis. The full async
// matchBooking loop (offer/wait/timeout/requeue) is intentionally NOT
// covered here: waiting on a background goroutine's timing in a unit test
// is exactly the kind of flaky-by-construction test to avoid, and the loop
// is proven end-to-end against the actual running service instead (see the
// live verification in this session's work). What IS deterministic and
// worth a real test is whether a candidate that fails compliance,
// operational-state, or rejection-set checks is correctly excluded BEFORE
// scoring ever runs.

func matchingTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for matching integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func matchingTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TAXI_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TAXI_TEST_REDIS_ADDR is required for matching integration tests")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// seedCompliantOnlineProvider inserts a fully compliant taxi_providers row
// (every CanGoAvailable dimension satisfied), an AVAILABLE
// provider_operational_state row, and a matching Redis online/geo-index
// entry -- everything findEligibleCandidates checks, all real.
func seedCompliantOnlineProvider(t *testing.T, db *pgxpool.Pool, redisClient *redis.Client, marketID string, lat, lng float64) string {
	t.Helper()
	ctx := context.Background()
	providerID := uuid.NewString()

	if _, err := db.Exec(ctx, `
		INSERT INTO taxi_providers (
			id, phone, account_status, onboarding_status,
			identity_verification_status, vehicle_verification_status, document_compliance_status, rating
		) VALUES ($1, $2, 'active', 'complete', 'verified', 'verified', 'compliant', 4.8)
	`, providerID, "+234"+providerID[:10]); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO provider_operational_state (provider_id, state, version) VALUES ($1, 'AVAILABLE', 1)
	`, providerID); err != nil {
		t.Fatalf("seed operational state: %v", err)
	}

	store := availabilityrepositories.NewRedisAvailabilityStore(redisClient)
	if err := store.SetOnline(ctx, availabilityrepositories.ProviderStatus{
		ProviderID: providerID, MarketID: marketID, VehicleClass: "economy", Lat: lat, Lng: lng,
	}); err != nil {
		t.Fatalf("SetOnline: %v", err)
	}

	t.Cleanup(func() {
		cctx := context.Background()
		_ = store.SetOffline(cctx, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM provider_operational_state_history WHERE provider_id = $1`, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM provider_operational_state WHERE provider_id = $1`, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_providers WHERE id = $1`, providerID)
	})
	return providerID
}

func newMatchingService(t *testing.T, db *pgxpool.Pool, redisClient *redis.Client) (*BookingService, string, string) {
	t.Helper()
	marketID, zoneID := seedFixtureForMatching(t, db)
	bookings := bookingrepositories.NewPostgresBookingRepository(db)
	opState := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	availability := availabilityrepositories.NewRedisAvailabilityStore(redisClient)
	// The real Postgres provider repository already implements
	// GetByID(ctx, id) (providerauthmodels.Provider, error), which
	// structurally satisfies ProviderComplianceLookup -- no wrapper needed.
	providers := providerauthrepositories.NewPostgresProviderRepository(db)
	svc := NewBookingService(Options{
		Bookings: bookings, Markets: marketrepositories.NewPostgresMarketRepository(db),
		Pricing: newFakePricing(), OperationalState: opState,
		Availability: availability, Providers: providers, Vehicles: nil, Routes: nil,
	})
	return svc, marketID, zoneID
}

func TestFindEligibleCandidates_ExcludesNonEligibleOperationalState(t *testing.T) {
	db := matchingTestDB(t)
	redisClient := matchingTestRedis(t)
	svc, marketID, _ := newMatchingService(t, db, redisClient)

	providerID := seedCompliantOnlineProvider(t, db, redisClient, marketID, 1.0005, -9.9995)
	// Flip to ON_TRIP after seeding as AVAILABLE -- a provider mid-ride must
	// never be offered a second booking, even though they are still present
	// in the Redis geo index (nothing removes them from it mid-trip in this
	// build; the durable Postgres operational state is the actual guard).
	if _, err := db.Exec(context.Background(),
		`UPDATE provider_operational_state SET state = 'ON_TRIP', version = version + 1 WHERE provider_id = $1`, providerID); err != nil {
		t.Fatalf("flip to ON_TRIP: %v", err)
	}

	booking := bookingmodels.Booking{
		ID: uuid.NewString(), MarketID: marketID, VehicleClass: "economy",
		Stops: []bookingmodels.Stop{{Lat: 1.0, Lng: -10.0}},
	}
	candidates := svc.findEligibleCandidates(context.Background(), booking, 15)
	for _, c := range candidates {
		if c.ProviderID == providerID {
			t.Fatal("an ON_TRIP provider was offered a new booking -- operational state must be re-checked against Postgres, not just Redis presence")
		}
	}
}

func TestFindEligibleCandidates_ExcludesNonCompliantProvider(t *testing.T) {
	db := matchingTestDB(t)
	redisClient := matchingTestRedis(t)
	svc, marketID, _ := newMatchingService(t, db, redisClient)

	providerID := seedCompliantOnlineProvider(t, db, redisClient, marketID, 1.0005, -9.9995)
	// Suspend the account after seeding compliant -- CanGoAvailable must
	// reject this candidate even though their operational state and Redis
	// presence both still say "available".
	if _, err := db.Exec(context.Background(),
		`UPDATE taxi_providers SET account_status = 'suspended' WHERE id = $1`, providerID); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	booking := bookingmodels.Booking{
		ID: uuid.NewString(), MarketID: marketID, VehicleClass: "economy",
		Stops: []bookingmodels.Stop{{Lat: 1.0, Lng: -10.0}},
	}
	candidates := svc.findEligibleCandidates(context.Background(), booking, 15)
	for _, c := range candidates {
		if c.ProviderID == providerID {
			t.Fatal("a suspended provider was offered a booking")
		}
	}
}

func TestFindEligibleCandidates_ExcludesRejectedForThisBooking(t *testing.T) {
	db := matchingTestDB(t)
	redisClient := matchingTestRedis(t)
	svc, marketID, _ := newMatchingService(t, db, redisClient)

	providerID := seedCompliantOnlineProvider(t, db, redisClient, marketID, 1.0005, -9.9995)
	bookingID := uuid.NewString()
	availability := availabilityrepositories.NewRedisAvailabilityStore(redisClient)
	if err := availability.MarkRejected(context.Background(), bookingID, providerID, time.Minute); err != nil {
		t.Fatalf("MarkRejected: %v", err)
	}

	booking := bookingmodels.Booking{
		ID: bookingID, MarketID: marketID, VehicleClass: "economy",
		Stops: []bookingmodels.Stop{{Lat: 1.0, Lng: -10.0}},
	}
	candidates := svc.findEligibleCandidates(context.Background(), booking, 15)
	for _, c := range candidates {
		if c.ProviderID == providerID {
			t.Fatal("a provider already rejected for THIS booking was offered it again")
		}
	}
}

func TestFindEligibleCandidates_IncludesAFullyEligibleProvider(t *testing.T) {
	db := matchingTestDB(t)
	redisClient := matchingTestRedis(t)
	svc, marketID, _ := newMatchingService(t, db, redisClient)

	providerID := seedCompliantOnlineProvider(t, db, redisClient, marketID, 1.0005, -9.9995)

	booking := bookingmodels.Booking{
		ID: uuid.NewString(), MarketID: marketID, VehicleClass: "economy",
		Stops: []bookingmodels.Stop{{Lat: 1.0, Lng: -10.0}},
	}
	candidates := svc.findEligibleCandidates(context.Background(), booking, 15)
	found := false
	for _, c := range candidates {
		if c.ProviderID == providerID {
			found = true
		}
	}
	if !found {
		t.Fatal("a fully compliant, available, nearby, non-rejected provider was NOT offered the booking")
	}
}

// seedFixtureForMatching mirrors the repository package's own seedFixture
// (a throwaway market/zone at synthetic mid-Atlantic coordinates -- these
// tests never resolve a zone by real-world coordinate collision risk).
// Registers its own cleanup: bookings created against this market first
// (RESTRICT on taxi_bookings' market/zone references, by design -- see the
// repository package's seedFixture doc comment for why), then the
// market/zone themselves (operating_zones cascades).
func seedFixtureForMatching(t *testing.T, db *pgxpool.Pool) (marketID, zoneID string) {
	t.Helper()
	ctx := context.Background()
	marketID = uuid.NewString()
	zoneID = uuid.NewString()
	code := "MATCH-" + uuid.NewString()[:8]
	if _, err := db.Exec(ctx, `INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Test Market', $2)`, marketID, code); err != nil {
		t.Fatalf("seed market: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Test Zone', $3, 1.0, -10.0, 15, '{economy,comfort,xl}')
	`, zoneID, marketID, code); err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	t.Cleanup(func() {
		cctx := context.Background()
		_, _ = db.Exec(cctx, `DELETE FROM taxi_outbox WHERE aggregate_id IN (SELECT id FROM taxi_bookings WHERE market_id = $1)`, marketID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_bookings WHERE market_id = $1`, marketID)
		_, _ = db.Exec(cctx, `DELETE FROM markets WHERE id = $1`, marketID) // cascades to operating_zones
	})
	return marketID, zoneID
}

// ─── Parcel opt-in ──────────────────────────────────────────────────────────

func TestFindEligibleCandidates_ExcludesDriverWhoDeclinedParcels(t *testing.T) {
	db := matchingTestDB(t)
	redisClient := matchingTestRedis(t)
	svc, marketID, _ := newMatchingService(t, db, redisClient)

	// Fully compliant, available, nearby -- and opted OUT of parcels, which is
	// the seeded default. Only the parcel filter can exclude them.
	providerID := seedCompliantOnlineProvider(t, db, redisClient, marketID, 1.0005, -9.9995)

	parcel := bookingmodels.Booking{
		ID: uuid.NewString(), MarketID: marketID, VehicleClass: "economy",
		RidePurpose: bookingmodels.RidePurposeParcel,
		Stops:       []bookingmodels.Stop{{Lat: 1.0, Lng: -10.0}},
	}
	for _, c := range svc.findEligibleCandidates(context.Background(), parcel, 15) {
		if c.ProviderID == providerID {
			t.Fatal("a driver who never opted into parcels was offered a package")
		}
	}

	// The very same driver must still be eligible for a passenger ride --
	// proving the filter is scoped to parcels, not a blanket exclusion.
	ride := parcel
	ride.ID = uuid.NewString()
	ride.RidePurpose = bookingmodels.RidePurposePassenger
	var found bool
	for _, c := range svc.findEligibleCandidates(context.Background(), ride, 15) {
		if c.ProviderID == providerID {
			found = true
		}
	}
	if !found {
		t.Fatal("the parcel opt-out also blocked an ordinary passenger ride")
	}
}

func TestFindEligibleCandidates_IncludesDriverWhoAcceptsParcels(t *testing.T) {
	db := matchingTestDB(t)
	redisClient := matchingTestRedis(t)
	svc, marketID, _ := newMatchingService(t, db, redisClient)

	providerID := seedCompliantOnlineProvider(t, db, redisClient, marketID, 1.0005, -9.9995)
	if _, err := db.Exec(context.Background(),
		`UPDATE taxi_providers SET accepts_parcels = true WHERE id = $1`, providerID); err != nil {
		t.Fatalf("opt in: %v", err)
	}

	parcel := bookingmodels.Booking{
		ID: uuid.NewString(), MarketID: marketID, VehicleClass: "economy",
		RidePurpose: bookingmodels.RidePurposeParcel,
		Stops:       []bookingmodels.Stop{{Lat: 1.0, Lng: -10.0}},
	}
	var found bool
	for _, c := range svc.findEligibleCandidates(context.Background(), parcel, 15) {
		if c.ProviderID == providerID {
			found = true
		}
	}
	if !found {
		t.Fatal("an opted-in driver was not offered a package")
	}
}
