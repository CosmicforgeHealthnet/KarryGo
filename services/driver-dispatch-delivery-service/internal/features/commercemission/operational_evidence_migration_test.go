package commercemission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperationalEvidenceExceptionAndTransferMigrationShape(t *testing.T) {
	root := filepath.Join("..", "..", "..", "migrations")
	up, err := os.ReadFile(filepath.Join(root, "000043_operational_evidence_exceptions_provider_transfer.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000043_operational_evidence_exceptions_provider_transfer.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	upSQL, downSQL := string(up), string(down)
	for _, required := range []string{
		"commerce_mission_evidence", "commerce_delivery_exceptions", "commerce_return_operations",
		"commerce_provider_transfers", "commerce_mission_carrier_history",
		"commerce_mission_evidence_immutable", "commerce_mission_parcel_manifest_frozen",
		"commerce_mission_stop_manifest_frozen", "commerce_provider_transfers_one_open_idx",
		"commerce_mission_carrier_history_one_active_idx", "commerce_mission_carrier_history_append_only",
		"completion_evidence_id", "manifest_binding",
		"old_provider_id <> new_provider_id", "PREPARED_FOOD", "SHOP_RETAIL",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"wallet_transactions", "vendor_orders", "hauling_bookings"} {
		if strings.Contains(strings.ToLower(upSQL), forbidden) {
			t.Errorf("Dispatch migration improperly owns another authority %q", forbidden)
		}
	}
	for _, required := range []string{
		"DROP TABLE IF EXISTS commerce_mission_evidence",
		"DROP TABLE IF EXISTS commerce_provider_transfers",
		"DROP TRIGGER IF EXISTS commerce_mission_stop_manifest_frozen",
		"DROP COLUMN IF EXISTS replacement_state",
	} {
		if !strings.Contains(downSQL, required) {
			t.Errorf("down migration missing %q", required)
		}
	}
}
