package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSection20TimingProjectionMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000092_vendor_production_timing_projection.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"logistics_window_start",
		"logistics_window_end",
		"customer_eta_status",
		"customer_eta_start",
		"customer_eta_end",
		"timing_revision",
		"timing_source_evidence",
		"pickup_not_before",
		"scheduled_pickup_at",
		"logistics_timing_status",
		"logistics_timing_version",
		"commerce_vendor_order_readiness_eta_idx",
		"commerce_order_missions_timing_idx",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("000092 migration missing %q", required)
		}
	}
}
