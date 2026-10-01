package migrations

import (
	"os"
	"strings"
	"testing"
)

// The operator repair columns exist so a requeue is auditable. A requeue must
// stay a REDELIVERY: nothing that identifies the event may be changed by one,
// which is why this migration adds only counters and never touches payload,
// payload_hash or aggregate_version.
func TestOutboxOperatorRepairMigration(t *testing.T) {
	up, err := os.ReadFile("000057_outbox_operator_repair.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	statement := string(up)
	for _, fragment := range []string{
		"ALTER TABLE dispatch_marketplace_outbox",
		"ADD COLUMN IF NOT EXISTS requeue_count INT NOT NULL DEFAULT 0",
		"ADD COLUMN IF NOT EXISTS last_requeued_at TIMESTAMPTZ NULL",
		"ADD COLUMN IF NOT EXISTS last_requeued_by TEXT NOT NULL DEFAULT ''",
		"CHECK (requeue_count >= 0)",
		"idx_dispatch_outbox_operator_state",
	} {
		if !strings.Contains(statement, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "payload_hash =", "aggregate_version ="} {
		if strings.Contains(statement, forbidden) {
			t.Errorf("the repair migration alters event identity: %q", forbidden)
		}
	}

	down, err := os.ReadFile("000057_outbox_operator_repair.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	reverse := string(down)
	for _, fragment := range []string{
		"DROP INDEX IF EXISTS idx_dispatch_outbox_operator_state",
		"DROP CONSTRAINT IF EXISTS chk_dispatch_outbox_requeue_count",
		"DROP COLUMN IF EXISTS requeue_count",
		"DROP COLUMN IF EXISTS last_requeued_at",
		"DROP COLUMN IF EXISTS last_requeued_by",
	} {
		if !strings.Contains(reverse, fragment) {
			t.Errorf("down migration is not a full reversal, missing %q", fragment)
		}
	}
	if strings.Contains(reverse, "DROP TABLE") {
		t.Error("the down migration drops the outbox itself, losing undelivered events")
	}
}
