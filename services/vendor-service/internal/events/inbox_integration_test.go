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
