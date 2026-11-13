package section30contract

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"testing"
)

// The two latest migrations:
//   - 000088 publishes company activation policy version 2 and archives
//     version 1; rolling back restores version 1 and re-applying restores 2.
//   - 000087 backfills an active Head Office branch for every company without
//     one; rolling it back keeps the branches (other records may reference
//     them) and re-applying it never duplicates them.
func TestMigrationReplay_LatestMigrationRollbackAndReapply(t *testing.T) {
	if replayMigrationTip(t) != "000088_company_activation_policy_v2" {
		t.Fatal("review latest migration retention assertions")
	}
	pool := isolatedTestDatabase(t, "fleet_partner_onboarding_tip")
	ctx := context.Background()
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	activePolicy := func() string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `SELECT id FROM fleet_partner_eligibility_policies WHERE organization_id IS NULL AND policy_key='COMPANY_ACTIVATION' AND status='ACTIVE'`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	if got := activePolicy(); got != "elp_default_company_activation_v2" {
		t.Fatalf("active policy after apply: %s", got)
	}
	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	if got := activePolicy(); got != "elp_default_company_activation" {
		t.Fatalf("active policy after rollback: %s", got)
	}
	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO fleet_partner_organizations(id,legal_name,registration_number,country_code,timezone,default_currency,status,organization_type) VALUES('org_no_branch','No Branch','RC-NB','NG','Africa/Lagos','NGN','UNDER_REVIEW','PRIVATE_LIMITED')`,
		`INSERT INTO fleet_partner_organizations(id,legal_name,registration_number,country_code,timezone,default_currency,status,organization_type) VALUES('org_has_branch','Has Branch','RC-HB','NG','Africa/Lagos','NGN','DRAFT','PRIVATE_LIMITED')`,
		`INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone) VALUES('brn_existing','org_has_branch','IKJ','Ikeja','Africa/Lagos')`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	branches := func(org string) (count int, headOffice int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE code='HQ' AND name='Head Office' AND status='ACTIVE' AND timezone='Africa/Lagos') FROM fleet_partner_branches WHERE organization_id=$1`, org).Scan(&count, &headOffice); err != nil {
			t.Fatal(err)
		}
		return
	}
	if count, hq := branches("org_no_branch"); count != 1 || hq != 1 {
		t.Fatalf("backfill: %d branches, %d head office", count, hq)
	}
	if count, hq := branches("org_has_branch"); count != 1 || hq != 0 {
		t.Fatalf("company with a branch was changed: %d branches, %d head office", count, hq)
	}
	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	if count, _ := branches("org_no_branch"); count != 1 {
		t.Fatalf("rollback removed the head office: %d", count)
	}
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	if count, _ := branches("org_no_branch"); count != 1 {
		t.Fatalf("reapply duplicated the head office: %d", count)
	}
	if got := activePolicy(); got != "elp_default_company_activation_v2" {
		t.Fatalf("active policy after reapply: %s", got)
	}
}
