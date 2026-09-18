package truckcapability

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const migrationName = "021_truck_capability_matching"

func readMigration(t *testing.T, suffix string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", migrationName+suffix))
	if err != nil {
		t.Fatalf("read %s%s: %v", migrationName, suffix, err)
	}
	return string(raw)
}

func TestMigrationCreatesEveryCapabilityObject(t *testing.T) {
	up := readMigration(t, ".up.sql")
	for _, required := range []string{
		"hauling_truck_product_eligibility",
		"hauling_policy_snapshots",
		"hauling_match_decisions",
		"max_payload_grams",
		"internal_length_mm", "internal_width_mm", "internal_height_mm", "usable_volume_ml",
		"opening_width_mm", "opening_height_mm",
		"enclosed", "weather_protected", "upright_safe", "fragile_safe",
		"refrigerated", "temperature_min_c", "temperature_max_c",
		"tail_lift", "crane", "hydraulic", "loading_ramp",
		"supported_container_sizes", "supported_goods_classes", "prohibited_goods_classes",
		"capability_version", "capability_source_version",
		"uq_hauling_truck_product", "uq_hauling_policy_snapshot",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("up migration missing %q", required)
		}
	}
}

// Every table the up creates must be dropped by the down, and every column it
// adds must be dropped too. A down that half-reverses leaves a database no
// later migration can be applied to cleanly.
func TestDownReversesEverythingUpCreates(t *testing.T) {
	up := readMigration(t, ".up.sql")
	down := readMigration(t, ".down.sql")

	tablePattern := regexp.MustCompile(`(?i)CREATE TABLE IF NOT EXISTS\s+(\w+)`)
	for _, match := range tablePattern.FindAllStringSubmatch(up, -1) {
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+match[1]) {
			t.Errorf("down migration does not drop %q", match[1])
		}
	}

	columnPattern := regexp.MustCompile(`(?i)ADD COLUMN IF NOT EXISTS\s+(\w+)`)
	for _, match := range columnPattern.FindAllStringSubmatch(up, -1) {
		if !strings.Contains(down, "DROP COLUMN IF EXISTS "+match[1]) {
			t.Errorf("down migration does not drop column %q", match[1])
		}
	}
}

// The legacy columns the old matcher depends on must survive both directions,
// so a rollback leaves weight-and-type matching exactly as it found it.
func TestLegacyColumnsSurviveRollback(t *testing.T) {
	down := readMigration(t, ".down.sql")
	for _, legacy := range []string{"capacity_kg", "truck_type", "has_insurance", "goods_types"} {
		if strings.Contains(down, "DROP COLUMN IF EXISTS "+legacy) {
			t.Errorf("down migration drops legacy column %q, which the old matcher still reads", legacy)
		}
	}
}

// The only safe backfill is capacity_kg, which is a real measurement someone
// entered. Anything else -- a body type implying enclosure, a truck type
// implying refrigeration -- must be left NULL and answered at read time from
// the catalogue, so a genuinely surveyed truck stays distinguishable from one
// that merely has a suggestive name.
func TestMigrationBackfillsOnlyRealMeasurements(t *testing.T) {
	up := readMigration(t, ".up.sql")
	updatePattern := regexp.MustCompile(`(?is)UPDATE\s+trucks\s+SET\s+(\w+)`)
	backfilled := map[string]bool{}
	for _, match := range updatePattern.FindAllStringSubmatch(up, -1) {
		backfilled[strings.ToLower(match[1])] = true
	}

	allowed := map[string]bool{
		// A real number a provider entered, converted to grams.
		"max_payload_grams": true,
		// Only a TRUE has_insurance is promoted; a FALSE is indistinguishable
		// from an unanswered form and stays NULL.
		"insurance_valid": true,
	}
	for column := range backfilled {
		if !allowed[column] {
			t.Errorf("migration backfills %q, which is inferred rather than measured", column)
		}
	}
	if !backfilled["max_payload_grams"] {
		t.Error("capacity_kg was not carried forward; every truck would lose its known payload")
	}
}

// Hauling owns its own schema.
func TestMigrationOwnsOnlyHaulingSchema(t *testing.T) {
	up := strings.ToLower(readMigration(t, ".up.sql"))
	for _, foreign := range []string{"vendor_orders", "dispatch_bookings", "bikes", "wallet_accounts", "taxi_"} {
		if strings.Contains(up, foreign) {
			t.Errorf("Hauling migration touches foreign schema %q", foreign)
		}
	}
}

func TestMigrationNumberIsUnique(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "..", "migrations"))
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	seen := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "021_") && strings.HasSuffix(entry.Name(), ".up.sql") {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("found %d migrations numbered 021, want exactly 1", seen)
	}
}
