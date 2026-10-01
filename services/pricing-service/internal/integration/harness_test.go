// Package integration holds pricing-service's database-backed tests.
//
// They are gated on PRICING_TEST_DATABASE_URL and skip when it is unset, so
// `go test ./...` stays runnable without Postgres. CI sets it against a fresh
// database. Run them locally with:
//
//	PRICING_TEST_DATABASE_URL='postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5451/pricing_service?sslmode=disable' \
//	  go test ./internal/integration/ -v
//
// A note on cleanup: several tables here are append-only by database trigger, so
// these tests CANNOT delete what they write. Every test therefore uses unique
// identifiers and asserts on its own rows rather than on table counts. That is
// the immutability guarantee making itself felt in the test suite, which is the
// point.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/pricing-service/internal/database"
)

var (
	migrateOnce sync.Once
	migrateErr  error
)

// migrationsDir is the migrations directory relative to this package.
func migrationsDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..", "migrations")
}

// testPool returns a pool against the test database, applying migrations once
// per run. It skips the test when no test database is configured.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("PRICING_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PRICING_TEST_DATABASE_URL is not set; skipping database-backed test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)

	migrateOnce.Do(func() {
		migrateErr = database.ApplyMigrations(ctx, pool, migrationsDir(t))
	})
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}
	return pool
}

// unique returns an identifier unique to this test run, so append-only tables
// can be asserted on without cleanup.
func unique(prefix string) string {
	return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
}
