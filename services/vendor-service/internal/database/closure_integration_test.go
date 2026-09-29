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
	if err = ApplyMigrations(context.Background(), pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
}
