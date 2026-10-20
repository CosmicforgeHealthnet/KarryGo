package testutil

import (
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
)

// Database tests use fresh schemas inside an explicitly named disposable DB.
func Database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	value := os.Getenv("ADMIN_BRIDGE_TEST_DATABASE_URL")
	if value == "" {
		t.Skip("ADMIN_BRIDGE_TEST_DATABASE_URL is not configured; PostgreSQL integration test NOT RUN")
	}
	cfg, err := pgxpool.ParseConfig(value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("refusing a database without 'test' in its name")
	}
	ctx := context.Background()
	root, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := "admin_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = root.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		root.Close()
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); _, _ = root.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); root.Close() })
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}
