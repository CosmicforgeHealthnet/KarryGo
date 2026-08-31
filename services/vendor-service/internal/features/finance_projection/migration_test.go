package finance_projection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVendorFinanceProjectionMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000098_vendor_finance_and_wallet_allocation_projection.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, required := range []string{"allocation_snapshot_fingerprint", "vendor.finance.withdraw", "role_owner", "role_finance"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
