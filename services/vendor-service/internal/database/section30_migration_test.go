package database

import (
	"os"
	"strings"
	"testing"
)

func TestSection30LoadingDurationMigrationIsBoundedAndReversible(t *testing.T) {
	up, err := os.ReadFile("../../migrations/000110_section30_hauling_loading_duration.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	statement := strings.ToLower(string(up))
	for _, fragment := range []string{
		"commerce_fulfilment_missions", "commerce_fulfilment_plan_missions",
		"loading_duration_seconds", "between 0 and 28800",
	} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("Section 30 Vendor migration missing %q", fragment)
		}
	}
	down, err := os.ReadFile("../../migrations/000110_section30_hauling_loading_duration.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.ToLower(string(down)), "drop column if exists loading_duration_seconds") != 2 {
		t.Fatal("Section 30 Vendor down migration does not remove both additive columns")
	}
}
