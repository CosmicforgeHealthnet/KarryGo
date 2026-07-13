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

func TestBookingDetailsMigrationAddsColumnsAndRenames(t *testing.T) {
	content, err := os.ReadFile("007_partner_booking_details.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(content)
	for _, want := range []string{
		"RENAME COLUMN dest_address TO dropoff_address",
		"RENAME COLUMN dest_lat TO dropoff_lat",
		"RENAME COLUMN dest_lng TO dropoff_lng",
		"ADD COLUMN IF NOT EXISTS distance_km",
		"ADD COLUMN IF NOT EXISTS package_desc",
		"ADD COLUMN IF NOT EXISTS package_weight",
		"ADD COLUMN IF NOT EXISTS currency",
		"ADD COLUMN IF NOT EXISTS pickup_contact_name",
		"ADD COLUMN IF NOT EXISTS pickup_contact_phone",
		"ADD COLUMN IF NOT EXISTS rider_id",
		"ADD COLUMN IF NOT EXISTS rider_name",
		"ADD COLUMN IF NOT EXISTS rider_phone",
		"ADD COLUMN IF NOT EXISTS rider_rating",
		"ADD COLUMN IF NOT EXISTS vehicle_color",
		"ADD COLUMN IF NOT EXISTS vehicle_model",
		"ADD COLUMN IF NOT EXISTS plate_number",
		"arrived",
		"partner_bookings_status_check",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("007_partner_booking_details.sql missing %q", want)
		}
	}
	if !strings.Contains(sql, "information_schema.columns") {
		t.Fatal("dest_* -> dropoff_* rename must be guarded via information_schema checks")
	}
}

func TestBookingDetailsDownReverts(t *testing.T) {
	content, err := os.ReadFile("007_partner_booking_details_down.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(content)
	for _, want := range []string{
		"DROP COLUMN IF EXISTS distance_km",
		"DROP COLUMN IF EXISTS package_desc",
		"DROP COLUMN IF EXISTS package_weight",
		"DROP COLUMN IF EXISTS pickup_contact_name",
		"DROP COLUMN IF EXISTS rider_id",
		"DROP COLUMN IF EXISTS plate_number",
		"RENAME COLUMN dropoff_address TO dest_address",
		"RENAME COLUMN dropoff_lat TO dest_lat",
		"RENAME COLUMN dropoff_lng TO dest_lng",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("007_partner_booking_details_down.sql missing %q", want)
		}
	}
	if strings.Contains(sql, "DROP COLUMN IF EXISTS currency") {
		t.Fatal("007 rollback must not drop currency (owned by 001)")
	}
}

func TestBillingChargeMigrationExtendsTables(t *testing.T) {
	content, err := os.ReadFile("008_partner_billing.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(content)
	for _, want := range []string{
		"ALTER TABLE partner_invoices",
		"ADD COLUMN IF NOT EXISTS partner_email text",
		"ADD COLUMN IF NOT EXISTS due_at timestamptz",
		"ADD COLUMN IF NOT EXISTS wallet_reference text",
		"ADD COLUMN IF NOT EXISTS failure_reason text",
		"ADD COLUMN IF NOT EXISTS updated_at timestamptz",
		"partner_invoices_status_check",
		"charge_failed",
		"CREATE TABLE IF NOT EXISTS partner_billing_authorizations",
		"CREATE TABLE IF NOT EXISTS partner_invoice_charge_attempts",
		"idempotency_key text NOT NULL UNIQUE",
		"amount_kobo bigint NOT NULL",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("008_partner_billing.sql missing %q", want)
		}
	}
	if strings.Contains(sql, "CREATE TABLE IF NOT EXISTS partner_invoices") {
		t.Fatal("008 must extend the 006 partner_invoices table, not create it")
	}
}

func TestBillingChargeDownRevertsAdditions(t *testing.T) {
	content, err := os.ReadFile("008_partner_billing_down.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(content)
	for _, table := range []string{
		"partner_invoice_charge_attempts",
		"partner_billing_authorizations",
	} {
		if !strings.Contains(sql, "DROP TABLE IF EXISTS "+table) {
			t.Fatalf("008 rollback must drop %s", table)
		}
	}
	for _, want := range []string{
		"DROP COLUMN IF EXISTS partner_email",
		"DROP COLUMN IF EXISTS due_at",
		"DROP COLUMN IF EXISTS wallet_reference",
		"DROP COLUMN IF EXISTS failure_reason",
		"DROP COLUMN IF EXISTS updated_at",
		"CHECK (status IN ('pending', 'paid', 'failed'))",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("008_partner_billing_down.sql missing %q", want)
		}
	}
	if strings.Contains(sql, "DROP TABLE IF EXISTS partner_invoices") {
		t.Fatal("008 rollback must preserve partner_invoices because 006 owns it")
	}
}

func TestNoDestColumnsRemainInCodebaseContracts(t *testing.T) {
	// Guard: 007 replaces dest_* naming; after 001+007 the schema exposes only
	// dropoff_*. 001 is historical and still contains dest_* by design.
	content, err := os.ReadFile("007_partner_booking_details.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	sql := string(content)
	for _, want := range []string{"dropoff_address", "dropoff_lat", "dropoff_lng"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("007 must establish %s", want)
		}
	}
}
