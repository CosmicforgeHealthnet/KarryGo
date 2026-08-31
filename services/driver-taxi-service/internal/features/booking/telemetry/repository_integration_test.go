package bookingtelemetry

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func telemetryTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for telemetry repository integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func mustExec(t *testing.T, db *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// seedTelemetryFixture inserts a market/zone/provider/booking triple so
// taxi_trip_location_points' foreign keys have something real to point at
// -- this table has no meaning detached from a booking+provider pair.
func seedTelemetryFixture(t *testing.T, db *pgxpool.Pool) (bookingID, providerID string) {
	t.Helper()
	marketID := uuid.NewString()
	zoneID := uuid.NewString()
	providerID = uuid.NewString()
	bookingID = uuid.NewString()
	code := "TEL-" + uuid.NewString()[:8]

	mustExec(t, db, `INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Test Market', $2)`, marketID, code)
	mustExec(t, db, `
		INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Test Zone', $3, 1.0, -10.0, 10, '{economy}')
	`, zoneID, marketID, code)
	mustExec(t, db, `INSERT INTO taxi_providers (id, phone) VALUES ($1, $2)`, providerID, "+234"+providerID[:10])
	mustExec(t, db, `
		INSERT INTO taxi_bookings (id, customer_id, provider_id, market_id, operating_zone_id, vehicle_class, status, pricing_quote_reference)
		VALUES ($1, $2, $3, $4, $5, 'economy', 'in_progress', 'quote-ref-test')
	`, bookingID, uuid.NewString(), providerID, marketID, zoneID)

	t.Cleanup(func() {
		cctx := context.Background()
		_, _ = db.Exec(cctx, `DELETE FROM taxi_trip_location_points WHERE booking_id = $1`, bookingID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_bookings WHERE id = $1`, bookingID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_providers WHERE id = $1`, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM markets WHERE id = $1`, marketID)
	})
	return bookingID, providerID
}

func TestPostgresRepository_InsertThenLatestRoundTrips(t *testing.T) {
	db := telemetryTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, providerID := seedTelemetryFixture(t, db)

	now := time.Now().UTC().Truncate(time.Millisecond)
	p := Point{
		BookingID: bookingID, ProviderID: providerID, Sequence: 1,
		Lat: 6.5244, Lng: 3.3792, RecordedAt: now, ReceivedAt: now, Source: "gps",
	}
	inserted, err := repo.Insert(context.Background(), p)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if !inserted {
		t.Fatal("inserted = false, want true for a brand new point")
	}

	got, ok, err := repo.Latest(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true after an insert")
	}
	if got.Sequence != 1 || got.Lat != p.Lat || got.Lng != p.Lng {
		t.Errorf("Latest = %+v, want it to match the inserted point", got)
	}
}

func TestPostgresRepository_LatestReturnsFalseWhenNothingStored(t *testing.T) {
	db := telemetryTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, providerID := seedTelemetryFixture(t, db)

	_, ok, err := repo.Latest(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if ok {
		t.Error("ok = true, want false when no point has ever been stored")
	}
}

func TestPostgresRepository_LatestReturnsHighestSequence(t *testing.T) {
	db := telemetryTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, providerID := seedTelemetryFixture(t, db)
	now := time.Now().UTC()

	for _, seq := range []int64{1, 3, 2} { // deliberately not inserted in order
		mustInsert(t, repo, Point{BookingID: bookingID, ProviderID: providerID, Sequence: seq, Lat: 6.5, Lng: 3.4, RecordedAt: now, ReceivedAt: now, Source: "gps"})
	}

	got, ok, err := repo.Latest(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if !ok || got.Sequence != 3 {
		t.Errorf("Latest sequence = %d (ok=%v), want 3", got.Sequence, ok)
	}
}

func TestPostgresRepository_InsertIsIdempotentOnSameSequence(t *testing.T) {
	db := telemetryTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, providerID := seedTelemetryFixture(t, db)
	now := time.Now().UTC()

	first := Point{BookingID: bookingID, ProviderID: providerID, Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: now, ReceivedAt: now, Source: "gps"}
	inserted, err := repo.Insert(context.Background(), first)
	if err != nil || !inserted {
		t.Fatalf("first Insert: inserted=%v err=%v", inserted, err)
	}

	// A resend of the same (booking_id, provider_id, sequence) with a
	// DIFFERENT lat/lng must be silently ignored -- the original row is the
	// source of truth for what was actually received first, never
	// overwritten by a later resend.
	resend := first
	resend.Lat, resend.Lng = 9.9, 9.9
	inserted, err = repo.Insert(context.Background(), resend)
	if err != nil {
		t.Fatalf("second Insert: %v", err)
	}
	if inserted {
		t.Error("inserted = true on a resend of the same sequence, want false")
	}

	got, _, err := repo.Latest(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if got.Lat != 6.5 || got.Lng != 3.4 {
		t.Errorf("stored point = (%v,%v), want the ORIGINAL (6.5,3.4), not the resend's coordinates", got.Lat, got.Lng)
	}
}

func TestPostgresRepository_FlaggedPointIsStillStored(t *testing.T) {
	db := telemetryTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, providerID := seedTelemetryFixture(t, db)
	now := time.Now().UTC()

	flagged := Point{
		BookingID: bookingID, ProviderID: providerID, Sequence: 1,
		Lat: 6.5, Lng: 3.4, RecordedAt: now, ReceivedAt: now, Source: "gps",
		IntegrityFlags: []string{string(FlagImpossibleJump)},
	}
	inserted, err := repo.Insert(context.Background(), flagged)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if !inserted {
		t.Fatal("inserted = false, want true -- a flagged point must still be durably stored")
	}

	got, ok, err := repo.Latest(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if !ok || len(got.IntegrityFlags) != 1 || got.IntegrityFlags[0] != string(FlagImpossibleJump) {
		t.Errorf("stored flags = %v, want [impossible_jump] preserved", got.IntegrityFlags)
	}
}

func TestPostgresRepository_ValidatedTrailExcludesFlaggedPoints(t *testing.T) {
	db := telemetryTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, providerID := seedTelemetryFixture(t, db)
	now := time.Now().UTC()

	mustInsert(t, repo, Point{BookingID: bookingID, ProviderID: providerID, Sequence: 1, Lat: 6.50, Lng: 3.40, RecordedAt: now, ReceivedAt: now, Source: "gps"})
	mustInsert(t, repo, Point{BookingID: bookingID, ProviderID: providerID, Sequence: 2, Lat: 99.0, Lng: 99.0, RecordedAt: now, ReceivedAt: now, Source: "gps", IntegrityFlags: []string{string(FlagImpossibleJump)}})
	mustInsert(t, repo, Point{BookingID: bookingID, ProviderID: providerID, Sequence: 3, Lat: 6.52, Lng: 3.42, RecordedAt: now, ReceivedAt: now, Source: "gps"})

	trail, err := repo.ValidatedTrail(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("ValidatedTrail: %v", err)
	}
	if len(trail) != 2 {
		t.Fatalf("len(trail) = %d, want 2 (the flagged point 2 excluded)", len(trail))
	}
	if trail[0].Sequence != 1 || trail[1].Sequence != 3 {
		t.Errorf("trail sequences = [%d,%d], want [1,3] in order, flagged point 2 skipped", trail[0].Sequence, trail[1].Sequence)
	}
}

func TestPostgresRepository_ValidatedTrailEmptyWhenNothingClean(t *testing.T) {
	db := telemetryTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, providerID := seedTelemetryFixture(t, db)
	now := time.Now().UTC()

	mustInsert(t, repo, Point{BookingID: bookingID, ProviderID: providerID, Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: now, ReceivedAt: now, Source: "gps", IntegrityFlags: []string{string(FlagStale)}})

	trail, err := repo.ValidatedTrail(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("ValidatedTrail: %v", err)
	}
	if len(trail) != 0 {
		t.Errorf("len(trail) = %d, want 0 when every stored point is flagged", len(trail))
	}
}

func mustInsert(t *testing.T, repo *PostgresRepository, p Point) {
	t.Helper()
	if _, err := repo.Insert(context.Background(), p); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}
