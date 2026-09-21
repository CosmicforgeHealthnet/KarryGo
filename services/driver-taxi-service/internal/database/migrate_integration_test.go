package database

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyMigrationsCleanAndReplay(t *testing.T) {
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for migration certification")
	}
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test location")
	}
	serviceRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(serviceRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("first migration pass: %v", err)
	}
	if err = ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("idempotent migration replay: %v", err)
	}
	// The expected count is read from the directory rather than written here.
	// A literal is a number somebody has to remember to change, and the two
	// failure modes it is supposed to catch -- a migration file that never gets
	// recorded, and a replay that records one twice -- are both caught by
	// comparing against what is actually on disk. Hardcoding it only adds a
	// third failure mode: a correct migration that fails the build.
	expected, err := countUpMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if expected == 0 {
		t.Fatal("found no migrations to certify")
	}
	var count int
	if err = db.QueryRow(context.Background(), `SELECT count(*) FROM taxi_schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != expected {
		t.Fatalf("applied migrations=%d, want %d (one row per migrations/*.sql, rollbacks excluded)",
			count, expected)
	}

	// Every recorded name must still exist on disk. A tracker row for a file
	// that has been renamed or deleted means the next clean database will build
	// a different schema from this one, which is the bug a count alone misses.
	rows, err := db.Query(context.Background(), `SELECT name FROM taxi_schema_migrations`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if _, statErr := os.Stat(filepath.Join("migrations", name)); statErr != nil {
			t.Fatalf("migration %q is recorded as applied but is not in migrations/: %v", name, statErr)
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// countUpMigrations counts the files ApplyMigrations would apply, using the
// same rule it does: *.sql, excluding *.down.sql rollbacks.
func countUpMigrations() (int, error) {
	entries, err := os.ReadDir("migrations")
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") ||
			strings.HasSuffix(entry.Name(), ".down.sql") {
			continue
		}
		count++
	}
	return count, nil
}
