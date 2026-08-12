package integration_test

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

func TestSections19To21MigrationRollbackReplay(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	// Roll back through the Section 19-21 range while tolerating later
	// additive sections, matching the pattern in section17_18_migration_test.go:
	// a fixed step count silently under-rolls-back every time a later section
	// (most recently Section 25's search projection, 000053) adds a migration
	// on top of the stack, aborts this test before it replays, and leaves the
	// shared test database mid-rollback for whatever test runs next.
	for count := 0; ; count++ {
		var latest string
		if e := pool.QueryRow(ctx, `SELECT version FROM fleet_partner_schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&latest); e != nil {
			t.Fatal(e)
		}
		if latest < "000043" {
			break
		}
		if e := database.RollbackLastMigration(ctx, pool, migrationsDir); e != nil {
			t.Fatalf("rollback %d (%s): %v", count+1, latest, e)
		}
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT
		to_regclass('public.fleet_partner_restrictions') IS NOT NULL OR
		to_regclass('public.fleet_partner_incidents') IS NOT NULL OR
		to_regclass('public.fleet_partner_specialized_records') IS NOT NULL OR
		to_regclass('public.fleet_partner_warehouses') IS NOT NULL OR
		to_regclass('public.fleet_partner_fulfilment_orders') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("Sections 19-21 tables remain after rollback")
	}
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT
		to_regclass('public.fleet_partner_restrictions') IS NOT NULL AND
		to_regclass('public.fleet_partner_incidents') IS NOT NULL AND
		to_regclass('public.fleet_partner_specialized_records') IS NOT NULL AND
		to_regclass('public.fleet_partner_warehouses') IS NOT NULL AND
		to_regclass('public.fleet_partner_fulfilment_orders') IS NOT NULL AND
		to_regclass('public.fleet_partner_job_attempts') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("Sections 19-21 schema missing after replay")
	}
}
