package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliveryPolicyHybridMigrationShape(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "000096_delivery_policy_hybrid_execution.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, required := range []string{
		"delivery_mode", "fee_rule", "operating_timezone", "service_zones",
		"distance_tiers", "operating_periods", "fallback_triggers",
		"delivery_policy_snapshot", "customer_delivery_price_guarantee_minor",
		"commerce_delivery_execution", "commerce_delivery_custody_events",
		"unique (vendor_order_id)",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("delivery policy migration missing %q", required)
		}
	}
}
