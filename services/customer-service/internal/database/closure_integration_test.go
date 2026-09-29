package database

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
)

func TestClosureApplyOwningMigrations(t *testing.T) {
	dsn := os.Getenv("CLOSURE_MIGRATION_DATABASE_URL")
	if dsn == "" {
		t.Skip("closure disposable database not configured")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir("internal/database") })
	if err = ApplyMigrations(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
}
