package database

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// historicalMigrationPool replays a pinned schema in an empty disposable
// database; it never unwinds a different section's populated schema.
func historicalMigrationPool(t *testing.T, ceiling string) (*pgxpool.Pool, string) {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(cfg.ConnConfig.Database), "test") && !strings.HasPrefix(cfg.ConnConfig.Database, "vendor_iso_") {
		t.Fatal("test database required")
	}
	admin, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	name := "vendor_test_history_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ident := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(context.Background(), "CREATE DATABASE "+ident); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer admin.Close()
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+ident+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	cfg = cfg.Copy()
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	dir := t.TempDir()
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || len(e.Name()) < 6 || e.Name()[:6] > ceiling {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("../../migrations", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, e.Name()), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = ApplyMigrations(context.Background(), pool, dir); err != nil {
		t.Fatal(err)
	}
	return pool, dir
}

func knownMigrationCount(t *testing.T) int {
	t.Helper()
	migrations, err := ListMigrations("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	return len(migrations)
}
