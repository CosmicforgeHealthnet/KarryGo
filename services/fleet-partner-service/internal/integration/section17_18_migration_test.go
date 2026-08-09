package integration_test

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

func TestSection17And18MigrationRollbackReplay(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	// Roll back through the Section 17/18 range while tolerating later additive
	// sections. A fixed "last five" silently began testing Sections 19-21 once
	// those migrations were appended.
	for count := 0; ; count++ {
		var latest string
		if e := pool.QueryRow(ctx, `SELECT version FROM fleet_partner_schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&latest); e != nil {
			t.Fatal(e)
		}
		if latest < "000038" {
			break
		}
		if e := database.RollbackLastMigration(ctx, pool, migrationsDir); e != nil {
			t.Fatalf("rollback %d (%s): %v", count+1, latest, e)
		}
	}
	var exists bool
	if e := pool.QueryRow(ctx, `SELECT to_regclass('public.fleet_partner_workforce_profiles') IS NOT NULL OR to_regclass('public.fleet_partner_assets') IS NOT NULL`).Scan(&exists); e != nil {
		t.Fatal(e)
	}
	if exists {
		t.Fatal("Section 17/18 tables remain after rollback")
	}
	if e := database.ApplyMigrations(ctx, pool, migrationsDir); e != nil {
		t.Fatalf("replay: %v", e)
	}
	if e := database.MigrationsCurrent(ctx, pool, migrationsDir); e != nil {
		t.Fatal(e)
	}
	if e := pool.QueryRow(ctx, `SELECT to_regclass('public.fleet_partner_workforce_profiles') IS NOT NULL AND to_regclass('public.fleet_partner_assets') IS NOT NULL AND to_regclass('public.fleet_partner_fuel_records') IS NOT NULL`).Scan(&exists); e != nil {
		t.Fatal(e)
	}
	if !exists {
		t.Fatal("Section 17/18 tables missing after replay")
	}
}
