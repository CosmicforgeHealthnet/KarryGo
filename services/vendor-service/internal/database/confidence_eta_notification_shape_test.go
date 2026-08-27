package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfidenceETANotificationMigrationShape(t *testing.T) {
	root := filepath.Join("..", "..", "migrations")
	up, err := os.ReadFile(filepath.Join(root, "000106_confidence_eta_notifications.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000106_confidence_eta_notifications.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	upSQL, downSQL := string(up), string(down)
	for _, required := range []string{
		"commerce_delivery_eta_versions", "commerce_delivery_eta_current",
		"commerce_scheduled_notification_markers", "input_fingerprint",
		"source_event_id text NOT NULL", "UNIQUE(parent_order_id, mission_slot_ref, source_event_id)", "source_authority",
		"eta_version bigint NOT NULL", "policy_version", "components jsonb",
		"reject_commerce_eta_history_mutation", "commerce_delivery_eta_versions_immutable", "PROVIDER_REASSIGNED",
		"BREAKDOWN", "ROUTE_DEVIATION", "RETURNING",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"CREATE TABLE notification_messages", "CREATE TABLE trips", "CREATE TABLE wallet"} {
		if strings.Contains(upSQL, forbidden) {
			t.Errorf("Vendor migration creates foreign authority %q", forbidden)
		}
	}
	for _, required := range []string{"DROP TABLE IF EXISTS commerce_delivery_eta_current", "DROP TABLE IF EXISTS commerce_delivery_eta_versions", "DROP TABLE IF EXISTS commerce_scheduled_notification_markers"} {
		if !strings.Contains(downSQL, required) {
			t.Errorf("down migration missing %q", required)
		}
	}
}
