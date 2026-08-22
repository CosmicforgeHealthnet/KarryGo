package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSection14DMigrationShape(
	t *testing.T,
) {
	path := filepath.Join(
		"..",
		"..",
		"migrations",
		"000062_vendor_inventory_core_stock.up.sql",
	)

	raw, err :=
		os.ReadFile(path)

	if err != nil {
		t.Fatal(err)
	}

	sql := string(raw)

	required := []string{
		"CREATE TABLE inventory_balances",
		"CREATE TABLE inventory_lots",
		"CREATE TABLE inventory_movements",
		"CREATE TABLE inventory_receipts",
		"CREATE TABLE inventory_receipt_lines",
		"CREATE VIEW inventory_balance_availability",
		"'RECEIVE'",
		"'RESERVE'",
		"'RELEASE'",
		"'COMMIT'",
		"'ADJUSTMENT_IN'",
		"'ADJUSTMENT_OUT'",
		"'TRANSFER_OUT'",
		"'TRANSFER_IN'",
		"'RETURN'",
		"'DAMAGE'",
		"'WASTE'",
		"'EXPIRY'",
		"'QUARANTINE_IN'",
		"'QUARANTINE_RELEASE'",
		"'STOCKTAKE_CORRECTION'",
		"inventory_movements_immutable",
		"inventory_movements_immutable_trigger",
		"inventory_authorized_mutation_required",
		"karrygo.inventory_mutation",
	}

	for _, needle := range required {

		if !strings.Contains(
			sql,
			needle,
		) {
			t.Fatalf(
				"14D migration missing %q",
				needle,
			)
		}
	}

	if strings.Contains(
		sql,
		"available_quantity bigint",
	) {
		t.Fatal(
			"14D persisted independently editable available quantity",
		)
	}
}

func TestSection14DMovementLedgerCannotBeUpdatedOrDeleted(
	t *testing.T,
) {
	path := filepath.Join(
		"..",
		"..",
		"migrations",
		"000062_vendor_inventory_core_stock.up.sql",
	)

	raw, err :=
		os.ReadFile(path)

	if err != nil {
		t.Fatal(err)
	}

	sql := string(raw)

	required := []string{
		"BEFORE UPDATE OR DELETE",
		"ON inventory_movements",
		"inventory movement ledger is append-only",
	}

	for _, needle := range required {

		if !strings.Contains(
			sql,
			needle,
		) {
			t.Fatalf(
				"movement immutability guard missing %q",
				needle,
			)
		}
	}
}
