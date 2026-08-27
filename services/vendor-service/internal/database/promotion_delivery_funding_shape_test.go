package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromotionDeliveryFundingMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000097_promotion_campaigns_and_delivery_funding.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, required := range []string{
		"commerce_promotion_campaigns", "commerce_promotion_scopes", "commerce_promotion_bundle_requirements",
		"commerce_promotion_quantity_tiers", "commerce_promotion_coupons", "commerce_promotion_redemptions",
		"promotion_snapshot", "delivery_funding_snapshot", "commerce_delivery_funding_operations",
		"platform_approval_ref", "vendor.promotions.read", "vendor.promotions.manage",
		"hybrid_fallback_funding_sponsor", "hybrid_vendor_share_bps", "hybrid_platform_approval_ref",
		"customer_contribution_minor + vendor_subsidy_minor + platform_subsidy_minor = authoritative_delivery_minor",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("promotion/delivery-funding migration missing %q", required)
		}
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000097_promotion_campaigns_and_delivery_funding.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"drop table if exists commerce_promotion_campaigns", "drop table if exists commerce_delivery_funding_operations"} {
		if !strings.Contains(strings.ToLower(string(down)), required) {
			t.Fatalf("rollback missing %q", required)
		}
	}
}
