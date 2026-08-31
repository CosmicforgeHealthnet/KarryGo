package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSection14GMigrationShape(
	t *testing.T,
) {
	path :=
		filepath.Join(
			"..",
			"..",
			"migrations",
			"000065_vendor_inventory_availability_signals.up.sql",
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
			"CREATE TABLE inventory_availability_signal_state",
			"is_available boolean",
			"available_quantity bigint",
			"inventory_revision bigint",
			"inventory_availability_signal_state_item_fk",
		}

	for _, needle := range required {

		if !strings.Contains(
			sql,
			needle,
		) {
			t.Fatalf(
				"14G migration missing %q",
				needle,
			)
		}
	}

	for _, forbidden := range []string{
		"CREATE TABLE recipes",
		"CREATE TABLE ingredients",
		"CREATE TABLE recipe_ingredients",
		"CREATE TABLE production_batches",
		"yield_quantity",
		"portion_quantity",
	} {

		if strings.Contains(
			strings.ToLower(sql),
			strings.ToLower(
				forbidden,
			),
		) {
			t.Fatalf(
				"14G introduced forbidden Section 16 authority %q",
				forbidden,
			)
		}
	}
}
