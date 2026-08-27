package commercemission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommerceDeliveryMigrationShape(t *testing.T) {
	root := filepath.Join("..", "..", "..", "migrations")
	up, err := os.ReadFile(filepath.Join(root, "000042_commerce_multistop_delivery.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000042_commerce_multistop_delivery.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	upSQL, downSQL := string(up), string(down)
	for _, required := range []string{
		"commerce_mission_stops", "commerce_mission_parcels", "commerce_mission_operations",
		"commerce_mission_assignment_attempts", "commerce_mission_legs",
		"uq_commerce_mission_caller_operation", "request_fingerprint", "plan_fingerprint",
		"quote_fingerprint", "source_version", "target_service", "CANCELLED",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"vendor_orders", "vendor_fulfilment", "vendor_service"} {
		if strings.Contains(strings.ToLower(upSQL), forbidden) {
			t.Errorf("Dispatch migration improperly owns Vendor schema %q", forbidden)
		}
	}
	for _, required := range []string{
		"DROP TABLE IF EXISTS commerce_mission_legs",
		"DROP TABLE IF EXISTS commerce_mission_parcels",
		"DROP TABLE IF EXISTS commerce_mission_stops",
		"DROP COLUMN IF EXISTS target_service",
	} {
		if !strings.Contains(downSQL, required) {
			t.Errorf("down migration missing %q", required)
		}
	}
	if strings.Index(downSQL, "commerce_mission_parcels") > strings.Index(downSQL, "commerce_mission_stops") {
		t.Error("down migration must drop parcel children before stops")
	}
}
