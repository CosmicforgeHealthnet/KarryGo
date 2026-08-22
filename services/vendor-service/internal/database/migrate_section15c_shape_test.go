package database

import (
	"os"
	"strings"
	"testing"
)

func TestSection15CMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(
		"../../migrations/000069_vendor_goods_receiving_inventory_posting.up.sql",
	)
	if err != nil {
		t.Fatal(err)
	}

	sql := strings.ToLower(string(raw))

	for _, required := range []string{
		"vendor_goods_receipts",
		"vendor_goods_receipt_lines",
		"vendor_supplier_discrepancies",
		"vendor_purchase_cost_history",
		"vendor_goods_receipt_commands",
		"inventory_receipts_purchasing_source_uq",
		"vendor.purchasing.receive",
		"cost_variance",
		"wrong_item",
		"damaged",
		"short_delivery",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("missing 15C migration contract %q", required)
		}
	}

	for _, forbidden := range []string{
		"payment_wallet",
		"wallet_transaction",
		"wallet_transactions",
		"wallet_service",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf(
				"15C incorrectly introduced Wallet authority: %s",
				forbidden,
			)
		}
	}
}
