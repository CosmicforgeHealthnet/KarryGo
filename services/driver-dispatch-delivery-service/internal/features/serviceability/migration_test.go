package serviceability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliveryServiceabilityMigrationShape(t *testing.T) {
	root := filepath.Join("..", "..", "..", "migrations")
	up, err := os.ReadFile(filepath.Join(root, "000044_delivery_serviceability_foundation.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000044_delivery_serviceability_foundation.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	upSQL, downSQL := string(up), string(down)
	for _, required := range []string{"dispatch_delivery_groups", "dispatch_delivery_missions", "dispatch_mission_stops", "dispatch_parcels", "dispatch_parcel_manifests", "dispatch_manifest_parcels", "dispatch_serviceability_quotes", "request_fingerprint", "source_version", "package_policy_version", "UNIQUE(customer_id, idempotency_key)", "dispatch_reject_manifest_mutation"} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"DROP TABLE dispatch_bookings", "ALTER TABLE dispatch_bookings ALTER", "CREATE TABLE trips", "wallet_ledger"} {
		if strings.Contains(upSQL, forbidden) {
			t.Errorf("migration crosses compatibility boundary with %q", forbidden)
		}
	}
	if strings.Index(downSQL, "dispatch_manifest_parcels") > strings.Index(downSQL, "dispatch_parcel_manifests") {
		t.Fatal("down migration must drop manifest membership first")
	}
}
