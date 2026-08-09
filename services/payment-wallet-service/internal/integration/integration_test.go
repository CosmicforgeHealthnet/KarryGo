// Package integration_test holds the PostgreSQL-backed suite for the
// payment-wallet-service financial foundation.
//
// The suite is gated on PAYMENT_WALLET_TEST_DATABASE_URL. It applies the real
// migrations into a disposable database and exercises the ledger, wallets,
// earning accounts, subscriptions, commissions, holds, settlements, refunds,
// idempotency, RBAC, reconciliation and the durable workers against real SQL.
package integration_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
)

const migrationsDir = "../../migrations"

var (
	migrateOnce sync.Once
	migrateErr  error
)

// testPool connects to the disposable test database, applies migrations once
// per process and gives each test a clean slate.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	rawURL := os.Getenv("PAYMENT_WALLET_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("PAYMENT_WALLET_TEST_DATABASE_URL not set; skipping PostgreSQL integration tests (CI must always set it)")
	}
	requireTestDatabaseName(t, rawURL)

	pool, err := database.NewPool(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)

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
		  AND tablename LIKE 'finance\_%'
		  AND tablename <> 'payment_wallet_schema_migrations'`)
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
		t.Fatal("no finance tables found; migrations did not apply")
	}
	// TRUNCATE bypasses the row-level append-only triggers by design — it is
	// test tooling, never a runtime path.
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE "+strings.Join(tables, ", ")+" CASCADE"); err != nil {
		t.Fatalf("clean test database: %v", err)
	}

	// Truncating wipes the seeded permission/role/policy catalogue. Every
	// migration that seeds it is idempotent, so replaying them in order
	// restores exactly what a freshly migrated database would hold — including
	// catalogue entries added by later migrations.
	for _, seedMigration := range []string{
		"000004_finance_foundation.up.sql",
		"000005_reversal_permission.up.sql",
		"000006_reversal_contract_and_job_payments.up.sql",
		"000007_provider_subwallet_operations.up.sql",
	} {
		seed, err := os.ReadFile(filepath.Join(migrationsDir, seedMigration))
		if err != nil {
			t.Fatalf("read seed migration %s: %v", seedMigration, err)
		}
		if _, err := pool.Exec(context.Background(), string(seed)); err != nil {
			t.Fatalf("restore finance catalogue from %s: %v", seedMigration, err)
		}
	}
	return pool
}

// requireTestDatabaseName refuses to run against anything that does not look
// like a dedicated disposable test database.
func requireTestDatabaseName(t *testing.T, rawURL string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("PAYMENT_WALLET_TEST_DATABASE_URL is not a valid URL")
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

// TestMigrationsReplayAndRollback proves fresh apply, replay, rollback and
// reapply all work against real PostgreSQL.
func TestMigrationsReplayAndRollback(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("migrations not current after apply: %v", err)
	}
	// Replay is a no-op.
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	// Roll the last migration back and re-apply it.
	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err == nil {
		t.Fatal("migrations reported current after rollback")
	}
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("re-apply after rollback: %v", err)
	}
	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("migrations not current after re-apply: %v", err)
	}
}

// TestMigrationNumbersAreUnique proves the discovery layer refuses duplicate
// migration slots, which is what keeps applied order deterministic across
// environments.
func TestMigrationNumbersAreUnique(t *testing.T) {
	migrations, err := database.ListMigrations(migrationsDir)
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("no migrations discovered")
	}
	seen := map[int]string{}
	for _, migration := range migrations {
		if other, duplicate := seen[migration.Number]; duplicate {
			t.Fatalf("duplicate migration number %d: %s and %s", migration.Number, other, migration.Version)
		}
		seen[migration.Number] = migration.Version
	}
	// Numeric ordering, not string ordering: 001 must sort before 000004.
	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].Number >= migrations[i].Number {
			t.Fatalf("migrations out of numeric order: %s then %s",
				migrations[i-1].Version, migrations[i].Version)
		}
	}
}
