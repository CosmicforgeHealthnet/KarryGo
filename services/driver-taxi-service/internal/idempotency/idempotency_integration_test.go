package idempotency

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for PostgreSQL idempotency tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func cleanupKey(t *testing.T, db *pgxpool.Pool, scopeID, endpoint, key string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `
			DELETE FROM taxi_idempotency_keys WHERE scope_id = $1 AND endpoint = $2 AND idempotency_key = $3
		`, scopeID, endpoint, key)
	})
}

func TestStore_FirstUseClaimsAndReturnsNil(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, time.Hour)
	scopeID, endpoint, key := uuid.NewString(), "POST /customer/bookings", uuid.NewString()
	cleanupKey(t, db, scopeID, endpoint, key)

	record, err := store.Begin(context.Background(), scopeID, endpoint, key, "fingerprint-a")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if record != nil {
		t.Fatalf("first use should claim the key and return nil, got %+v", record)
	}
}

func TestStore_CompletedSameFingerprintReplays(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, time.Hour)
	scopeID, endpoint, key := uuid.NewString(), "POST /customer/bookings", uuid.NewString()
	cleanupKey(t, db, scopeID, endpoint, key)
	ctx := context.Background()

	if _, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a"); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := store.Complete(ctx, scopeID, endpoint, key, 201, []byte(`{"booking_id":"abc"}`)); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	record, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a")
	if err != nil {
		t.Fatalf("Begin (replay): %v", err)
	}
	if record == nil {
		t.Fatal("a completed key with the same fingerprint should replay, not re-claim")
	}
	if record.ResponseStatus != 201 || string(record.ResponseBody) != `{"booking_id":"abc"}` {
		t.Fatalf("replayed record does not match what was stored: %+v", record)
	}
}

func TestStore_DifferentFingerprintReturnsMismatch(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, time.Hour)
	scopeID, endpoint, key := uuid.NewString(), "POST /customer/bookings", uuid.NewString()
	cleanupKey(t, db, scopeID, endpoint, key)
	ctx := context.Background()

	if _, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a"); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := store.Complete(ctx, scopeID, endpoint, key, 201, []byte(`{}`)); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	_, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-b")
	if err != ErrFingerprintMismatch {
		t.Fatalf("expected ErrFingerprintMismatch, got %v", err)
	}
}

func TestStore_InProgressSameFingerprintReturnsInProgress(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, time.Hour)
	scopeID, endpoint, key := uuid.NewString(), "POST /customer/bookings", uuid.NewString()
	cleanupKey(t, db, scopeID, endpoint, key)
	ctx := context.Background()

	if _, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a"); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// Second caller races in before the first ever calls Complete.
	_, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a")
	if err != ErrInProgress {
		t.Fatalf("expected ErrInProgress, got %v", err)
	}
}

func TestStore_ConcurrentBeginOnlyOneClaimsTheKey(t *testing.T) {
	// The core guarantee: two concurrent retries of the same logical request
	// must not both execute the underlying command. Settled by the primary
	// key insert, not application-level locking.
	db := testDB(t)
	store := NewStore(db, time.Hour)
	scopeID, endpoint, key := uuid.NewString(), "POST /customer/bookings", uuid.NewString()
	cleanupKey(t, db, scopeID, endpoint, key)
	ctx := context.Background()

	const attempts = 10
	claimed := make(chan bool, attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			record, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a")
			claimed <- (err == nil && record == nil)
		}()
	}

	winners := 0
	for i := 0; i < attempts; i++ {
		if <-claimed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly 1 caller to claim the key, got %d", winners)
	}
}

func TestStore_ExpiredKeyIsReclaimed(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, 50*time.Millisecond)
	scopeID, endpoint, key := uuid.NewString(), "POST /customer/bookings", uuid.NewString()
	cleanupKey(t, db, scopeID, endpoint, key)
	ctx := context.Background()

	if _, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a"); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := store.Complete(ctx, scopeID, endpoint, key, 201, []byte(`{"v":1}`)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	time.Sleep(60 * time.Millisecond)

	// Even with a DIFFERENT fingerprint, an expired key must be reclaimed
	// rather than reported as a mismatch -- its TTL has passed, so it is
	// logically a new request, not a conflicting reuse of an old one.
	record, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-b")
	if err != nil {
		t.Fatalf("Begin after expiry: %v", err)
	}
	if record != nil {
		t.Fatalf("an expired key should be reclaimed as new, got a replay record %+v", record)
	}
}

func TestStore_ReleaseAllowsRetryAfterNonCacheableOutcome(t *testing.T) {
	db := testDB(t)
	store := NewStore(db, time.Hour)
	scopeID, endpoint, key := uuid.NewString(), "POST /customer/bookings", uuid.NewString()
	cleanupKey(t, db, scopeID, endpoint, key)
	ctx := context.Background()

	if _, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a"); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// A 503 must not be cached -- Complete should release, not store.
	if err := store.Complete(ctx, scopeID, endpoint, key, 503, []byte(`{"error":"unavailable"}`)); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	record, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a")
	if err != nil {
		t.Fatalf("Begin after a non-cacheable outcome should succeed as a fresh claim: %v", err)
	}
	if record != nil {
		t.Fatalf("a released key must not replay the failed outcome, got %+v", record)
	}
}

func TestStore_ValidationFailureIsCachedAndReplayed(t *testing.T) {
	// Per cacheable()'s own reasoning: a deterministic 4xx will fail exactly
	// the same way on retry, so it is safe -- and useful -- to replay.
	db := testDB(t)
	store := NewStore(db, time.Hour)
	scopeID, endpoint, key := uuid.NewString(), "POST /customer/bookings", uuid.NewString()
	cleanupKey(t, db, scopeID, endpoint, key)
	ctx := context.Background()

	if _, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a"); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := store.Complete(ctx, scopeID, endpoint, key, 422, []byte(`{"error":"validation_failed"}`)); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	record, err := store.Begin(ctx, scopeID, endpoint, key, "fingerprint-a")
	if err != nil {
		t.Fatalf("Begin (replay): %v", err)
	}
	if record == nil || record.ResponseStatus != 422 {
		t.Fatalf("a validation failure should be cached and replayed, got %+v", record)
	}
}
