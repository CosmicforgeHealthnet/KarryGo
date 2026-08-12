package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMediaAssetsMigrationContainsRequiredTableAndIndexes(t *testing.T) {
	content, err := os.ReadFile("001_media_assets.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(content)
	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS media_assets",
		"CREATE INDEX IF NOT EXISTS idx_media_assets_owner",
		"CREATE INDEX IF NOT EXISTS idx_media_assets_purpose",
		"CREATE INDEX IF NOT EXISTS idx_media_assets_status",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected migration to contain %s", expected)
		}
	}
}

// TestMediaObjectAuthorityMigrationShape checks the properties of the WP10
// migration that a later edit could quietly remove: the tables, the tenant and
// state indexes, the idempotency constraints and the append-only triggers.
func TestMediaObjectAuthorityMigrationShape(t *testing.T) {
	content, err := os.ReadFile("000002_media_object_authority.up.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(content)

	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS media_objects",
		"CREATE TABLE IF NOT EXISTS media_upload_authorizations",
		"CREATE TABLE IF NOT EXISTS media_object_history",
		"CREATE TABLE IF NOT EXISTS media_audit_records",
		"CREATE TABLE IF NOT EXISTS media_outbox",
		"CREATE TABLE IF NOT EXISTS media_service_request_nonces",
		"CREATE TABLE IF NOT EXISTS media_worker_leases",
		"CREATE TABLE IF NOT EXISTS media_deletion_requests",
		// Tenant, owner, state, retention and version indexes.
		"idx_media_objects_tenant",
		"idx_media_objects_owner",
		"idx_media_objects_upload_status",
		"idx_media_objects_safety_status",
		"idx_media_objects_deletion_state",
		"idx_media_objects_retention",
		"idx_media_objects_version",
		// Idempotency and single-use constraints.
		"uq_media_upload_authorization_object",
		"uq_media_upload_authorization_idempotency",
		"uq_media_deletion_request_idempotency",
		"uq_media_object_storage_key",
		// Lease/retry indexes.
		"idx_media_outbox_claimable",
		"idx_media_deletion_request_status",
		// Append-only history and audit.
		"trg_media_object_history_append_only",
		"trg_media_audit_append_only",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected migration to contain %s", expected)
		}
	}

	// Safety must never default to SAFE. A default of SAFE would make "we
	// never looked" indistinguishable from "we looked and it was clean".
	if !strings.Contains(sql, "safety_status            text NOT NULL DEFAULT 'PENDING'") {
		t.Fatal("safety_status must default to PENDING")
	}
	if strings.Contains(sql, "DEFAULT 'SAFE'") {
		t.Fatal("no column may default to SAFE")
	}

	// The reverse migration must exist and undo the tables.
	down, err := os.ReadFile("000002_media_object_authority.down.sql")
	if err != nil {
		t.Fatalf("ReadFile(down) error = %v", err)
	}
	downSQL := string(down)
	for _, expected := range []string{
		"DROP TABLE IF EXISTS media_objects",
		"DROP TABLE IF EXISTS media_upload_authorizations",
		"DROP TABLE IF EXISTS media_deletion_requests",
		"DROP TABLE IF EXISTS media_outbox",
	} {
		if !strings.Contains(downSQL, expected) {
			t.Fatalf("expected down migration to contain %s", expected)
		}
	}
}

// TestNoCrossServiceForeignKeys proves the media schema never depends on
// another service's table. A cross-service foreign key would make this
// service's writes fail whenever the other service's rows disagreed.
func TestNoCrossServiceForeignKeys(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	foreignServiceTables := []string{
		"fleet_partner_", "finance_", "dispatch_", "verification_", "support_",
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, readErr := os.ReadFile(entry.Name())
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", entry.Name(), readErr)
		}
		sql := strings.ToLower(string(content))
		for _, prefix := range foreignServiceTables {
			if strings.Contains(sql, "references "+prefix) {
				t.Fatalf("%s creates a cross-service foreign key to %s*", entry.Name(), prefix)
			}
		}
	}
}
