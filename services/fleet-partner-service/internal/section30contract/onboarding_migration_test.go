package section30contract

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"testing"
)

func TestMigrationReplay_LatestMigrationRollbackAndReapply(t *testing.T) {
	if replayMigrationTip(t) != "000086_onboarding_persistence" {
		t.Fatal("review latest migration retention assertions")
	}
	pool := isolatedTestDatabase(t, "fleet_partner_onboarding_tip")
	ctx := context.Background()
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	if !columnExists(t, pool, "fleet_partner_organizations", "organization_type") {
		t.Fatal("company type missing")
	}
	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	if columnExists(t, pool, "fleet_partner_organizations", "organization_type") {
		t.Fatal("empty rollback failed")
	}
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_organizations(id,legal_name,registration_number,country_code,timezone,default_currency,status,organization_type) VALUES('org_type_retained','Retained','RC-TYPE','NG','Africa/Lagos','NGN','DRAFT','PRIVATE_LIMITED')`); err != nil {
		t.Fatal(err)
	}
	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err == nil {
		t.Fatal("rollback discarded legal data")
	}
}
