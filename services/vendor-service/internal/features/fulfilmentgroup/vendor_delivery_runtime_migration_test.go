package fulfilmentgroup

import (
	"os"
	"strings"
	"testing"
)

func TestVendorDeliveryRuntimeMigrationShape(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000104_vendor_owned_delivery_runtime.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{"commerce_vendor_rider_availability", "commerce_vendor_mission_assignments", "commerce_vendor_mission_runtime", "commerce_vendor_mission_operations", "commerce_vendor_hybrid_fallbacks", "one_active_assignment", "current_vendor_mission_id", "source_plan_version", "platform_mission_ref_id", "wallet_transition_ref"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToLower(sql), "otp_hash") || strings.Contains(strings.ToLower(sql), "qr_secret") {
		t.Fatal("Vendor migration stores handover secrets")
	}
}
