package migrations

import (
	"os"
	"strings"
	"testing"
)

// The vehicle authority contract's only schema requirement is replay
// protection. The test guards the two properties that make it work: the
// (service, nonce) pair is the primary key, so a second use collides, and the
// rows expire, so the table does not grow without bound.
func TestVehicleAuthorityNonceMigration(t *testing.T) {
	up, err := os.ReadFile("000036_vehicle_authority_nonces.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	statement := string(up)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS dispatch_service_request_nonces",
		"PRIMARY KEY (service_name, nonce)",
		"request_fingerprint text NOT NULL",
		"expires_at          timestamptz NOT NULL",
		"idx_dispatch_service_request_nonces_expiry",
	} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}

	down, err := os.ReadFile("000036_vehicle_authority_nonces.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if !strings.Contains(string(down), "DROP TABLE IF EXISTS dispatch_service_request_nonces") {
		t.Error("down migration must drop the nonce table")
	}
}

func TestPrivateExecutionAuthorityMigration(t *testing.T) {
	up, err := os.ReadFile("000038_private_execution_authority.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	statement := strings.ToLower(string(up))
	for _, fragment := range []string{"dispatch_private_execution_mappings",
		"fleet_operation_id text", "fleet_private_order_id text", "request_fingerprint",
		"cancellation_status", "source_version"} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("private execution migration missing %q", fragment)
		}
	}
	down, err := os.ReadFile("000038_private_execution_authority.down.sql")
	if err != nil || !strings.Contains(string(down), "DROP TABLE IF EXISTS dispatch_private_execution_mappings") {
		t.Errorf("private execution down migration is incomplete: %v", err)
	}
}

func TestRestrictionAuthorityMigration(t *testing.T) {
	up, err := os.ReadFile("000039_authority_restrictions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	statement := strings.ToLower(string(up))
	for _, fragment := range []string{
		"dispatch_authority_restrictions",
		"dispatch_authority_restriction_history",
		"dispatch_authority_restriction_operations",
		"source_version bigint",
		"request_fingerprint text",
		"append_only",
	} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("restriction migration missing %q", fragment)
		}
	}
	down, err := os.ReadFile("000039_authority_restrictions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS dispatch_authority_restriction_operations",
		"DROP TABLE IF EXISTS dispatch_authority_restriction_history",
		"DROP TABLE IF EXISTS dispatch_authority_restrictions",
	} {
		if !strings.Contains(string(down), fragment) {
			t.Errorf("restriction down migration missing %q", fragment)
		}
	}
}
