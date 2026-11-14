package section30contract

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"testing"
)

// The latest migrations, newest first. Each down file is exercised in
// reverse order and must remove exactly what its up file added; re-applying
// must restore it:
//   - 000099 owner grants for the operating-mode permissions
//   - 000098 TAXI as a restriction propagation authority
//   - 000097 payment source reference on completed work and settlement
//     instructions
//   - 000096 managed settlement delivery columns, per-record temperature replay
//   - 000095 provider association read-back columns on provisioning sagas
//   - 000094 provider invitation resend counters
//   - 000093 custody subjects, incident investigations, compliance checks
//   - 000092 rescue cases and the asset lifecycle tables
//   - 000091 sales leads
//   - 000090 approval policy versions, delegations, cost centres, teams,
//     operating locations
//   - 000089 shift templates and training enrolments
//   - 000088 publishes company activation policy version 2 and archives
//     version 1; rolling back restores version 1 and re-applying restores 2.
//   - 000087 backfills an active Head Office branch for every company without
//     one; rolling it back keeps the branches (other records may reference
//     them) and re-applying it never duplicates them.
func TestMigrationReplay_LatestMigrationRollbackAndReapply(t *testing.T) {
	if replayMigrationTip(t) != "000099_operating_mode_owner_grants" {
		t.Fatal("review latest migration retention assertions")
	}
	pool := isolatedTestDatabase(t, "fleet_partner_onboarding_tip")
	ctx := context.Background()
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatal(err)
	}
	// A marker is a table, a column, or -- with check set -- a CHECK
	// constraint on the table whose definition contains check, or -- with
	// grant set -- the owner role holding that permission (table names the
	// role-permission table for the failure message).
	type marker struct{ migration, table, column, check, grant string }
	markers := []marker{
		{"000099", "fleet_partner_role_permissions", "", "", "fleet_partner_foundation.manage"},
		{"000098", "fleet_partner_restriction_operations", "", "TAXI", ""},
		{"000097", "fleet_partner_managed_work_settlement_instructions", "payment_source_reference", "", ""},
		{"000096", "fleet_partner_managed_work_settlement_instructions", "wallet_posting_operation_id", "", ""},
		{"000095", "fleet_partner_provider_provisioning_sagas", "association_id", "", ""},
		{"000094", "fleet_partner_provider_invitations", "resend_count", "", ""},
		{"000093", "fleet_partner_incident_investigations", "", "", ""},
		{"000092", "fleet_partner_rescue_cases", "", "", ""},
		{"000091", "fleet_partner_sales_leads", "", "", ""},
		{"000090", "fleet_partner_cost_centres", "", "", ""},
		{"000089", "fleet_partner_shift_templates", "", "", ""},
	}
	present := func(m marker) bool {
		t.Helper()
		var n int
		query := `SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1`
		args := []any{m.table}
		switch {
		case m.grant != "":
			query = `SELECT count(*) FROM fleet_partner_role_permissions WHERE role_id = 'rol_system_owner' AND permission_code = $1`
			args = []any{m.grant}
		case m.check != "":
			query = `SELECT count(*) FROM pg_constraint WHERE conrelid = to_regclass($1::text) AND contype = 'c' AND pg_get_constraintdef(oid) LIKE '%' || $2 || '%'`
			args = append(args, m.check)
		case m.column != "":
			query = `SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`
			args = append(args, m.column)
		}
		if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	for _, m := range markers {
		if !present(m) {
			t.Fatalf("%s marker %s.%s missing after apply", m.migration, m.table, m.column)
		}
	}
	for _, m := range markers {
		if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
			t.Fatalf("rollback %s: %v", m.migration, err)
		}
		if present(m) {
			t.Fatalf("%s marker %s.%s survived its rollback", m.migration, m.table, m.column)
		}
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
	for _, m := range markers {
		if !present(m) {
			t.Fatalf("%s marker %s.%s missing after re-apply", m.migration, m.table, m.column)
		}
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
