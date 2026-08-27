package walletrepositories

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliveryFundingMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "000009_commerce_delivery_funding.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, required := range []string{
		"commerce_delivery_funding_allocations", "customer_contribution_minor", "vendor_subsidy_minor",
		"platform_subsidy_minor", "request_fingerprint", "source_service", "source_reference",
		"customer_contribution_minor + vendor_subsidy_minor + platform_subsidy_minor = authoritative_delivery_minor",
		"platform_approval_ref", "platform_subsidy_minor = 0 or platform_approval_ref <> ''",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("wallet funding migration missing %q", required)
		}
	}
}
