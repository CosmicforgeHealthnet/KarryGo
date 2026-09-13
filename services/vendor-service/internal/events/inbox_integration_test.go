package events

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
)

func eventIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = database.ApplyMigrations(context.Background(), pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestInboxClaimIsTransactionalReplayableAndFingerprintSafe(t *testing.T) {
	pool := eventIntegrationPool(t)
	ctx := context.Background()
	repo := NewInboxRepository(pool)
	eventID := "evt_" + uuid.NewString()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimTx(ctx, tx, "wallet", eventID, "payment.settled", "fp-a")
	if err != nil || !claimed {
		t.Fatalf("first claim=%t err=%v", claimed, err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err = repo.ClaimTx(ctx, tx, "wallet", eventID, "payment.settled", "fp-a")
	if err != nil || !claimed {
		t.Fatalf("claim after rollback=%t err=%v", claimed, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	claimed, err = repo.Claim(ctx, "wallet", eventID, "payment.settled")
	if err != nil || claimed {
		t.Fatalf("committed duplicate claim=%t err=%v", claimed, err)
	}
	claimed, err = repo.ClaimTx(ctx, pool, "wallet", eventID, "payment.settled", "fp-b")
	if claimed || !errors.Is(err, ErrPayloadMismatch) {
		t.Fatalf("payload identity conflict claim=%t err=%v", claimed, err)
	}
}

func TestInboxRejectsFingerprintWhenOriginalClaimHadNone(t *testing.T) {
	pool := eventIntegrationPool(t)
	ctx := context.Background()
	repo := NewInboxRepository(pool)
	eventID := "evt_" + uuid.NewString()

	claimed, err := repo.Claim(ctx, "wallet", eventID, "payment.settled")
	if err != nil || !claimed {
		t.Fatalf("unfingerprinted first claim=%t err=%v", claimed, err)
	}
	claimed, err = repo.ClaimTx(ctx, pool, "wallet", eventID, "payment.settled", "fp-late")
	if claimed || !errors.Is(err, ErrPayloadMismatch) {
		t.Fatalf("late fingerprint claim=%t err=%v", claimed, err)
	}
}

func TestOutboxAppendIsTransactionalAndMetricsExposeBacklog(t *testing.T) {
	pool := eventIntegrationPool(t)
	ctx := context.Background()
	repo := NewOutboxRepository(pool)
	envelope, err := NewEnvelope(
		"vendor.catalog.catalogued.v1", 1, "Vendor", "vendor-ref", 7,
		"vendor-ref", "correlation-44", "causation-44",
		map[string]any{"safe_ref": "opaque"}, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Append(ctx, tx, envelope); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE id=$1`, envelope.EventID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rolled-back outbox count=%d err=%v", count, err)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Append(ctx, tx, envelope); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var schemaVersion int
	var correlationID, causationID string
	if err = pool.QueryRow(ctx, `SELECT schema_version,correlation_id,causation_id FROM vendor_outbox WHERE id=$1`, envelope.EventID).
		Scan(&schemaVersion, &correlationID, &causationID); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != 1 || correlationID != "correlation-44" || causationID != "causation-44" {
		t.Fatalf("outbox envelope lost contract: version=%d correlation=%q causation=%q", schemaVersion, correlationID, causationID)
	}
	metrics, err := repo.Metrics(ctx)
	if err != nil || metrics.PendingCount < 1 || metrics.OldestPendingSeconds < 0 {
		t.Fatalf("outbox metrics=%+v err=%v", metrics, err)
	}
}
