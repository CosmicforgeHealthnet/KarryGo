// Package integration_test holds the PostgreSQL-backed suite for the
// authoritative media object contract.
//
// The suite is gated on MEDIA_FILE_TEST_DATABASE_URL. It applies the real
// migrations into a disposable database and exercises upload authorization,
// file validation against real generated files, safety states, private
// download grants, deletion and the durable workers against real SQL and the
// real HTTP handlers.
package integration_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/media-file-service/internal/database"
)

const migrationsDir = "../../migrations"

var (
	migrateOnce sync.Once
	migrateErr  error
	// Tests share one disposable database and reset it with TRUNCATE, so one
	// test must own it until its cleanup completes.
	databaseTestMu sync.Mutex
)

// testPool connects to the disposable test database, applies migrations once
// per process and gives each test a clean slate.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseTestMu.Lock()
	t.Cleanup(databaseTestMu.Unlock)

	rawURL := os.Getenv("MEDIA_FILE_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("MEDIA_FILE_TEST_DATABASE_URL not set; skipping PostgreSQL integration tests (CI must always set it)")
	}
	requireTestDatabaseName(t, rawURL)

	pool, err := database.NewPool(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)

	// Serialize against a second `go test` process pointed at the same
	// database; a process-local mutex cannot protect that boundary.
	lockConn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire database test lock connection: %v", err)
	}
	if _, err = lockConn.Exec(context.Background(),
		`SELECT pg_advisory_lock(hashtext('media-file-integration-suite'))`); err != nil {
		lockConn.Release()
		t.Fatalf("acquire database test advisory lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = lockConn.Exec(context.Background(),
			`SELECT pg_advisory_unlock(hashtext('media-file-integration-suite'))`)
		lockConn.Release()
	})

	migrateOnce.Do(func() {
		migrateErr = database.ApplyMigrations(context.Background(), pool, migrationsDir)
	})
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}

	// The table list is derived from the live schema rather than hand-written,
	// so a table added by a later migration is cleaned the moment it exists.
	rows, err := pool.Query(context.Background(), `
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public'
		  AND (tablename LIKE 'media\_%' OR tablename = 'media_assets')
		  AND tablename <> 'media_file_schema_migrations'`)
	if err != nil {
		t.Fatalf("list test tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			rows.Close()
			t.Fatalf("scan test table name: %v", scanErr)
		}
		tables = append(tables, pgx.Identifier{name}.Sanitize())
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("list test tables: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("no media tables found; migrations did not apply")
	}
	// TRUNCATE bypasses the row-level append-only triggers by design — it is
	// test tooling, never a runtime path.
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE "+strings.Join(tables, ", ")+" CASCADE"); err != nil {
		t.Fatalf("clean test database: %v", err)
	}
	return pool
}

// requireTestDatabaseName refuses to run against anything that does not look
// like a dedicated disposable test database.
func requireTestDatabaseName(t *testing.T, rawURL string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("MEDIA_FILE_TEST_DATABASE_URL is not a valid URL")
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

// TestMigrationsReplayAndRollback proves the paired up/down files are real:
// a fresh apply, a rollback of the newest migration and a reapply all work
// against the same database.
func TestMigrationsReplayAndRollback(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Replay: applying again is a no-op because every version is recorded.
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("rollback last migration: %v", err)
	}
	// The current tip (000003) is additive capture metadata on media_objects,
	// so rolling back the latest migration must preserve the 000002 table and
	// remove only 000003's columns. Older versions of this test incorrectly
	// assumed 000002 would always remain the tip.
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT to_regclass('public.media_objects') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("check rolled-back table: %v", err)
	}
	if !exists {
		t.Fatal("latest rollback removed the media_objects baseline table")
	}
	var capturedAtExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema='public' AND table_name='media_objects' AND column_name='captured_at'
	)`).Scan(&capturedAtExists); err != nil {
		t.Fatalf("check rolled-back capture metadata: %v", err)
	}
	if capturedAtExists {
		t.Fatal("expected captured_at to be removed by rolling back 000003")
	}

	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT to_regclass('public.media_objects') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("check reapplied table: %v", err)
	}
	if !exists {
		t.Fatal("expected media_objects to exist after reapply")
	}
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema='public' AND table_name='media_objects' AND column_name='captured_at'
	)`).Scan(&capturedAtExists); err != nil {
		t.Fatalf("check reapplied capture metadata: %v", err)
	}
	if !capturedAtExists {
		t.Fatal("expected captured_at to exist after reapplying 000003")
	}
	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("migrations not current after reapply: %v", err)
	}
}

// TestMigrationNumbersAreUnique fails the build rather than letting two
// migrations claim the same slot, which would make the applied order depend on
// the filesystem.
func TestMigrationNumbersAreUnique(t *testing.T) {
	migrations, err := database.ListMigrations(migrationsDir)
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	if len(migrations) < 2 {
		t.Fatalf("expected at least two migrations, found %d", len(migrations))
	}
	seen := map[int]string{}
	for _, migration := range migrations {
		if other, duplicate := seen[migration.Number]; duplicate {
			t.Fatalf("duplicate migration number %d: %s and %s", migration.Number, other, migration.Version)
		}
		seen[migration.Number] = migration.Version
	}
	// Every migration added by this work package carries a down file.
	for _, migration := range migrations {
		if migration.Number >= 2 && migration.DownFile == "" {
			t.Fatalf("migration %s has no down file", migration.Version)
		}
	}
}
