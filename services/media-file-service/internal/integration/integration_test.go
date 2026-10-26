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
	// The disposable database is verified and exclusively locked above. Keep
	// statement-level evidence guards enabled during every test; disable only
	// the ledger and legacy-adoption guards inside this atomic cleanup transaction.
	clean, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer clean.Rollback(context.Background())
	for _, pair := range [][2]string{{"media_admin_case_grants", "media_admin_case_grant_immutable"}, {"media_admin_case_revocations", "media_admin_case_revocation_immutable"}, {"media_legacy_adoptions", "media_legacy_adoption_immutable"}, {"media_admin_case_redemptions", "media_admin_case_redemption_immutable"}, {"media_legacy_asset_events", "media_legacy_asset_event_immutable"}} {
		if _, err := clean.Exec(context.Background(), "ALTER TABLE "+pgx.Identifier{pair[0]}.Sanitize()+" DISABLE TRIGGER "+pgx.Identifier{pair[1]}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := clean.Exec(context.Background(), "TRUNCATE "+strings.Join(tables, ", ")+" CASCADE"); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"media_admin_case_grants", "media_admin_case_grant_immutable"}, {"media_admin_case_revocations", "media_admin_case_revocation_immutable"}, {"media_legacy_adoptions", "media_legacy_adoption_immutable"}, {"media_admin_case_redemptions", "media_admin_case_redemption_immutable"}, {"media_legacy_asset_events", "media_legacy_asset_event_immutable"}} {
		if _, err := clean.Exec(context.Background(), "ALTER TABLE "+pgx.Identifier{pair[0]}.Sanitize()+" ENABLE TRIGGER "+pgx.Identifier{pair[1]}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	if err := clean.Commit(context.Background()); err != nil {
		t.Fatal(err)
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
	migrations, err := database.ListMigrations(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) == 0 || migrations[len(migrations)-1].Version != "000009_secure_admin_section19" {
		t.Fatal("Review and update the tip-specific rollback assertions for the new migration")
	}

	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("rollback last migration: %v", err)
	}
	// Reapply however this test ends. A failed assertion below used to skip
	// the reapply at the bottom and leave the shared test database rolled
	// back, which made every later test in the package fail with an
	// unrelated-looking internal error.
	t.Cleanup(func() {
		if err := database.ApplyMigrations(context.Background(), pool, migrationsDir); err != nil {
			t.Errorf("cleanup reapply migrations: %v", err)
		}
	})
	// Rolling back the tip must remove exactly what the tip added and leave
	// everything before it standing. The assertion is written against whatever
	// the current tip is rather than against a named version: an earlier form
	// of this test hardcoded 000003 and had to be edited the moment a later
	// migration landed, which is how a guard quietly stops guarding.
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT to_regclass('public.media_objects') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("check rolled-back table: %v", err)
	}
	if !exists {
		t.Fatal("latest rollback removed the media_objects baseline table")
	}
	// The current tip (Section 19) removes only the empty redemption ledger,
	// the legacy asset event trail and the legacy hold/tombstone columns. The
	// legacy adoption mapping, case grant ledger, native privacy, binding and
	// export constraints beneath it must remain intact.
	var bindingExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema='public' AND table_name='media_objects' AND column_name='binding_type'
	)`).Scan(&bindingExists); err != nil {
		t.Fatalf("check rolled-back binding metadata: %v", err)
	}
	if !bindingExists {
		t.Fatal("rolling back vendor privacy removed earlier evidence binding columns")
	}
	var vendorConstraints int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname IN ('vendor_asset_private_ck','vendor_asset_authorization_private_ck')`).Scan(&vendorConstraints); err != nil {
		t.Fatal(err)
	}
	if vendorConstraints != 2 {
		t.Fatalf("tip rollback changed native vendor privacy constraints: %d", vendorConstraints)
	}
	var caseExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.media_admin_case_grants') IS NOT NULL AND to_regclass('public.media_admin_case_revocations') IS NOT NULL AND to_regclass('public.media_admin_case_policy_heads') IS NOT NULL AND to_regclass('public.media_legacy_adoptions') IS NOT NULL AND to_regclass('public.media_admin_case_redemptions') IS NULL AND to_regclass('public.media_legacy_asset_events') IS NULL`).Scan(&caseExists); err != nil || !caseExists {
		t.Fatal("tip rollback removed the case ledger/adoption mapping or kept Section 19 tables", err)
	}
	var holdColumns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='media_assets' AND column_name IN ('legal_hold','legal_hold_reason','retention_until','tombstoned_at')`).Scan(&holdColumns); err != nil || holdColumns != 0 {
		t.Fatal("tip rollback kept legacy hold/tombstone columns", holdColumns, err)
	}
	// 000004's data-export visibility is NOT the tip's, so it must survive.
	var visibilityExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema='public' AND table_name='media_objects' AND column_name='visibility'
	)`).Scan(&visibilityExists); err != nil {
		t.Fatalf("check preserved export metadata: %v", err)
	}
	if !visibilityExists {
		t.Fatal("rolling back the tip removed 000004's visibility column")
	}
	// And everything the tip did not own survives untouched.
	var capturedAtExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema='public' AND table_name='media_objects' AND column_name='captured_at'
	)`).Scan(&capturedAtExists); err != nil {
		t.Fatalf("check preserved capture metadata: %v", err)
	}
	if !capturedAtExists {
		t.Fatal("rolling back the tip removed an earlier migration's columns")
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
		WHERE table_schema='public' AND table_name='media_objects' AND column_name='binding_type'
	)`).Scan(&bindingExists); err != nil {
		t.Fatalf("check reapplied binding metadata: %v", err)
	}
	if !bindingExists {
		t.Fatal("expected the evidence binding columns to exist after reapply")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname IN ('vendor_asset_private_ck','vendor_asset_authorization_private_ck')`).Scan(&vendorConstraints); err != nil {
		t.Fatal(err)
	}
	if vendorConstraints != 2 {
		t.Fatalf("reapply restored %d vendor privacy constraints, want 2", vendorConstraints)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.media_admin_case_grants') IS NOT NULL AND to_regclass('public.media_admin_case_revocations') IS NOT NULL AND to_regclass('public.media_admin_case_policy_heads') IS NOT NULL AND to_regclass('public.media_legacy_adoptions') IS NOT NULL AND to_regclass('public.media_admin_case_redemptions') IS NOT NULL AND to_regclass('public.media_legacy_asset_events') IS NOT NULL`).Scan(&caseExists); err != nil || !caseExists {
		t.Fatal("case ledger / adoption / Section 19 reapply failed", err)
	}
	// The private-only rule is part of the schema, not just the use case, so it
	// must come back with the reapply.
	var constraintExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM pg_constraint WHERE conname='chk_media_object_data_export_private'
	)`).Scan(&constraintExists); err != nil {
		t.Fatalf("check reapplied private-only constraint: %v", err)
	}
	if !constraintExists {
		t.Fatal("the DATA_EXPORT private-only constraint did not survive a reapply")
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
