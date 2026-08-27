package walletrepositories

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommerceAllocationAuthorityMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "000010_commerce_payment_allocation_authority.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, required := range []string{
		"commerce_payment_intents", "commerce_payment_allocations", "commerce_financial_operations",
		"max_authorized_minor", "snapshot_fingerprint", "vendor_released_minor", "delivery_released_minor",
		"partial_refund", "pending_liability", "other_funding_minor", "unique (operation_type, idempotency_key)",
		"commerce_allocation_snapshot_immutable", "commerce_intent_snapshot_immutable",
		"commerce_release_allocation_type_idx", "commerce_refund_allocation_item_idx",
		"customer_payable_minor + platform_product_discount_minor + platform_delivery_subsidy_minor + other_funding_minor =",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("commerce authority migration missing %q", required)
		}
	}
}

func TestCommerceRefundRecoveryAccountingMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "000011_commerce_refund_recovery_accounting.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, required := range []string{
		"refund_escrow_recovery_minor", "refund_vendor_recovery_minor", "refund_provider_recovery_minor",
		"refund_platform_recovery_minor", "refund_platform_loss_funding_minor",
		"refund_vendor_merchandise_reduction_minor", "refund_vendor_delivery_reduction_minor",
		"refund_provider_delivery_reduction_minor", "refund_platform_revenue_reduction_minor",
		"refund_authorization_release_minor",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("commerce refund recovery migration missing %q", required)
		}
	}
}
