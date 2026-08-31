package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformDeliveryOperationalProjectionMigrationShape(t *testing.T) {
	root := filepath.Join("..", "..", "migrations")
	up, err := os.ReadFile(filepath.Join(root, "000105_platform_delivery_operational_projection.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000105_platform_delivery_operational_projection.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	upSQL, downSQL := string(up), string(down)
	for _, required := range []string{
		"commerce_platform_operational_events", "event_id text NOT NULL UNIQUE",
		"canonical_mission_ref", "payload_hash", "commerce.provider_transfer.completed.v1",
		"commerce.return.completed.v1", "Append-only Vendor projection",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"CREATE TABLE commerce_missions", "CREATE TABLE wallet", "CREATE TABLE trips"} {
		if strings.Contains(upSQL, forbidden) {
			t.Errorf("Vendor projection migration creates foreign authority %q", forbidden)
		}
	}
	if !strings.Contains(downSQL, "DROP TABLE IF EXISTS commerce_platform_operational_events") {
		t.Error("down migration does not remove the projection table")
	}
}
