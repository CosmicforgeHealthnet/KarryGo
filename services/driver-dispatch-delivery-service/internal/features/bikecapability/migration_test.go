package bikecapability

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const migrationName = "000061_dispatch_capability_matching"

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
		"dispatch_bike_capabilities",
		"dispatch_vehicle_product_eligibility",
		"dispatch_policy_snapshots",
		"dispatch_match_decisions",
		"max_payload_grams", "box_length_mm", "box_width_mm", "box_height_mm",
		"enclosed_box", "weather_protected", "upright_safe", "fragile_safe",
		"thermal_bag", "temperature_class",
		"capability_version", "source_version",
		"uq_dispatch_vehicle_product",
		"uq_dispatch_policy_snapshot",
		"requirement_snapshot",
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
		table := match[1]
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("down migration does not drop %q", table)
		}
	}

	columnPattern := regexp.MustCompile(`(?i)ADD COLUMN IF NOT EXISTS\s+(\w+)`)
	for _, match := range columnPattern.FindAllStringSubmatch(up, -1) {
		column := match[1]
		if !strings.Contains(down, "DROP COLUMN IF EXISTS "+column) {
			t.Errorf("down migration does not drop column %q", column)
		}
	}
}

// The physical capability columns must all be nullable, because NULL is how
// "never surveyed" is represented and the fit engine's fail-closed behaviour
// depends on it. A NOT NULL here with a default would silently turn every
// unsurveyed bike into a bike that declares a value.
func TestCapabilityFactsAreNullable(t *testing.T) {
	up := readMigration(t, ".up.sql")
	start := strings.Index(up, "CREATE TABLE IF NOT EXISTS dispatch_bike_capabilities")
	if start < 0 {
		t.Fatal("capability table not found")
	}
	end := strings.Index(up[start:], ");")
	body := up[start : start+end]

	for _, column := range []string{
		"max_payload_grams", "box_length_mm", "box_width_mm", "box_height_mm",
		"enclosed_box", "weather_protected", "upright_safe", "fragile_safe", "thermal_bag",
	} {
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, column+" ") {
				continue
			}
			if strings.Contains(strings.ToUpper(trimmed), "NOT NULL") {
				t.Errorf("%s is NOT NULL; unsurveyed bikes could not be represented", column)
			}
		}
	}
}

// Nothing in this migration may fabricate a capability from bike_type. The
// whole fail-closed design rests on an unsurveyed bike staying unsurveyed.
func TestMigrationBackfillsNoCapabilityFromBikeType(t *testing.T) {
	up := strings.ToLower(readMigration(t, ".up.sql"))
	if strings.Contains(up, "update dispatch_bike_capabilities") {
		t.Error("migration backfills capability values; unsurveyed bikes must stay unsurveyed")
	}
	if strings.Contains(up, "insert into dispatch_bike_capabilities") {
		t.Error("migration inserts capability rows; a row invented from bike_type is a fabricated survey")
	}
}

// Dispatch owns its own schema. A migration here reaching into another
// service's tables would be a cross-service write in disguise.
func TestMigrationOwnsOnlyDispatchSchema(t *testing.T) {
	up := strings.ToLower(readMigration(t, ".up.sql"))
	for _, foreign := range []string{"vendor_orders", "haulage_bookings", "trucks", "wallet_accounts", "taxi_"} {
		if strings.Contains(up, foreign) {
			t.Errorf("Dispatch migration touches foreign schema %q", foreign)
		}
	}
}

// The migration number must not collide with one already on the branch.
func TestMigrationNumberIsUnique(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "..", "migrations"))
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	seen := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "000061_") && strings.HasSuffix(entry.Name(), ".up.sql") {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("found %d migrations numbered 000061, want exactly 1", seen)
	}
}
