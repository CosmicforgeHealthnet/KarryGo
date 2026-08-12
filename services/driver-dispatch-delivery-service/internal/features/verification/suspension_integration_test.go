package verification

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVerificationSuspensionIsAtomicAndIdempotent(t *testing.T) {
	dsn := os.Getenv("DISPATCH_VERIFICATION_EVENT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DISPATCH_VERIFICATION_EVENT_TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// This database is dedicated to this test through the explicit
	// DISPATCH_VERIFICATION_EVENT_TEST_DATABASE_URL contract. Reset it so the
	// migration exercise is repeatable across focused and full-suite runs.
	if _, err := pool.Exec(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset dedicated verification event test database: %v", err)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, file := range files {
		sql, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, applyErr := pool.Exec(context.Background(), string(sql)); applyErr != nil {
			t.Fatalf("apply %s: %v", filepath.Base(file), applyErr)
		}
	}
	providerID := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `INSERT INTO providers(id,phone,verification_status) VALUES($1,$2,'verified')`, providerID, "+234"+providerID[:10]); err != nil {
		t.Fatal(err)
	}
	repository := NewPostgresRepository(pool)
	input := SuspensionInput{ProviderID: providerID, ExpectedVersion: 1, ReasonCode: "COMPLIANCE_HOLD", ActorID: uuid.NewString(), CorrelationID: uuid.NewString()}
	result, err := repository.SuspendProvider(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "suspended" || result.SourceVersion != 2 {
		t.Fatalf("result=%+v", result)
	}
	replayed, err := repository.SuspendProvider(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.SourceVersion != 2 {
		t.Fatalf("replay changed version: %+v", replayed)
	}
	var events int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM dispatch_marketplace_outbox WHERE aggregate_id=$1 AND event_type=$2`, providerID, TopicVerificationSuspended).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("suspension events=%d want=1", events)
	}
	input.ReasonCode = "OTHER_HOLD"
	if _, err := repository.SuspendProvider(context.Background(), input); !errors.Is(err, ErrVerificationVersionConflict) {
		t.Fatalf("stale version error=%v", err)
	}
}
