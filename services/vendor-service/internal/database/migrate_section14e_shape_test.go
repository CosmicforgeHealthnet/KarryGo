package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSection14EMigrationShape(
	t *testing.T,
) {
	path :=
		filepath.Join(
			"..",
			"..",
			"migrations",
			"000063_vendor_inventory_reservations.up.sql",
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
			"CREATE TABLE inventory_reservations",
			"CREATE TABLE inventory_reservation_lines",
			"CREATE TABLE inventory_reservation_allocations",
			"'HELD'",
			"'CONFIRMED'",
			"'COMMITTED'",
			"'RELEASED'",
			"'EXPIRED'",
			"'CANCELLED'",
			"'ORDER'",
			"'CHECKOUT'",
			"'RFQ'",
			"'MANUAL'",
			"'INTERNAL'",
			"request_fingerprint",
			"ALL_OR_NOTHING",
			"UNIQUE (\n        business_id,\n        source_type,\n        source_ref",
			"inventory_reservations_expiry_idx",
			"stale_allocations",
			// 000062 created this view with numeric columns; 14E casts them to
			// bigint, and PostgreSQL refuses a column type change through
			// CREATE OR REPLACE. The view must be dropped and recreated or an
			// empty database can never reach the migration tip.
			"DROP VIEW IF EXISTS inventory_balance_availability",
			"CREATE VIEW inventory_balance_availability",
		}

	for _, needle := range required {

		if !strings.Contains(
			sql,
			needle,
		) {
			t.Fatalf(
				"14E migration missing %q",
				needle,
			)
		}
	}

	if strings.Contains(
		sql,
		"PARTIAL",
	) {
		t.Fatal(
			"14E introduced unapproved partial reservation policy",
		)
	}
}
