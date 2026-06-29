package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAvailabilityMigrationFilesExist(t *testing.T) {
	required := []string{
		"000017_create_provider_availability.up.sql",
		"000017_create_provider_availability.down.sql",
		"000018_create_availability_sessions.up.sql",
		"000018_create_availability_sessions.down.sql",
	}
	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(".", name)); err != nil {
				t.Fatalf("migration %s is missing: %v", name, err)
			}
		})
	}
}

func TestAvailabilityMigrationsApplyValidateAndRollback(t *testing.T) {
	databaseURL := os.Getenv("DISPATCH_AVAILABILITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_AVAILABILITY_TEST_DATABASE_URL or TEST_DATABASE_URL to run database migration checks")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()

	schema := "availability_migration_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema+", public"); err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	for _, name := range []string{
		"000004_create_providers.up.sql",
		"000008_create_verification_steps.up.sql",
		"000014_create_bikes.up.sql",
		"000017_create_provider_availability.up.sql",
		"000018_create_availability_sessions.up.sql",
	} {
		execMigrationFile(t, ctx, conn, name)
	}

	assertAvailabilityIndexes(t, ctx, conn, schema)
	assertAvailabilityStatusConstraint(t, ctx, conn)
	assertOneAvailabilityRowPerProvider(t, ctx, conn)
	assertOneOpenAvailabilitySessionPerProvider(t, ctx, conn)

	for _, name := range []string{
		"000018_create_availability_sessions.down.sql",
		"000017_create_provider_availability.down.sql",
	} {
		execMigrationFile(t, ctx, conn, name)
	}
	assertTableDropped(t, ctx, conn, schema, "availability_sessions")
	assertTableDropped(t, ctx, conn, schema, "provider_availability")
}

func assertAvailabilityIndexes(t *testing.T, ctx context.Context, conn *pgxpool.Conn, schema string) {
	t.Helper()
	assertIndexExists(t, ctx, conn, schema, "idx_avail_provider")
	assertIndexExists(t, ctx, conn, schema, "idx_avail_status")
	assertIndexExists(t, ctx, conn, schema, "idx_sessions_provider")
	assertIndexExists(t, ctx, conn, schema, "idx_sessions_online_at")
	assertIndexExists(t, ctx, conn, schema, "idx_sessions_provider_open")
}

func assertAvailabilityStatusConstraint(t *testing.T, ctx context.Context, conn *pgxpool.Conn) {
	t.Helper()
	providerID := uuid.NewString()
	if _, err := conn.Exec(ctx, `INSERT INTO providers (id, phone) VALUES ($1, $2)`, providerID, "+2348000001001"); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	_, err := conn.Exec(ctx, `
		INSERT INTO provider_availability (provider_id, status)
		VALUES ($1, 'paused')
	`, providerID)
	if err == nil {
		t.Fatal("invalid availability status was accepted")
	}
}

func assertOneAvailabilityRowPerProvider(t *testing.T, ctx context.Context, conn *pgxpool.Conn) {
	t.Helper()
	providerID := uuid.NewString()
	if _, err := conn.Exec(ctx, `INSERT INTO providers (id, phone) VALUES ($1, $2)`, providerID, "+2348000001002"); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO provider_availability (provider_id) VALUES ($1)`, providerID); err != nil {
		t.Fatalf("insert availability: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO provider_availability (provider_id) VALUES ($1)`, providerID); err == nil {
		t.Fatal("duplicate provider availability row was accepted")
	}
}

func assertOneOpenAvailabilitySessionPerProvider(t *testing.T, ctx context.Context, conn *pgxpool.Conn) {
	t.Helper()
	providerID := uuid.NewString()
	if _, err := conn.Exec(ctx, `INSERT INTO providers (id, phone) VALUES ($1, $2)`, providerID, "+2348000001003"); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO availability_sessions (provider_id, went_online_at) VALUES ($1, now())`, providerID); err != nil {
		t.Fatalf("insert open session: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO availability_sessions (provider_id, went_online_at) VALUES ($1, now())`, providerID); err == nil {
		t.Fatal("second open session was accepted")
	}
	if _, err := conn.Exec(ctx, `UPDATE availability_sessions SET went_offline_at = now(), duration_minutes = 1 WHERE provider_id = $1`, providerID); err != nil {
		t.Fatalf("close open session: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO availability_sessions (provider_id, went_online_at) VALUES ($1, now())`, providerID); err != nil {
		t.Fatalf("insert second closed-after session: %v", err)
	}
}
