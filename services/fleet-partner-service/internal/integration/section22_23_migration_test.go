package integration_test

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

func TestSections22And23MigrationRollbackReplay(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	// Roll back through the Section 22-23 range while tolerating later
	// additive sections, matching the pattern in section17_18_migration_test.go
	// and section19_21_migration_test.go: a fixed step count silently
	// under-rolls-back every time a later section (most recently Section 25's
	// search projection, 000053) adds a migration on top of the stack, aborts
	// this test before it replays, and leaves the shared test database
	// mid-rollback for whatever test runs next.
	for count := 0; ; count++ {
		var latest string
		if e := pool.QueryRow(ctx, `SELECT version FROM fleet_partner_schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&latest); e != nil {
			t.Fatal(e)
		}
		if latest < "000048" {
			break
		}
		if e := database.RollbackLastMigration(ctx, pool, migrationsDir); e != nil {
			t.Fatalf("rollback %d (%s): %v", count+1, latest, e)
		}
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.fleet_partner_invoices') IS NOT NULL OR to_regclass('public.fleet_partner_payroll_periods') IS NOT NULL OR to_regclass('public.fleet_partner_communication_threads') IS NOT NULL OR to_regclass('public.fleet_partner_sla_measurements') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("Sections 22-23 tables remain after rollback")
	}
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.fleet_partner_invoices') IS NOT NULL AND to_regclass('public.fleet_partner_payroll_periods') IS NOT NULL AND to_regclass('public.fleet_partner_communication_threads') IS NOT NULL AND to_regclass('public.fleet_partner_sla_measurements') IS NOT NULL AND to_regclass('public.fleet_partner_escalations') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("Sections 22-23 schema missing after replay")
	}
}
