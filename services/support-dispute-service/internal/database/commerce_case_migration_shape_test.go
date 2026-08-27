package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommerceCaseMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "007_commerce_case_integration.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{"request_fingerprint", "commerce_category", "commerce_context", "resolution_version", "support_case_outbox", "UNIQUE(event_type, aggregate_ref, aggregate_version)", "WRONG_ITEM_OR_MEAL", "VENDOR_OWNED_DELIVERY_ISSUE"} {
		if !strings.Contains(sql, required) {
			t.Errorf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"section38", "CREATE TABLE wallet", "BYTEA"} {
		if strings.Contains(strings.ToUpper(sql), strings.ToUpper(forbidden)) {
			t.Errorf("migration contains forbidden authority/name %q", forbidden)
		}
	}
}
