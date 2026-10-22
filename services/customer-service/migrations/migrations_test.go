package migrations

import (
	"os"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/admincommand"
)

func TestCustomerAuthMigrationContainsRequiredTables(t *testing.T) {
	content, err := os.ReadFile("001_customer_auth.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(content)
	for _, table := range []string{"customers", "customer_sessions", "customer_auth_events"} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("expected migration to create %s", table)
		}
	}
	for _, fragment := range []string{"email text UNIQUE", "idx_customer_auth_events_email"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to contain %q", fragment)
		}
	}
}

func TestAdminCommandLedgerMigrationEmbedsSharedSchemaVerbatim(t *testing.T) {
	content, err := os.ReadFile("018_admin_command_ledger.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(content)
	if !strings.Contains(sql, "-- shared-template-sha256: "+admincommand.SchemaSHA256()+"\n") {
		t.Fatal("ledger migration header does not pin the current shared schema digest")
	}
	if !strings.HasSuffix(sql, admincommand.Schema) {
		t.Fatal("ledger migration drifted from shared/go/admincommand/schema.sql")
	}
}

func TestAccountStandingMigrationHistoryIsAppendOnly(t *testing.T) {
	content, err := os.ReadFile("019_admin_account_standing.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(content)
	for _, table := range []string{"customer_account_standing_history", "customer_privacy_request_transitions", "customer_support_decisions"} {
		if !strings.Contains(sql, "BEFORE UPDATE OR DELETE OR TRUNCATE ON "+table+" ") {
			t.Fatalf("%s is not append-only", table)
		}
	}
}
