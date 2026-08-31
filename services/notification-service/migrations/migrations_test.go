package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestNotificationMigrationContainsRequiredTables(t *testing.T) {
	content, err := os.ReadFile("001_notifications.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(content)
	for _, table := range []string{
		"notification_messages",
		"notification_deliveries",
		"notification_delivery_attempts",
		"notification_templates",
		"notification_preferences",
		"notification_devices",
	} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("expected migration to create %s", table)
		}
	}
}

func TestCommerceSemanticNotificationMigration(t *testing.T) {
	content, err := os.ReadFile("002_commerce_semantic_notifications.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, required := range []string{
		"source_aggregate_type", "source_aggregate_id", "source_event_id",
		"source_version", "correlation_ref",
		"uq_notification_messages_semantic_source_recipient",
		"commerce.vendor.new_order", "commerce.vendor.scheduled_cutoff",
		"commerce.customer.delay", "commerce.customer_eta_changed",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("commerce migration missing %q", required)
		}
	}
	if strings.Contains(sql, "CREATE TABLE commerce_vendor_orders") || strings.Contains(sql, "CREATE TABLE commerce_delivery_eta_versions") {
		t.Fatal("Notification migration creates business-domain authority")
	}
}
