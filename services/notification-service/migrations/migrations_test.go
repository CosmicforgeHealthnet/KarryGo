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
