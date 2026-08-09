package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestFleetAuthorityMigrationIsReversibleAndServiceLocal(t *testing.T) {
	up, err := os.ReadFile("008_fleet_authority.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("008_fleet_authority.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, required := range []string{"hauling_provider_availability", "hauling_organization_provider_links", "hauling_organization_truck_links", "hauling_organization_reservations", "hauling_command_operations", "hauling_domain_outbox", "hauling_service_nonces", "request_fingerprint", "source_version"} {
		if !strings.Contains(text, required) {
			t.Errorf("up migration missing %s", required)
		}
	}
	for _, foreign := range []string{"fleet_organizations", "fleet_provider_memberships", "wallet_ledger", "dispatch_organization_reservations"} {
		if strings.Contains(text, foreign) {
			t.Errorf("migration crosses service boundary via %s", foreign)
		}
	}
	if !strings.Contains(string(down), "DROP TABLE IF EXISTS hauling_domain_outbox") {
		t.Error("down migration does not remove authority tables")
	}
}

func TestPrivateExecutionAuthorityMigration(t *testing.T) {
	up, err := os.ReadFile("010_private_execution_authority.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	statement := strings.ToLower(string(up))
	for _, fragment := range []string{"hauling_private_execution_mappings",
		"fleet_operation_id text", "fleet_private_order_id text", "request_fingerprint",
		"cancellation_status", "source_version"} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("private execution migration missing %q", fragment)
		}
	}
	down, err := os.ReadFile("010_private_execution_authority.down.sql")
	if err != nil || !strings.Contains(string(down), "DROP TABLE IF EXISTS hauling_private_execution_mappings") {
		t.Errorf("private execution down migration is incomplete: %v", err)
	}
}

func TestRestrictionAuthorityMigration(t *testing.T) {
	up, err := os.ReadFile("011_authority_restrictions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	statement := strings.ToLower(string(up))
	for _, fragment := range []string{
		"hauling_authority_restrictions",
		"hauling_authority_restriction_history",
		"hauling_authority_restriction_operations",
		"source_version bigint",
		"request_fingerprint text",
		"append_only",
	} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("restriction migration missing %q", fragment)
		}
	}
	down, err := os.ReadFile("011_authority_restrictions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS hauling_authority_restriction_operations",
		"DROP TABLE IF EXISTS hauling_authority_restriction_history",
		"DROP TABLE IF EXISTS hauling_authority_restrictions",
	} {
		if !strings.Contains(string(down), fragment) {
			t.Errorf("restriction down migration missing %q", fragment)
		}
	}
}
