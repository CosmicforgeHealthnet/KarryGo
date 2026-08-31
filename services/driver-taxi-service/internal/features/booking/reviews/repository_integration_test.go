package bookingreviews

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func reviewsTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for booking_reviews repository integration tests")
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

// seedReviewsFixture inserts a market/zone/provider/completed-booking
// quadruple so booking_reviews' foreign keys are satisfiable.
func seedReviewsFixture(t *testing.T, db *pgxpool.Pool) (bookingID, customerID, providerID string) {
	t.Helper()
	marketID := uuid.NewString()
	zoneID := uuid.NewString()
	providerID = uuid.NewString()
	customerID = uuid.NewString()
	bookingID = uuid.NewString()
	code := "REV-" + uuid.NewString()[:8]

	mustExec(t, db, `INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Test Market', $2)`, marketID, code)
	mustExec(t, db, `
		INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Test Zone', $3, 1.0, -10.0, 10, '{economy}')
	`, zoneID, marketID, code)
	mustExec(t, db, `INSERT INTO taxi_providers (id, phone) VALUES ($1, $2)`, providerID, "+234"+providerID[:10])
	mustExec(t, db, `
		INSERT INTO taxi_bookings (id, customer_id, provider_id, market_id, operating_zone_id, vehicle_class, status, pricing_quote_reference)
		VALUES ($1, $2, $3, $4, $5, 'economy', 'completed', 'quote-ref-test')
	`, bookingID, customerID, providerID, marketID, zoneID)

	t.Cleanup(func() {
		cctx := context.Background()
		_, _ = db.Exec(cctx, `DELETE FROM booking_reviews WHERE booking_id = $1`, bookingID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_bookings WHERE id = $1`, bookingID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_providers WHERE id = $1`, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM markets WHERE id = $1`, marketID)
	})
	return bookingID, customerID, providerID
}

func TestPostgresRepository_CreateThenGetForBookingRoundTrips(t *testing.T) {
	db := reviewsTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, customerID, providerID := seedReviewsFixture(t, db)

	recommends := true
	created, err := repo.Create(context.Background(), Review{
		BookingID: bookingID, Direction: DirectionCustomerToProvider,
		CustomerID: customerID, ProviderID: providerID,
		Rating: 5, ReviewText: "Great ride", Tags: []string{"clean_car", "safe_driving"},
		RecommendsDriver: &recommends,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Error("created.ID is empty, want a generated UUID")
	}

	all, err := repo.GetForBooking(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("GetForBooking: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("len(all) = %d, want 1", len(all))
	}
	if all[0].Rating != 5 || len(all[0].Tags) != 2 || all[0].RecommendsDriver == nil || !*all[0].RecommendsDriver {
		t.Errorf("stored review = %+v, want it to round-trip exactly", all[0])
	}
}

func TestPostgresRepository_CreateWithNilTagsSucceeds(t *testing.T) {
	db := reviewsTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, customerID, providerID := seedReviewsFixture(t, db)

	// tags is NOT NULL DEFAULT '{}' -- a Go nil []string must not violate
	// that constraint (the exact bug already found and fixed once in
	// bookingtelemetry.Insert; this proves the same fix applied here works).
	_, err := repo.Create(context.Background(), Review{
		BookingID: bookingID, Direction: DirectionCustomerToProvider,
		CustomerID: customerID, ProviderID: providerID, Rating: 4,
	})
	if err != nil {
		t.Fatalf("Create with nil Tags: %v", err)
	}
}

func TestPostgresRepository_DuplicateDirectionIsRejectedByRealUniqueConstraint(t *testing.T) {
	db := reviewsTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, customerID, providerID := seedReviewsFixture(t, db)

	rev := Review{BookingID: bookingID, Direction: DirectionCustomerToProvider, CustomerID: customerID, ProviderID: providerID, Rating: 5}
	if _, err := repo.Create(context.Background(), rev); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err := repo.Create(context.Background(), rev)
	if err == nil {
		t.Fatal("expected the second Create over the same (booking_id, direction) to fail")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Errorf("err = %v, want a real Postgres unique-violation (23505) -- this is what bookingusecases.submitReview maps to a friendly Conflict", err)
	}
}

func TestPostgresRepository_BothDirectionsCoexist(t *testing.T) {
	db := reviewsTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, customerID, providerID := seedReviewsFixture(t, db)

	if _, err := repo.Create(context.Background(), Review{
		BookingID: bookingID, Direction: DirectionCustomerToProvider, CustomerID: customerID, ProviderID: providerID, Rating: 5,
	}); err != nil {
		t.Fatalf("customer->provider Create: %v", err)
	}
	if _, err := repo.Create(context.Background(), Review{
		BookingID: bookingID, Direction: DirectionProviderToCustomer, CustomerID: customerID, ProviderID: providerID, Rating: 3,
	}); err != nil {
		t.Fatalf("provider->customer Create: %v", err)
	}

	all, err := repo.GetForBooking(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("GetForBooking: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(all) = %d, want 2 (both directions)", len(all))
	}
}

func TestPostgresRepository_GetForBookingEmptyWhenNoneSubmitted(t *testing.T) {
	db := reviewsTestDB(t)
	repo := NewPostgresRepository(db)
	bookingID, _, _ := seedReviewsFixture(t, db)

	all, err := repo.GetForBooking(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("GetForBooking: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("len(all) = %d, want 0", len(all))
	}
}
