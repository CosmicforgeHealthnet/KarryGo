package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSection14BMigrationShape(t *testing.T) {
	path := filepath.Join(
		"..",
		"..",
		"migrations",
		"000060_vendor_inventory_items.up.sql",
	)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	sql := string(raw)

	required := []string{
		"CREATE TABLE inventory_items",
		"business_id text NOT NULL",
		"branch_id text NOT NULL",
		"item_type text NOT NULL",
		"'SHOP_VARIANT'",
		"'INGREDIENT'",
		"'PACKAGING'",
		"'PRODUCTION_COMPONENT'",
		"source_ref text NOT NULL",
		"stock_measure_ref text NOT NULL",
		"tracking_mode text NOT NULL",
		"lot_policy text NOT NULL",
		"lot_tracking_enabled boolean NOT NULL",
		"CREATE UNIQUE INDEX inventory_items_logical_uq",
		"CREATE TABLE inventory_revisions",
		"inventory_items_validate_shop_variant",
		"REFERENCES vendor_branches",
	}

	for _, needle := range required {
		if !strings.Contains(sql, needle) {
			t.Fatalf(
				"14B migration missing %q",
				needle,
			)
		}
	}

	for _, forbidden := range []string{
		"CREATE TABLE inventory_balances",
		"CREATE TABLE inventory_movements",
		"CREATE TABLE inventory_lots",
		"CREATE TABLE inventory_reservations",
		"CREATE TABLE inventory_transfers",
		"CREATE TABLE inventory_stocktakes",
		"CREATE TABLE inventory_alerts",
		"CREATE TABLE ingredients",
		"CREATE TABLE recipes",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf(
				"14B leaked later authority %q",
				forbidden,
			)
		}
	}
}
