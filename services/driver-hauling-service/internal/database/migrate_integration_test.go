package database

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyMigrationsTracksAndReplays(t *testing.T) {
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required for PostgreSQL migration tests")
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	serviceRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(serviceRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := ApplyMigrations(context.Background(), pool); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := ApplyMigrations(context.Background(), pool); err != nil {
		t.Fatalf("replay apply: %v", err)
	}
	var applied int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM hauling_schema_migrations`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 12 {
		t.Fatalf("applied migrations = %d, want 12", applied)
	}
	var exists bool
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='hauling_organization_provider_links' AND column_name='link_id')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("provider link lifecycle migration was not applied")
	}
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='hauling_private_execution_mappings')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("private execution authority migration was not applied")
	}
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='hauling_authority_restrictions')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("restriction authority migration was not applied")
	}
}
