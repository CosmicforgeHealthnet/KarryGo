package deliveryrecovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliveryRecoveryMigrationShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000048_delivery_recovery_route_integrity.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	required := []string{"create table dispatch_delivery_attempts", "create table dispatch_delivery_contact_attempts", "dispatch_contact_attempt_append_only", "create table dispatch_delivery_recovery_actions", "dispatch_delivery_recovery_one_active", "create table dispatch_return_workflows", "create table dispatch_hub_hold_operations", "create table dispatch_hub_hold_checkpoints", "dispatch_hub_checkpoint_append_only", "create table dispatch_delivery_incidents", "create table dispatch_emergency_recovery_actions", "dispatch_emergency_recovery_append_only", "create table dispatch_expected_routes", "dispatch_expected_route_one_current", "dispatch_expected_route_history_guard", "create table dispatch_device_commands", "unique (provider_id, device_id, device_command_id)", "dispatch_handover_challenges_stage_check", "'return'"}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Errorf("migration missing %q", fragment)
		}
	}
}
