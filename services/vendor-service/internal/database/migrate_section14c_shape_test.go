package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSection14CMigrationShape(t *testing.T) {
	path := filepath.Join(
		"..",
		"..",
		"migrations",
		"000061_vendor_inventory_shop_compatibility.up.sql",
	)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	sql := string(raw)

	required := []string{
		"inventory_items_shop_variant_identity_uq",
		"inventory_shop_compat_backfill_jobs",
		"inventory_shop_compat_backfill_batches",
		"cursor_stock_basis_id",
		"processed_count",
		"batch_count",
		"attempt_count",
	}

	for _, needle := range required {
		if !strings.Contains(
			sql,
			needle,
		) {
			t.Fatalf(
				"14C migration missing %q",
				needle,
			)
		}
	}

	// 14C must not reconstruct operational stock state.
	for _, forbidden := range []string{
		"CREATE TABLE inventory_balances",
		"CREATE TABLE inventory_movements",
		"CREATE TABLE inventory_lots",
		"CREATE TABLE inventory_reservations",
		"INSERT INTO inventory_balances",
		"INSERT INTO inventory_movements",
		"INSERT INTO inventory_lots",
		"INSERT INTO inventory_reservations",
	} {
		if strings.Contains(
			sql,
			forbidden,
		) {
			t.Fatalf(
				"14C attempted operational-state reconstruction: %q",
				forbidden,
			)
		}
	}
}
