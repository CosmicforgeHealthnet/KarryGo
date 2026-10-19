// Package testfixture applies the owning service's actual migrations in an
// isolated disposable PostgreSQL schema. It does not substitute a domain schema.
package testfixture

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Database(t *testing.T, migrations string, upOnly ...bool) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ADMIN_OWNER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ADMIN_OWNER_TEST_DATABASE_URL required for actual owner migration tests")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.Contains(config.ConnConfig.Database, "test") {
		t.Fatal("disposable test database required")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Extensions are database-wide. Installing pgcrypto inside one disposable
	// schema makes sibling owners' real event triggers unable to resolve digest,
	// and that schema's cleanup can remove the extension from active tests.
	bootstrap, err := root.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bootstrap.Exec(ctx, "SELECT pg_advisory_xact_lock(807412569)"); err == nil {
		_, err = bootstrap.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public")
	}
	if err != nil {
		bootstrap.Rollback(ctx)
		t.Fatal(err)
	}
	if err = bootstrap.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	schema := "owner_source_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("disposable owner schema cleanup failed: %v", err)
		}
		root.Close()
	})
	files, err := filepath.Glob(filepath.Join(migrations, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatal("actual owner migrations required", err)
	}
	sort.Strings(files)
	for _, file := range files {
		if len(upOnly) > 0 && upOnly[0] && !strings.HasSuffix(file, ".up.sql") {
			continue
		}
		if strings.HasSuffix(file, ".down.sql") {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("owning migration %s: %v", filepath.Base(file), err)
		}
	}
	return pool
}
