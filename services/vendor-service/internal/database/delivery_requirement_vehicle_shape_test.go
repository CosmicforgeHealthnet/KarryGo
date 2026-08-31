package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const deliveryRequirementMigration = "000101_delivery_requirement_vehicle_selection"

func readDeliveryRequirementMigration(t *testing.T, suffix string) string {
	t.Helper()

	path := filepath.Join(
		"..", "..", "migrations", deliveryRequirementMigration+suffix,
	)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return strings.ToLower(string(raw))
}

func TestDeliveryRequirementMigrationShape(t *testing.T) {
	sql := readDeliveryRequirementMigration(t, ".up.sql")

	if !strings.Contains(
		sql, "create table commerce_delivery_requirement_snapshots",
	) {
		t.Fatal("migration is missing the requirement snapshot table")
	}

	// The columns a support agent and an operations query actually read. A
	// snapshot that stored only the chosen class could answer "what ran" but
	// never "why not something smaller", which is the question people ask.
	for _, column := range []string{
		"minimum_physical_class",
		"selected_vehicle_class",
		"selected_service",
		"primary_reason",
		"supporting_reasons",
		"candidates",
		"measurement_state",
		"measurement_source",
		"effective_volume_ml",
		"policy_version",
		"evidence",
		"fingerprint",
	} {
		if !strings.Contains(sql, column) {
			t.Errorf("migration is missing column %q", column)
		}
	}
}

// The invariants that matter under concurrency are database constraints, not
// handler checks. A handler check is not enforced against a second writer.
func TestDeliveryRequirementInvariantsAreConstraints(t *testing.T) {
	sql := readDeliveryRequirementMigration(t, ".up.sql")

	cases := []struct {
		fragment string
		why      string
	}{
		{"unique (plan_id, slot_ref)",
			"one decision per mission slot per plan version, never an overwrite"},
		{"selected_vehicle_class <> 'car'",
			"taxi has no goods mode, so no row may claim a car executed a delivery"},
		{"commerce_delivery_requirement_snapshots_executable_chk",
			"an executable decision names both class and service, a refused one names neither"},
		{"commerce_delivery_requirement_snapshots_value_currency_chk",
			"a declared value without a currency cannot be compared to anything"},
	}

	for _, testCase := range cases {
		if !strings.Contains(sql, testCase.fragment) {
			t.Errorf("missing constraint %q: %s",
				testCase.fragment, testCase.why)
		}
	}

	// The reason vocabulary is constrained in the database, not only in Go. A
	// reason code nobody can count is not evidence.
	for _, reason := range []string{
		"'vehicle_selected'",
		"'no_suitable_vehicle'",
		"'no_suitable_executable_service'",
		"'temperature_conflict'",
	} {
		if !strings.Contains(sql, reason) {
			t.Errorf("primary_reason check is missing %s", reason)
		}
	}
}

// A down migration that only forgets about a table leaves the next apply to
// fail on an object that already exists.
func TestDeliveryRequirementDownDropsTheTable(t *testing.T) {
	sql := readDeliveryRequirementMigration(t, ".down.sql")

	if !strings.Contains(
		sql, "drop table if exists commerce_delivery_requirement_snapshots",
	) {
		t.Fatal("down migration does not drop the table it created")
	}
}
