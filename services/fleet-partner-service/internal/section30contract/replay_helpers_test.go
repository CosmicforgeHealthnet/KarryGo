// Package section30contract holds Section 30's migration-safety and
// backward-compatibility contract tests. This file provides the shared
// PostgreSQL test-database plumbing the migration replay tests
// (migration_replay_test.go) use.
//
// Each replay test runs against its own disposable, uniquely-named
// PHYSICAL DATABASE (not merely a named schema) on the same Postgres
// server as FLEET_PARTNER_TEST_DATABASE_URL. This was deliberate, not
// incidental: an earlier version of this helper isolated tests with a
// named schema instead, and TestMigrationReplay_EmptySchemaAppliesCleanly
// then failed with:
//
//	apply migration 000056_generic_imports: ERROR: constraint
//	"fleet_partner_bulk_imports_status_check_v2" of relation
//	"fleet_partner_bulk_imports" does not exist (SQLSTATE 42704)
//
// Root cause: migrations 000056/000057/000059/000061/000063 discover an
// unnamed legacy CHECK constraint via a pg_constraint/pg_class lookup
// filtered only by `rel.relname = '<table>'`, with no namespace/schema
// qualification. On a single-schema-per-database deployment (every real
// deployment of this service: Docker Compose and every documented local
// setup give each service its own dedicated database with only the
// default "public" schema) that lookup is always unambiguous. But this
// disposable test database is shared with internal/integration's own
// suite, which migrates the SAME table names into "public" -- so a
// schema-only isolation strategy let the lookup match "public"'s
// already-migrated row (which already carries the "_v2" name from a prior
// test run) while the subsequent unqualified `ALTER TABLE ... DROP
// CONSTRAINT` resolved, via search_path, to THIS test's own schema, whose
// table still had the original pre-widen constraint name. Two different
// schemas' catalog rows, sharing a table name, produced a name mismatch.
//
// This is a real fragility in those five migrations' dynamic-discovery
// pattern (a schema-qualified lookup, e.g. `con.conrelid =
// '<table>'::regclass`, would be immune to it), but it is a test-only
// artifact for this repository today -- no real deployment runs two
// schemas of these tables in one database -- and Section 30's mandate is
// explicit that historical, already-applied migrations are not rewritten
// to satisfy new tooling. So the fix belongs in the test's isolation
// strategy, not in migrations 000056/57/59/61/63: a genuinely separate
// PHYSICAL DATABASE has its own independent pg_catalog, so it cannot
// observe another database's rows at all. See
// docs/section30-database-migrations-backward-compatibility.md's
// "Known fragility" note for the full writeup.
package section30contract

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pgversion"
)

const migrationsDir = "../../migrations"

// requireTestDatabaseName refuses to run against anything that does not
// look like a dedicated disposable test database -- the same guard
// internal/integration's suite applies, duplicated locally because that
// package's copy is unexported and lives in a `_test` package that cannot
// be imported.
func requireTestDatabaseName(t *testing.T, rawURL string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("FLEET_PARTNER_TEST_DATABASE_URL is not a valid URL")
	}
	name := strings.ToLower(strings.TrimPrefix(parsed.Path, "/"))
	if !strings.Contains(name, "test") {
		t.Fatalf("refusing to run: database name %q does not look test-specific (must contain \"test\")", name)
	}
	for _, forbidden := range []string{"prod", "production", "staging", "live"} {
		if strings.Contains(name, forbidden) {
			t.Fatalf("refusing to run: database name %q looks like a %s database", name, forbidden)
		}
	}
}

// urlWithDatabase returns a copy of rawURL with its database (path)
// component replaced by dbName.
func urlWithDatabase(t *testing.T, rawURL, dbName string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse FLEET_PARTNER_TEST_DATABASE_URL: %v", err)
	}
	parsed.Path = "/" + dbName
	return parsed.String()
}

// isolatedTestDatabase connects to FLEET_PARTNER_TEST_DATABASE_URL's server
// (skipping the test if it is unset), creates a fresh, uniquely-named,
// genuinely empty disposable database (dbName), and returns a pool
// connected to it. The database -- and a server-wide advisory lock scoped
// to its name, guarding against two concurrent test processes racing the
// same CREATE/DROP DATABASE -- are cleaned up via t.Cleanup.
func isolatedTestDatabase(t *testing.T, dbName string) *pgxpool.Pool {
	t.Helper()
	rawURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL not set; skipping Section 30 migration replay test")
	}
	requireTestDatabaseName(t, rawURL)

	adminURL := urlWithDatabase(t, rawURL, "postgres")
	adminCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminConn, err := pgx.Connect(adminCtx, adminURL)
	if err != nil {
		t.Fatalf("connect to admin (postgres) database: %v", err)
	}
	t.Cleanup(func() { _ = adminConn.Close(context.Background()) })

	if _, err := adminConn.Exec(context.Background(), `SELECT pg_advisory_lock(hashtext($1))`, "fleet-partner-section30-db-"+dbName); err != nil {
		t.Fatalf("acquire section30 replay database advisory lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminConn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, "fleet-partner-section30-db-"+dbName)
	})

	ident := pgx.Identifier{dbName}.Sanitize()
	if _, err := adminConn.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, ident)); err != nil {
		t.Fatalf("drop existing disposable database %s: %v", dbName, err)
	}
	if _, err := adminConn.Exec(context.Background(), fmt.Sprintf(`CREATE DATABASE %s`, ident)); err != nil {
		t.Fatalf("create disposable database %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		_, _ = adminConn.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, ident))
	})

	pool, err := pgxpool.New(context.Background(), urlWithDatabase(t, rawURL, dbName))
	if err != nil {
		t.Fatalf("connect to disposable database %s: %v", dbName, err)
	}
	t.Cleanup(pool.Close)

	// WP11: fail fast, with an actionable message, rather than mid-replay on
	// an inscrutable SQL error, if the target server is below this
	// service's documented minimum supported PostgreSQL major version.
	if err := pgversion.RequireSupported(context.Background(), pool); err != nil {
		t.Fatalf("disposable database %s: %v", dbName, err)
	}

	return pool
}

// tableExists reports whether name exists as a table in the pool's default
// ("public") schema.
func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)`,
		name,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("check table %s existence: %v", name, err)
	}
	return exists
}

// columnExists reports whether table.column exists.
func columnExists(t *testing.T, pool *pgxpool.Pool, table, column string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2)`,
		table, column,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("check column %s.%s existence: %v", table, column, err)
	}
	return exists
}

// indexExists reports whether an index named name exists.
func indexExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1)`,
		name,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("check index %s existence: %v", name, err)
	}
	return exists
}

// appliedMigrationVersions returns every version recorded in
// fleet_partner_schema_migrations, sorted ascending.
func appliedMigrationVersions(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT version FROM fleet_partner_schema_migrations ORDER BY version ASC`)
	if err != nil {
		t.Fatalf("query applied migrations: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan applied migration version: %v", err)
		}
		out = append(out, v)
	}
	return out
}
