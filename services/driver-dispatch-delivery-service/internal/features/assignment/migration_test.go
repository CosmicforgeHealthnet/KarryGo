package assignment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssignmentPlanningMigrationShape(t *testing.T) {
	root := filepath.Join("..", "..", "..", "migrations")
	up, err := os.ReadFile(filepath.Join(root, "000045_assignment_queue_mission_planning.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000045_assignment_queue_mission_planning.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(up)
	for _, fragment := range []string{"dispatch_assignment_one_live_offer", "dispatch_assignment_one_winner", "dispatch_queue_one_active_mission", "dispatch_mission_plan_one_active", "dispatch_active_custody_manifest_parcels", "dispatch_stop_expected_parcels", "dispatch_reject_manifest_mutation"} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("missing %q", fragment)
		}
	}
	if strings.Contains(sql, "CREATE TABLE trips") || strings.Contains(sql, "wallet_ledger") {
		t.Fatal("migration duplicates an authority")
	}
	if !strings.Contains(string(down), "DROP TABLE IF EXISTS dispatch_assignment_policies") {
		t.Fatal("down migration incomplete")
	}
}

func TestAssignmentExecutionIntegrityMigrationShape(t *testing.T) {
	root := filepath.Join("..", "..", "..", "migrations")
	up, err := os.ReadFile(filepath.Join(root, "000046_assignment_execution_and_manifest_integrity.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join(root, "000046_assignment_execution_and_manifest_integrity.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(up)
	for _, fragment := range []string{"dispatch_mission_execution_links", "dispatch_delivery_mission_one_booking", "priority_policy_version", "search_policy_version", "dispatch_assignment_audit_append_only", "karrygo_parcel_ref", "scannable_code", "dispatch_manifest_logical_version_unique", "dispatch_replan_operations", "dispatch_manifest_custody_operations"} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("missing %q", fragment)
		}
	}
	if strings.Contains(sql, "CREATE TABLE trips") || strings.Contains(sql, "wallet_ledger") {
		t.Fatal("migration duplicates canonical authority")
	}
	if !strings.Contains(string(down), "DROP TABLE IF EXISTS dispatch_mission_execution_links") {
		t.Fatal("down migration incomplete")
	}
}
