package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestCustomerAuthMigrationContainsRequiredTables(t *testing.T) {
	content, err := os.ReadFile("001_customer_auth.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(content)
	for _, table := range []string{"customers", "customer_sessions", "customer_auth_events"} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("expected migration to create %s", table)
		}
	}
	for _, fragment := range []string{"email text UNIQUE", "idx_customer_auth_events_email"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}
