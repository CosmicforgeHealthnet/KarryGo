package migrations_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readMigration(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(".", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(content)
}

// migrationStatements returns the migration with comment lines removed.
//
// The forbidden-column checks below must look at what the migration DECLARES,
// not at what its prose discusses. Without this, a comment explaining why a
// column is absent would fail the test asserting its absence.
func migrationStatements(t *testing.T, name string) string {
	t.Helper()
	var statements []string
	for _, line := range strings.Split(readMigration(t, name), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		statements = append(statements, line)
	}
	return strings.Join(statements, "\n")
}

// TestDeliveryOutcomeMigrationFilesExist proves both halves of each WP6
// migration are present.
func TestDeliveryOutcomeMigrationFilesExist(t *testing.T) {
	for _, name := range []string{
		"000034_recipient_confirmations.up.sql", "000034_recipient_confirmations.down.sql",
		"000035_delivery_failures.up.sql", "000035_delivery_failures.down.sql",
	} {
		if _, err := os.Stat(filepath.Join(".", name)); err != nil {
			t.Fatalf("migration %s is missing: %v", name, err)
		}
	}
}

// TestRecipientConfirmationMigrationShape checks the constraints the
// confirmation contract depends on.
func TestRecipientConfirmationMigrationShape(t *testing.T) {
	up := readMigration(t, "000034_recipient_confirmations.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS recipient_confirmations",
		"verification_method TEXT NOT NULL",
		"recipient_reference TEXT NOT NULL",
		"recipient_matched_expected BOOLEAN NOT NULL",
		"expected_source_version BIGINT NOT NULL",
		"idempotency_key TEXT NOT NULL",
		"request_fingerprint TEXT NOT NULL",
		"uq_recipient_confirmation_trip",
		"uq_recipient_confirmation_idempotency",
		"ADD COLUMN IF NOT EXISTS recipient_confirmed_at TIMESTAMPTZ",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	// No recipient identity is stored here. The trip and the proof already
	// hold the receiver's details; duplicating them into a record that feeds
	// an event stream is exactly the leak this design avoids.
	statements := migrationStatements(t, "000034_recipient_confirmations.up.sql")
	for _, forbidden := range []string{
		"recipient_name", "recipient_phone", "receiver_name", "receiver_phone",
	} {
		if strings.Contains(statements, forbidden) {
			t.Errorf("recipient_confirmations must not store %q", forbidden)
		}
	}
	// Confirmation is not a lifecycle move: no new trip status is introduced.
	if strings.Contains(up, "'recipient_confirmed'") {
		t.Error("recipient confirmation must not add a trip status")
	}

	down := readMigration(t, "000034_recipient_confirmations.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS recipient_confirmations",
		"DROP COLUMN IF EXISTS recipient_confirmed_at",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

// TestDeliveryFailureMigrationShape checks the constraints the failure
// contract depends on.
func TestDeliveryFailureMigrationShape(t *testing.T) {
	up := readMigration(t, "000035_delivery_failures.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS delivery_failures",
		"reason_category TEXT NOT NULL",
		"retry_eligible BOOLEAN NOT NULL",
		"recipient_unavailable BOOLEAN NOT NULL",
		"incident_reference TEXT NULL",
		"expected_source_version BIGINT NOT NULL",
		"idempotency_key TEXT NOT NULL",
		"request_fingerprint TEXT NOT NULL",
		"uq_delivery_failure_trip",
		"uq_delivery_failure_idempotency",
		"length(operator_notes) <= 500",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	// The incident reference points at another service's case, so it must not
	// carry a foreign key.
	if strings.Contains(up, "incident_reference TEXT NULL REFERENCES") {
		t.Error("incident_reference must not carry a cross-service foreign key")
	}
	// A failure record must have nowhere to record money. Whether a failed
	// delivery is refunded or penalised is the wallet service's to say.
	statements := migrationStatements(t, "000035_delivery_failures.up.sql")
	for _, forbidden := range []string{
		"refund_amount", "penalty_amount", "fare_amount", "settlement_amount",
	} {
		if strings.Contains(statements, forbidden) {
			t.Errorf("delivery_failures must not store %q", forbidden)
		}
	}
	// `failed` already exists in the trip state machine; the migration adds
	// evidence, not a parallel status.
	if strings.Contains(up, "ALTER TABLE trips") && !strings.Contains(up, "failed_at") {
		t.Error("the only trip column this migration adds is failed_at")
	}

	down := readMigration(t, "000035_delivery_failures.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS delivery_failures",
		"DROP COLUMN IF EXISTS failed_at",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}
