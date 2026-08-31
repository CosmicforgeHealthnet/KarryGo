package verification

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/handover"
)

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for pickup verification repository tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seedBookingForVerification inserts a market/zone/provider/booking triple
// so a taxi_pickup_verifications row has a valid FK target (booking_id
// REFERENCES taxi_bookings(id) ON DELETE CASCADE).
func seedBookingForVerification(t *testing.T, db *pgxpool.Pool) (bookingID string) {
	t.Helper()
	marketID := uuid.NewString()
	zoneID := uuid.NewString()
	providerID := uuid.NewString()
	bookingID = uuid.NewString()
	code := "VERIFY-" + uuid.NewString()[:8]

	mustExec(t, db, `INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Test Market', $2)`, marketID, code)
	mustExec(t, db, `
		INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Test Zone', $3, 1.0, -10.0, 10, '{economy}')
	`, zoneID, marketID, code)
	mustExec(t, db, `INSERT INTO taxi_providers (id, phone) VALUES ($1, $2)`, providerID, "+234"+providerID[:10])
	mustExec(t, db, `
		INSERT INTO taxi_bookings (id, customer_id, provider_id, market_id, operating_zone_id, vehicle_class, status, pricing_quote_reference)
		VALUES ($1, $2, $3, $4, $5, 'economy', 'arrived_at_pickup', 'quote-ref-test')
	`, bookingID, uuid.NewString(), providerID, marketID, zoneID)

	t.Cleanup(func() {
		cctx := context.Background()
		_, _ = db.Exec(cctx, `DELETE FROM taxi_bookings WHERE market_id = $1 OR provider_id = $2`, marketID, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_providers WHERE id = $1`, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM markets WHERE id = $1`, marketID)
	})
	return bookingID
}

func mustExec(t *testing.T, db *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestUpsert_ThenGet_RoundTrips(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresRepository(db)
	bookingID := seedBookingForVerification(t, db)

	rec, err := handover.NewCodeRecord([]byte("test-secret"), handover.BuildChallengeID("taxi", handover.StepPickup, bookingID), time.Now())
	if err != nil {
		t.Fatalf("NewCodeRecord: %v", err)
	}
	if err := repo.Upsert(context.Background(), nil, Record{BookingID: bookingID, CodeRecord: rec}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.Get(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Code != rec.Code {
		t.Errorf("Code = %q, want %q", got.Code, rec.Code)
	}
	if got.Hash != rec.Hash {
		t.Errorf("Hash mismatch")
	}
	if got.Attempts != 0 || got.ResendCount != 0 {
		t.Errorf("fresh record should have zero attempts/resends, got attempts=%d resends=%d", got.Attempts, got.ResendCount)
	}
	if got.VerifiedAt != nil {
		t.Error("fresh record should not be verified")
	}
}

func TestUpsert_OnConflictReplacesTheRecord(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresRepository(db)
	bookingID := seedBookingForVerification(t, db)
	secret := []byte("test-secret")
	challengeID := handover.BuildChallengeID("taxi", handover.StepPickup, bookingID)

	first, _ := handover.NewCodeRecord(secret, challengeID, time.Now())
	if err := repo.Upsert(context.Background(), nil, Record{BookingID: bookingID, CodeRecord: first}); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	// Simulate a resend: a fresh code, carrying forward resend bookkeeping.
	second, _ := handover.NewCodeRecord(secret, challengeID, time.Now())
	second.ResendCount = 1
	now := time.Now()
	second.LastSentAt = &now
	if err := repo.Upsert(context.Background(), nil, Record{BookingID: bookingID, CodeRecord: second}); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	got, err := repo.Get(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Code != second.Code {
		t.Errorf("Code = %q, want the SECOND (resent) code %q -- the old one must no longer verify", got.Code, second.Code)
	}
	if got.ResendCount != 1 {
		t.Errorf("ResendCount = %d, want 1", got.ResendCount)
	}
	if !handover.VerifyCode(secret, challengeID, second.Code, got.Hash) {
		t.Error("the resent code should verify against the stored hash")
	}
	if handover.VerifyCode(secret, challengeID, first.Code, got.Hash) && first.Code != second.Code {
		t.Error("the OLD code should no longer verify after a resend")
	}
}

func TestGet_NotFoundBeforeArrival(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresRepository(db)

	_, err := repo.Get(context.Background(), uuid.NewString())
	if err == nil {
		t.Fatal("expected an error for a booking with no generated code yet")
	}
	if appErr := apperrors.From(err); appErr == nil || appErr.Code != apperrors.CodeNotFound {
		t.Fatalf("expected apperrors.NotFound, got %v", err)
	}
}

func TestIncrementAttempts_IsAtomicAndCumulative(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresRepository(db)
	bookingID := seedBookingForVerification(t, db)
	rec, _ := handover.NewCodeRecord([]byte("s"), handover.BuildChallengeID("taxi", handover.StepPickup, bookingID), time.Now())
	if err := repo.Upsert(context.Background(), nil, Record{BookingID: bookingID, CodeRecord: rec}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	for i := 1; i <= 3; i++ {
		if err := repo.IncrementAttempts(context.Background(), bookingID); err != nil {
			t.Fatalf("IncrementAttempts[%d]: %v", i, err)
		}
	}

	got, err := repo.Get(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", got.Attempts)
	}
}

func TestMarkVerified_SetsVerifiedAt(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresRepository(db)
	bookingID := seedBookingForVerification(t, db)
	rec, _ := handover.NewCodeRecord([]byte("s"), handover.BuildChallengeID("taxi", handover.StepPickup, bookingID), time.Now())
	if err := repo.Upsert(context.Background(), nil, Record{BookingID: bookingID, CodeRecord: rec}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	at := time.Now()
	if err := repo.MarkVerified(context.Background(), nil, bookingID, at); err != nil {
		t.Fatalf("MarkVerified: %v", err)
	}

	got, err := repo.Get(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.VerifiedAt == nil {
		t.Fatal("expected VerifiedAt to be set")
	}
}
