package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommerceSupportCaseProjectionMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000107_commerce_support_case_projection.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{"commerce_support_case_links", "UNIQUE(customer_id, operation_ref)", "UNIQUE(support_case_ref)", "request_fingerprint", "resolution_version", "commerce_support_resolution_inbox", "payload_hash"} {
		if !strings.Contains(sql, required) {
			t.Errorf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"CREATE TABLE wallet", "CREATE TABLE support_cases", "section38"} {
		if strings.Contains(strings.ToLower(sql), strings.ToLower(forbidden)) {
			t.Errorf("migration contains forbidden authority/name %q", forbidden)
		}
	}
}
