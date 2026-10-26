// Package testdb is test-only support for the real-PostgreSQL suites. Every
// suite resets the shared test schema, so suites in different packages (which
// `go test ./...` runs as concurrent processes) serialize on one advisory lock.
package testdb

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/verification-compliance-service/internal/database"
)

const EnvDSN = "VERIFICATION_COMPLIANCE_TEST_DATABASE_URL"

// lockKey is arbitrary but fixed for this service's suites.
const lockKey = 7_181_800_418

// DSN returns the configured test DSN or skips. A database whose name does
// not say it is a test database is refused.
func DSN(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv(EnvDSN)
	if dsn == "" {
		t.Skip(EnvDSN + " is required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.Contains(strings.ToLower(parsed.Path), "test") {
		t.Fatal("refusing non-test database")
	}
	return dsn
}

// Lock holds the cross-process suite lock on a dedicated connection until the
// test ends. It never borrows from a pool, so closing a pool cannot deadlock.
func Lock(t testing.TB, dsn string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, int64(lockKey)); err != nil {
		_ = conn.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, int64(lockKey))
		_ = conn.Close(context.Background())
	})
}

// MigrationsDir is this service's migrations directory.
func MigrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}

// Fresh locks, resets the schema, applies every migration and returns a pool
// closed at test end (before the lock is released).
func Fresh(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := DSN(t)
	Lock(t, dsn)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err = pool.Exec(ctx, `DROP SCHEMA public CASCADE;CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = database.Apply(ctx, pool, MigrationsDir()); err != nil {
		t.Fatal(err)
	}
	return pool
}
