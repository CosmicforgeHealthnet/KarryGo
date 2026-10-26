// Package testdb gives Support's PostgreSQL tests a fresh, isolated database
// with the service's real migrations applied, so suites never race each other
// (or another package's rollback test) on shared schema.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrationsDir is the service's top-level migration directory.
func MigrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations"))
}

// Apply runs every top-level migration in name order, exactly like the
// service runner (internal/database.ApplyMigrations) does on boot.
func Apply(ctx context.Context, t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	entries, err := os.ReadDir(MigrationsDir())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(MigrationsDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
}

// Fresh creates a disposable database from SUPPORT_DISPUTE_TEST_DATABASE_URL,
// applies the migrations and drops it at cleanup. It skips only when the
// variable is unset.
func Fresh(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SUPPORT_DISPUTE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SUPPORT_DISPUTE_TEST_DATABASE_URL to run Section 16 PostgreSQL tests")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgxpool.NewWithConfig(ctx, config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "support_s16_" + hex.EncodeToString(suffix[:]) + "_test"
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatalf("create disposable database: %v", err)
	}
	config.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		admin.Close()
	})
	Apply(ctx, t, pool)
	return pool
}
