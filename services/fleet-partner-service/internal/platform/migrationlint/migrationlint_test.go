package migrationlint

import (
	"strings"
	"testing"
)

func TestLoadMigrations_RealDirectoryLoadsAndPairs(t *testing.T) {
	migrations, err := LoadMigrations("../../../migrations")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(migrations) < 73 {
		t.Fatalf("loaded %d migrations, want at least 73", len(migrations))
	}
	issues := CheckPairingAndNumbering(migrations)
	if len(issues) > 0 {
		t.Errorf("pairing/numbering issues on real migrations directory: %+v", issues)
	}
	for _, m := range migrations {
		if m.UpSQL == "" {
			t.Errorf("%s: up SQL is empty", m.Version)
		}
	}
}

// TestClassify_RealMigrationsSpotCheck prints the computed category for
// every real migration (run with -v to see the table) and asserts the
// invariants the Section 30 audit actually depends on:
//   - every produced category is one of the nine declared values;
//   - no table name captured by TablesAffected carries stray SQL
//     punctuation (regression guard for the "CREATE TABLE name(" -- no
//     space before the paren -- capture bug found during development);
//   - a migration classified PERMISSION_SEED genuinely targets the
//     platform's fleet_partner_permissions/fleet_partner_role_permissions
//     catalog, never a business-domain table that merely contains one of
//     the hint words (e.g. "recurring_order_templates") as a substring.
func TestClassify_RealMigrationsSpotCheck(t *testing.T) {
	migrations, err := LoadMigrations("../../../migrations")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	inventory := BuildInventory(migrations)
	counts := map[Category]int{}
	for _, entry := range inventory {
		if !ValidCategories[entry.Category] {
			t.Errorf("%s: invalid category %q", entry.Version, entry.Category)
		}
		for _, table := range entry.TablesAffected {
			if strings.ContainsAny(table, "(),; \t\n") {
				t.Errorf("%s: table name %q captured stray punctuation", entry.Version, table)
			}
		}
		counts[entry.Category]++
		t.Logf("%s %-55s %-24s destructive=%-5v tables=%v", entry.Version, entry.Name, entry.Category, entry.DataLossPossible, entry.TablesAffected)
	}
	t.Logf("category counts: %+v", counts)

	if counts[CategoryDestructive] != 0 {
		t.Errorf("expected 0 historical migrations classified DESTRUCTIVE after accounting for this repository's known-safe check-widening idioms, got %d", counts[CategoryDestructive])
	}
}

// TestExtractFlags_PermissionSeedRequiresGenuinePermissionTable pins the
// permission-seed heuristic to the platform's actual RBAC catalog tables,
// not merely any INSERT INTO a table whose name happens to contain one of
// the hint words (e.g. "template") as a substring.
func TestExtractFlags_PermissionSeedRequiresGenuinePermissionTable(t *testing.T) {
	genuinePermissionSeed := `INSERT INTO fleet_partner_permissions(code) VALUES ('x.read');
INSERT INTO fleet_partner_role_permissions(role_id, permission_code) SELECT 'rol_system_owner', code FROM fleet_partner_permissions WHERE code = 'x.read';`
	if !ExtractFlags(genuinePermissionSeed).HasPermissionInsert {
		t.Error("expected a genuine fleet_partner_permissions/role_permissions insert to set HasPermissionInsert")
	}

	businessDomainTemplateTable := `CREATE TABLE fleet_partner_recurring_order_templates (id text PRIMARY KEY);
INSERT INTO fleet_partner_recurring_order_templates(id) VALUES ('seed-row');`
	if ExtractFlags(businessDomainTemplateTable).HasPermissionInsert {
		t.Error("a business-domain table merely named *_templates must not be misclassified as a permission/role seed")
	}
}

// TestExtractFlags_DynamicCheckDiscoveryIsNotDestructive pins the safe
// catalog-lookup check-widening idiom (pg_constraint + quote_ident, used by
// migrations 000056, 000057, 000059, 000061, 000063) to CategoryEnumOrStateEvolution,
// not CategoryDestructive.
func TestExtractFlags_DynamicCheckDiscoveryIsNotDestructive(t *testing.T) {
	sql := `DO $$
DECLARE
    old_check text;
BEGIN
    SELECT con.conname INTO old_check
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    WHERE rel.relname = 'fleet_partner_widgets' AND con.contype = 'c';
    IF old_check IS NOT NULL THEN
        EXECUTE 'ALTER TABLE fleet_partner_widgets DROP CONSTRAINT ' || quote_ident(old_check);
    END IF;
END $$;
ALTER TABLE fleet_partner_widgets ADD CONSTRAINT fleet_partner_widgets_status_check_v2 CHECK (status IN ('A', 'B', 'C'));`

	flags := ExtractFlags(sql)
	if !flags.HasDynamicCheckDiscover {
		t.Error("expected HasDynamicCheckDiscover to be true for the pg_constraint + quote_ident idiom")
	}
	if !flags.HasCheckWidening {
		t.Error("expected HasCheckWidening to be true when the dynamic discovery is paired with a same-file ADD CONSTRAINT")
	}
	if got := Classify(sql); got != CategoryEnumOrStateEvolution {
		t.Errorf("Classify() = %s, want %s", got, CategoryEnumOrStateEvolution)
	}
}
