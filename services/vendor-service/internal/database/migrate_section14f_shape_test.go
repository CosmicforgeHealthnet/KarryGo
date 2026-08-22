package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSection14FMigrationShape(
	t *testing.T,
) {
	path :=
		filepath.Join(
			"..",
			"..",
			"migrations",
			"000064_vendor_inventory_operations.up.sql",
		)

	raw, err :=
		os.ReadFile(path)

	if err != nil {
		t.Fatal(err)
	}

	sql :=
		string(raw)

	required :=
		[]string{
			"CREATE TABLE inventory_quarantines",
			"CREATE TABLE inventory_quarantine_allocations",
			"CREATE TABLE inventory_stocktakes",
			"CREATE TABLE inventory_stocktake_lines",
			"CREATE TABLE inventory_transfers",
			"CREATE TABLE inventory_transfer_lines",
			"CREATE TABLE inventory_transfer_receipts",
			"CREATE TABLE inventory_reorder_rules",
			"CREATE TABLE inventory_alerts",

			"'FULL_STOCKTAKE'",
			"'CYCLE_COUNT'",
			"'COUNTING'",
			"'POSTED'",
			"'STALE'",
			"'CANCELLED'",

			"'DRAFT'",
			"'APPROVED'",
			"'IN_TRANSIT'",
			"'PARTIALLY_RECEIVED'",
			"'RECEIVED'",

			"'ACTIVE'",
			"'RELEASED'",
			"'DISPOSED'",

			"'LOW_STOCK'",
			"'OUT_OF_STOCK'",
			"'EXPIRING_SOON'",
			"'EXPIRED'",
			"'BACKORDER'",
			"'BALANCE_MISMATCH'",
			"'TRANSFER_EXCEPTION'",

			"'OPEN'",
			"'ACKNOWLEDGED'",
			"'RESOLVED'",

			"request_fingerprint",
			"supplier_ref",
		}

	for _, needle := range required {

		if !strings.Contains(
			sql,
			needle,
		) {
			t.Fatalf(
				"14F migration missing %q",
				needle,
			)
		}
	}

	if strings.Contains(
		sql,
		"CREATE TABLE suppliers",
	) ||
		strings.Contains(
			sql,
			"REFERENCES suppliers",
		) {

		t.Fatal(
			"14F incorrectly introduced supplier authority",
		)
	}
}
