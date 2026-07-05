package migrations

import (
	"os"
	"strings"
	"testing"
)

var partnerTables = []string{
	"partner_accounts",
	"partner_api_keys",
	"partner_bookings",
	"partner_webhooks",
	"partner_webhook_deliveries",
	"partner_idempotency_keys",
}

func TestPartnerSchemaMigrationCreatesRequiredTables(t *testing.T) {
	content, err := os.ReadFile("001_partner_schema.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(content)
	for _, table := range partnerTables {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("expected migration to create %s", table)
		}
	}
}

func TestPartnerBookingIdempotencyIsPartnerScoped(t *testing.T) {
	content, err := os.ReadFile("001_partner_schema.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(content)
	if !strings.Contains(sql, "UNIQUE (partner_id, idempotency_key)") {
		t.Fatal("expected partner_bookings idempotency to be partner-scoped via UNIQUE (partner_id, idempotency_key)")
	}
	if strings.Contains(sql, "idempotency_key text NOT NULL UNIQUE") {
		t.Fatal("partner_bookings idempotency_key must not be globally UNIQUE")
	}
}

func TestPartnerSchemaDownDropsAllTables(t *testing.T) {
	content, err := os.ReadFile("001_partner_schema_down.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(content)
	for _, table := range partnerTables {
		if !strings.Contains(sql, "DROP TABLE IF EXISTS "+table) {
			t.Fatalf("expected rollback to drop %s", table)
		}
	}
}

func TestPartnerAuthMigrationAddsPasswordAndSessions(t *testing.T) {
	content, err := os.ReadFile("002_partner_auth.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(content)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS password_hash",
		"CREATE TABLE IF NOT EXISTS partner_sessions",
		"token_hash text NOT NULL UNIQUE",
		"CREATE INDEX IF NOT EXISTS idx_sessions_partner",
		"CREATE INDEX IF NOT EXISTS idx_sessions_token",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("002_partner_auth.sql missing %q", want)
		}
	}
}

func TestPartnerAuthDownReverts(t *testing.T) {
	content, err := os.ReadFile("002_partner_auth_down.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(content)
	if !strings.Contains(sql, "DROP TABLE IF EXISTS partner_sessions") {
		t.Fatal("rollback must drop partner_sessions")
	}
	if !strings.Contains(sql, "DROP COLUMN IF EXISTS password_hash") {
		t.Fatal("rollback must drop password_hash")
	}
}
