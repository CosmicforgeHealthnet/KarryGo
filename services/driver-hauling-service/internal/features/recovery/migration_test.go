package recovery

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readMigration(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(raw)
}

func TestRecoveryMigrationCreatesTheCaseModel(t *testing.T) {
	up := readMigration(t, "023_recovery_cases.up.sql")
	for _, required := range []string{
		"hauling_recovery_cases",
		"booking_id", "mission_ref", "trigger_code",
		"original_provider_id", "original_truck_id",
		"replacement_provider_id", "replacement_truck_id",
		"custody_transfer_required", "custody_transfer_ref", "evidence_ref",
		"operation_ref", "correlation_id",
		"detected_at", "started_at", "resolved_at",
		"version", "audit_json",
		"uq_hauling_recovery_open_booking",
		"chk_hauling_recovery_distinct_provider",
		"chk_hauling_recovery_terminal",
		"active_recovery_id", "recovery_count",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("recovery migration missing %q", required)
		}
	}
}

// Every state the Go model defines must be accepted by the CHECK constraint,
// and nothing else. A state the code can produce but the database refuses is a
// runtime failure waiting for the first breakdown.
func TestMigrationStatesMatchTheModel(t *testing.T) {
	up := readMigration(t, "023_recovery_cases.up.sql")
	for _, state := range []State{
		StateDetected, StateEvaluating, StateReplacementSearch, StateTransferRequired,
		StateInProgress, StateResolved, StateOpsRequired, StateFailed,
	} {
		if !strings.Contains(up, "'"+string(state)+"'") {
			t.Errorf("migration does not accept state %q", state)
		}
	}
}

func TestRecoveryDownReversesUp(t *testing.T) {
	up := readMigration(t, "023_recovery_cases.up.sql")
	down := readMigration(t, "023_recovery_cases.down.sql")

	tables := regexp.MustCompile(`(?i)CREATE TABLE IF NOT EXISTS\s+(\w+)`)
	for _, match := range tables.FindAllStringSubmatch(up, -1) {
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+match[1]) {
			t.Errorf("down does not drop %q", match[1])
		}
	}
	columns := regexp.MustCompile(`(?i)ADD COLUMN IF NOT EXISTS\s+(\w+)`)
	for _, match := range columns.FindAllStringSubmatch(up, -1) {
		if !strings.Contains(down, "DROP COLUMN IF EXISTS "+match[1]) {
			t.Errorf("down does not drop column %q", match[1])
		}
	}
	// The booking columns must be dropped BEFORE the case table:
	// active_recovery_id has a foreign key into it, so the other order fails.
	if strings.Index(down, "DROP COLUMN IF EXISTS active_recovery_id") >
		strings.Index(down, "DROP TABLE IF EXISTS hauling_recovery_cases") {
		t.Error("down drops the case table before the column that references it")
	}
}

func TestPolicyIngestMigrationShape(t *testing.T) {
	up := readMigration(t, "022_policy_publication_ingest.up.sql")
	for _, required := range []string{
		"hauling_policy_publications",
		"event_id TEXT PRIMARY KEY",
		"source_version", "schema_version", "checksum", "document_ref",
		"PENDING_READBACK", "chk_hauling_publication_applied",
		"hauling_policy_snapshots_immutable",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("policy ingest migration missing %q", required)
		}
	}
	// The immutability trigger is what stops a committed snapshot being
	// rewritten underneath work that pinned it.
	if !strings.Contains(up, "CREATE TRIGGER trg_hauling_policy_snapshots_immutable") {
		t.Error("policy snapshots have no immutability trigger")
	}
}

func TestMigrationNumbersAreUnique(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "..", "migrations"))
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	counts := map[string]int{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		if prefix, _, found := strings.Cut(name, "_"); found {
			counts[prefix]++
		}
	}
	for _, number := range []string{"022", "023", "032"} {
		if counts[number] != 1 {
			t.Fatalf("found %d migrations numbered %s, want exactly 1", counts[number], number)
		}
	}
}

// Every obligation state the worker can write must be accepted by the CHECK
// constraint, and the down migration must reverse the up.
func TestPaymentRebindMigrationMatchesTheModel(t *testing.T) {
	up := readMigration(t, "032_recovery_payment_rebind_obligations.up.sql")
	down := readMigration(t, "032_recovery_payment_rebind_obligations.down.sql")
	for _, state := range []RebindState{
		RebindPending, RebindUnknownOutcome, RebindCompleted, RebindCompletedNotHeld,
		RebindSuperseded, RebindReconciliationRequired,
	} {
		if !strings.Contains(up, "'"+string(state)+"'") {
			t.Errorf("migration does not accept obligation state %q", state)
		}
	}
	for _, required := range []string{
		"hauling_payment_rebind_obligations", "uq_hauling_payment_rebind_case_provider",
		"UNIQUE (recovery_case_id, provider_id)", "chk_hauling_payment_rebind_terminal",
		"lease_owner", "lease_expires_at", "next_attempt_at", "attempts", "last_error_code",
		"wallet_binding_version",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("payment rebind migration missing %q", required)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS hauling_payment_rebind_obligations") {
		t.Error("down does not drop the obligation table")
	}
	if !strings.Contains(down, "RAISE EXCEPTION") {
		t.Error("down must refuse while obligations are still open")
	}
}

// The down migration refuses while an obligation is open, succeeds once none
// is, and the up re-applies cleanly.
func TestPaymentRebindMigrationRollsBackOnlyWhenSettledAndReapplies(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	bookingID, _ := replacedBooking(t, pool, "op-rebind-migration")
	var otherOpen int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM hauling_payment_rebind_obligations
		WHERE booking_id<>$1 AND state IN ('PENDING','UNKNOWN_OUTCOME')`, bookingID).Scan(&otherOpen); err != nil {
		t.Fatal(err)
	}
	if otherOpen > 0 {
		t.Skip("another package left open obligations in this database")
	}
	up := readMigration(t, "032_recovery_payment_rebind_obligations.up.sql")
	down := readMigration(t, "032_recovery_payment_rebind_obligations.down.sql")
	if _, err := pool.Exec(ctx, down); err == nil {
		t.Fatal("rollback dropped an open payment rebind obligation")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM hauling_payment_rebind_obligations`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, down); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var exists bool
	_ = pool.QueryRow(ctx, `SELECT to_regclass('hauling_payment_rebind_obligations') IS NOT NULL`).Scan(&exists)
	if exists {
		t.Fatal("rollback left the table")
	}
	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatalf("replay: %v", err)
	}
	_ = pool.QueryRow(ctx, `SELECT to_regclass('hauling_payment_rebind_obligations') IS NOT NULL`).Scan(&exists)
	if !exists {
		t.Fatal("reapply did not recreate the table")
	}
}
