package database

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestListMigrationsRejectsDuplicateNumbers and friends need no database.
func TestListMigrationsFindsThePairedFiles(t *testing.T) {
	migrations, err := ListMigrations("../../migrations")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("no migrations found")
	}
	for _, migration := range migrations {
		if migration.UpFile == "" {
			t.Errorf("%s has no up file", migration.Version)
		}
		// Every pricing migration must be reversible. A migration that cannot be
		// rolled back cannot be rehearsed before a production cutover, and
		// rehearsing the rollback is a release gate for this service.
		if migration.DownFile == "" {
			t.Errorf("%s has no down file; pricing migrations must be reversible", migration.Version)
		}
	}
}

// TestMigrationReplay proves a migration can be applied, rolled back and applied
// again, and that the down file actually DROPS what the up file created rather
// than merely forgetting it was applied.
//
// This matters more for pricing than for most services. A cutover plan whose
// rollback has never been executed is a hope, not a plan, and the release gate
// for moving a market to central pricing requires a rehearsed rollback.
func TestMigrationReplay(t *testing.T) {
	url := os.Getenv("PRICING_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PRICING_TEST_DATABASE_URL is not set; skipping migration replay")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := NewPool(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	// Leave the database migrated however this test exits, so a failure part-way
	// through does not strand the next run with a half-rolled-back schema.
	t.Cleanup(func() {
		if err := ApplyMigrations(context.Background(), pool, "../../migrations"); err != nil {
			t.Errorf("restore schema after replay: %v", err)
		}
	})

	if err := ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	if err := MigrationsCurrent(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrations should be current after apply: %v", err)
	}

	tableExists := func(name string) bool {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = $1)`, name,
		).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", name, err)
		}
		return exists
	}
	functionExists := func(name string) bool {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_proc WHERE proname = $1)`, name,
		).Scan(&exists); err != nil {
			t.Fatalf("check function %s: %v", name, err)
		}
		return exists
	}

	if !tableExists("pricing_audit_log") {
		t.Fatal("pricing_audit_log should exist after apply")
	}

	// Roll back every applied migration, one at a time.
	migrations, err := ListMigrations("../../migrations")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for range migrations {
		if err := RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback: %v", err)
		}
	}

	// The down files must have removed the objects, not just the tracking rows.
	for _, name := range []string{
		"pricing_audit_log", "pricing_outbox", "pricing_operators",
		"pricing_idempotency_keys", "pricing_service_nonces",
	} {
		if tableExists(name) {
			t.Errorf("%s still exists after rollback; the down file must drop what the up file created", name)
		}
	}
	for _, name := range []string{"pricing_reject_mutation", "pricing_reject_mutation_purgeable"} {
		if functionExists(name) {
			t.Errorf("function %s still exists after rollback", name)
		}
	}

	// And it must all come back.
	if err := ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if err := MigrationsCurrent(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrations should be current after re-apply: %v", err)
	}
	if !tableExists("pricing_audit_log") {
		t.Fatal("pricing_audit_log should exist again after re-apply")
	}
	if !functionExists("pricing_reject_mutation") {
		t.Fatal("the immutability guard should exist again after re-apply")
	}
}

// TestMigrationsCurrentReportsAnUnappliedMigration guards the readiness check:
// a replica running older schema than its policy data expects must not take
// quote traffic.
func TestMigrationsCurrentReportsAnUnappliedMigration(t *testing.T) {
	url := os.Getenv("PRICING_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PRICING_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := NewPool(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	// Registered BEFORE the restore below. Cleanups run last-registered-first,
	// so the restore still has a live pool. A plain `defer pool.Close()` would
	// run before any cleanup and leave this database with a forgotten
	// migration, which makes the service report not-ready -- exactly the kind
	// of silent test pollution that costs an afternoon.
	t.Cleanup(pool.Close)

	if err := ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Forget the newest migration, then assert the check notices.
	migrations, err := ListMigrations("../../migrations")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	newest := migrations[len(migrations)-1].Version
	if _, err := pool.Exec(ctx,
		`DELETE FROM pricing_service_schema_migrations WHERE version = $1`, newest); err != nil {
		t.Fatalf("forget migration: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO pricing_service_schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`, newest); err != nil {
			// Never swallow this: leaving the tracking table short one row makes
			// every later run of the suite, and the service itself, look broken.
			t.Errorf("restore migration record %s: %v", newest, err)
		}
	})

	if err := MigrationsCurrent(ctx, pool, "../../migrations"); err == nil {
		t.Fatal("MigrationsCurrent should report an unapplied migration")
	}
}
