package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", name, err)
	}
	return string(content)
}

func TestEveryUpMigrationHasDown(t *testing.T) {
	ups, err := filepath.Glob("*.up.sql")
	if err != nil || len(ups) == 0 {
		t.Fatalf("no up migrations found: %v", err)
	}
	for _, up := range ups {
		down := strings.TrimSuffix(up, ".up.sql") + ".down.sql"
		if _, err := os.Stat(down); err != nil {
			t.Errorf("missing down migration for %s", up)
		}
	}
}

func TestPlatformFoundationTables(t *testing.T) {
	sql := read(t, "000001_platform_foundation.up.sql")
	for _, table := range []string{
		"fleet_partner_outbox", "fleet_partner_inbox", "fleet_partner_jobs",
		"fleet_partner_idempotency_keys", "fleet_partner_audit_records",
		"fleet_partner_file_references",
	} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"PRIMARY KEY (consumer_name, event_id)",                    // inbox dedupe constraint
		"idx_fleet_partner_jobs_active_dedupe",                     // job dedupe partial unique index
		"PRIMARY KEY (organization_id, endpoint, idempotency_key)", // org-scoped idempotency
		"trg_fleet_partner_audit_append_only",                      // append-only audit
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("missing constraint fragment %q", fragment)
		}
	}
}

func TestOrganizationsFoundationTables(t *testing.T) {
	sql := read(t, "000002_organizations_foundation.up.sql")
	for _, table := range []string{
		"fleet_partner_organizations", "fleet_partner_branches",
		"fleet_partner_departments", "fleet_partner_organization_settings",
		"fleet_partner_organization_status_history",
	} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"uq_fleet_partner_branch_code UNIQUE (organization_id, code)",
		"uq_fleet_partner_department_code UNIQUE (organization_id, code)",
		"uq_fleet_partner_org_registration UNIQUE (country_code, registration_number)",
		"chk_fleet_partner_department_no_self_parent",
		"ON DELETE RESTRICT",
		"trg_fleet_partner_org_status_history_append_only",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("missing constraint fragment %q", fragment)
		}
	}
	if strings.Contains(sql, "REFERENCES customers") || strings.Contains(sql, "REFERENCES providers") {
		t.Error("migration must not reference another service's tables")
	}
}

func TestNoCrossServiceReferences(t *testing.T) {
	files, _ := filepath.Glob("*.sql")
	for _, file := range files {
		sql := strings.ToLower(read(t, file))
		for _, forbidden := range []string{"dblink", "foreign data wrapper", "postgres_fdw"} {
			if strings.Contains(sql, forbidden) {
				t.Errorf("%s uses cross-database mechanism %q", file, forbidden)
			}
		}
	}
}
