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

func TestSection30CommerceOperationsMigrationIsBoundedAndReversible(t *testing.T) {
	up, err := os.ReadFile("015_section30_commerce_operations.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	statement := strings.ToLower(string(up))
	for _, fragment := range []string{
		"loading_duration_seconds", "between 0 and 28800", "alter table commerce_missions",
	} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("Section 30 migration missing %q", fragment)
		}
	}
	down, err := os.ReadFile("015_section30_commerce_operations.down.sql")
	if err != nil || !strings.Contains(strings.ToLower(string(down)), "drop column if exists loading_duration_seconds") {
		t.Errorf("Section 30 down migration is incomplete: %v", err)
	}
}

func TestSecureHandoverMigrationIsHashOnlyAndSingleActive(t *testing.T) {
	up, err := os.ReadFile("016_secure_handover_challenges.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	statement := strings.ToLower(string(up))
	for _, fragment := range []string{
		"hauling_handover_challenges", "otp_hash", "qr_token_hash",
		"challenge_ref", "version bigint", "where status = 'active'",
		"haulage_bookings_no_plaintext_handover", "pickup_code = ''", "delivery_code = ''",
	} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("secure handover migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"otp_plaintext", "qr_token text", "raw_otp"} {
		if strings.Contains(statement, forbidden) {
			t.Errorf("secure handover migration persists forbidden secret field %q", forbidden)
		}
	}
	down, err := os.ReadFile("016_secure_handover_challenges.down.sql")
	if err != nil || !strings.Contains(strings.ToLower(string(down)), "drop table if exists hauling_handover_challenges") {
		t.Errorf("secure handover down migration is incomplete: %v", err)
	}
}
