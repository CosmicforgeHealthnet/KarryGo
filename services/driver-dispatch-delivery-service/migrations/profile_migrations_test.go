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

func TestProfileMigrationFilesExist(t *testing.T) {
	for _, name := range []string{
		"000025_create_provider_settings.up.sql",
		"000025_create_provider_settings.down.sql",
	} {
		if _, err := os.Stat(filepath.Join(".", name)); err != nil {
			t.Fatalf("migration %s is missing: %v", name, err)
		}
	}
}

func TestProfileSettingsMigrationApplyValidateAndRollback(t *testing.T) {
	databaseURL := os.Getenv("DISPATCH_PROFILE_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_PROFILE_TEST_DATABASE_URL or TEST_DATABASE_URL to run profile migration checks")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()

	schema := "profile_migration_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE") //nolint:errcheck
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema+", public"); err != nil {
		t.Fatal(err)
	}

	execMigrationFile(t, ctx, conn, "000004_create_providers.up.sql")
	execMigrationFile(t, ctx, conn, "000025_create_provider_settings.up.sql")

	providerID := uuid.NewString()
	if _, err := conn.Exec(ctx, `INSERT INTO providers (id,phone) VALUES ($1,$2)`, providerID, "+2348000072501"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO provider_settings (provider_id) VALUES ($1)`, providerID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `UPDATE provider_settings SET language='zz' WHERE provider_id=$1`, providerID); err == nil {
		t.Fatal("invalid provider settings language accepted")
	}

	execMigrationFile(t, ctx, conn, "000025_create_provider_settings.down.sql")
	assertTableDropped(t, ctx, conn, schema, "provider_settings")
}
