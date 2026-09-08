package database

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	var count int
	if err = db.QueryRow(context.Background(), `SELECT count(*) FROM taxi_schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 11 {
		t.Fatalf("applied migrations=%d, want 11", count)
	}
}
