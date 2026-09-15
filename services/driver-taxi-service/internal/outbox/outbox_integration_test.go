package outbox

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/taxi-service/internal/database"
)

var (
	migrateOnce sync.Once
	migrateErr  error
)

// testDB connects to the disposable test database and applies migrations once
// per process.
//
// Applying them here, rather than leaving the schema to whoever ran psql last,
// is what lets this suite run on a fresh database. Relying on a sibling test
// to have migrated first would make correctness depend on file naming order,
// which is the accidental coupling this harness exists to avoid.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for PostgreSQL outbox tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	migrateOnce.Do(func() {
		// ApplyMigrations reads a relative "migrations" directory, so it only
		// resolves from the service root.
		root, err := os.Getwd()
		if err != nil {
			migrateErr = err
			return
		}
		if err := os.Chdir("../.."); err != nil {
			migrateErr = err
			return
		}
		defer func() { _ = os.Chdir(root) }()
		migrateErr = database.ApplyMigrations(context.Background(), pool)
	})
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}
	return pool
}

// taxi_outbox.aggregate_id carries no foreign key -- the outbox must survive
// even if the aggregate row it describes is later archived or deleted -- so
// these tests write against arbitrary UUIDs with no booking/provider fixture
// required.
func TestWrite_InsertsAReadyToDeliverRow(t *testing.T) {
	db := testDB(t)
	aggregateID := uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM taxi_outbox WHERE aggregate_id = $1`, aggregateID)
	})

	payload, err := json.Marshal(map[string]string{"booking_id": aggregateID})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := Write(context.Background(), db, "booking", aggregateID, EventBookingCreated, payload); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var (
		eventType                        string
		gotAggregateType, gotAggregateID string
		deliveredAt, deadLetteredAt      *time.Time
		leaseOwner                       *string
		attempts                         int
		gotPayload                       []byte
	)
	err = db.QueryRow(context.Background(), `
		SELECT aggregate_type, aggregate_id::text, event_type, payload, delivered_at, dead_lettered_at, lease_owner, attempts
		FROM taxi_outbox WHERE aggregate_id = $1
	`, aggregateID).Scan(&gotAggregateType, &gotAggregateID, &eventType, &gotPayload, &deliveredAt, &deadLetteredAt, &leaseOwner, &attempts)
	if err != nil {
		t.Fatalf("query back: %v", err)
	}

	if gotAggregateType != "booking" {
		t.Errorf("aggregate_type = %q, want booking", gotAggregateType)
	}
	if eventType != EventBookingCreated {
		t.Errorf("event_type = %q, want %q", eventType, EventBookingCreated)
	}
	if deliveredAt != nil {
		t.Error("delivered_at should be NULL for a freshly written row")
	}
	if deadLetteredAt != nil {
		t.Error("dead_lettered_at should be NULL for a freshly written row")
	}
	if leaseOwner != nil {
		t.Error("lease_owner should be NULL until a publisher worker leases the row")
	}
	if attempts != 0 {
		t.Errorf("attempts = %d, want 0", attempts)
	}
	var decoded map[string]string
	if err := json.Unmarshal(gotPayload, &decoded); err != nil {
		t.Fatalf("unmarshal stored payload: %v", err)
	}
	if decoded["booking_id"] != aggregateID {
		t.Errorf("stored payload booking_id = %q, want %q", decoded["booking_id"], aggregateID)
	}
}

// TestWrite_RollsBackWithItsTransaction is the property the whole package
// exists for: an outbox row must never survive a transaction that rolls back,
// otherwise a caller could observe an event for a mutation that never
// actually committed.
func TestWrite_RollsBackWithItsTransaction(t *testing.T) {
	db := testDB(t)
	aggregateID := uuid.NewString()

	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := Write(context.Background(), tx, "booking", aggregateID, EventBookingCreated, []byte(`{}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM taxi_outbox WHERE aggregate_id = $1`, aggregateID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("row survived a rolled-back transaction: count = %d, want 0", count)
	}
}
