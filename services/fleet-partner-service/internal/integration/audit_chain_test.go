package integration_test

import (
	"context"
	"fmt"
	"time"

	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

func idFor(n int) string { return fmt.Sprintf("res_%d", n) }

// currentMonthPartition mirrors audit.chainPartition's exact format
// (unexported in that package, so tests in this different package
// reconstruct it) -- organization + calendar month.
func currentMonthPartition(organizationID string) string {
	return organizationID + ":" + time.Now().UTC().Format("2006-01")
}

// tamperRow simulates a modification that bypassed the append-only trigger:
// disables the trigger for one transaction (the same technique this
// package's own resetTestData cleanup helper already uses), changes the
// most recent row's action, then re-enables it.
func tamperRow(t *testing.T, pool *pgxpool.Pool, organizationID, newAction string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tamper tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatalf("disable trigger: %v", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE fleet_partner_audit_records SET action = $1
		WHERE id = (SELECT id FROM fleet_partner_audit_records
		            WHERE organization_id = $2 ORDER BY occurred_at DESC, id DESC LIMIT 1)`,
		newAction, organizationID)
	if err != nil {
		t.Fatalf("tamper update: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("tamper update affected %d rows, want 1", tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tamper tx: %v", err)
	}
}

// tamperPreviousHash corrupts the SECOND row's previous_entry_hash so it no
// longer matches the first row's real entry_hash -- simulating a deleted
// and differently-reinserted row, or an out-of-order restore.
func tamperPreviousHash(t *testing.T, pool *pgxpool.Pool, organizationID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tamper tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatalf("disable trigger: %v", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE fleet_partner_audit_records SET previous_entry_hash = 'deliberately-wrong-hash-for-testing'
		WHERE id = (SELECT id FROM fleet_partner_audit_records
		            WHERE organization_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1)`,
		organizationID)
	if err != nil {
		t.Fatalf("tamper update: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("tamper update affected %d rows, want 1", tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tamper tx: %v", err)
	}
}

// insertLegacyRow inserts a row exactly as a pre-migration-000069 row would
// look: chain fields all NULL, hash_version at its DEFAULT of 0.
func insertLegacyRow(t *testing.T, pool *pgxpool.Pool, organizationID string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_audit_records
			(id, organization_id, actor_type, actor_id, action, resource_type, resource_id,
			 request_id, correlation_id, occurred_at)
		VALUES ($1, $2, 'organization_user', 'ou_1', 'test.legacy_action', 'test_resource', 'res_legacy',
			'req_legacy', 'cor_legacy', now())`,
		ids.New(ids.PrefixAudit), organizationID)
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
}

// TestAuditChain_InitialEntryConcurrentAppendAndValidVerification proves the
// chain's happy path against real Postgres: a fresh partition's first entry
// chains against an empty previous hash, concurrent appends to the SAME
// partition (many goroutines racing database/sql.DB's own connection pool,
// serialized only by the per-partition advisory lock inside Record) never
// corrupt the chain, and a straightforward verification pass over the
// result reports zero breaks.
func TestAuditChain_InitialEntryConcurrentAppendAndValidVerification(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Audit Chain Org", "RC-CHAIN-1")
	recorder := audit.NewRecorder()

	const concurrency = 8
	errs := make(chan error, concurrency)
	for i := 0; i < concurrency; i++ {
		go func(n int) {
			errs <- recorder.Record(ctx, pool, audit.Entry{
				OrganizationID: org.ID, ActorType: "organization_user", ActorID: "ou_1",
				Action: "test.chain_append", ResourceType: "test_resource", ResourceID: idFor(n),
				RequestID: "req_x", CorrelationID: "cor_x",
			})
		}(i)
	}
	for i := 0; i < concurrency; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent Record: %v", err)
		}
	}

	partition := currentMonthPartition(org.ID)
	result, err := audit.VerifyChain(ctx, pool, partition)
	if err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
	if result.EntriesChecked != concurrency {
		t.Fatalf("entries checked = %d, want %d", result.EntriesChecked, concurrency)
	}
	if !result.Valid() {
		t.Fatalf("expected a valid chain, got breaks: %+v", result.Breaks)
	}
}

// TestAuditChain_TenantPartitionIsolation proves two organizations'
// entries, even created interleaved in time, land in separate partitions
// and verify independently -- one organization's chain can never be
// corrupted by, or need to account for, another's activity.
func TestAuditChain_TenantPartitionIsolation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Audit Chain Org A", "RC-CHAIN-A")
	orgB := createOrganization(t, pool, "Audit Chain Org B", "RC-CHAIN-B")
	recorder := audit.NewRecorder()

	for i := 0; i < 3; i++ {
		if err := recorder.Record(ctx, pool, audit.Entry{
			OrganizationID: orgA.ID, ActorType: "organization_user", ActorID: "ou_1",
			Action: "test.chain_append", ResourceType: "test_resource", ResourceID: idFor(i),
			RequestID: "req_a", CorrelationID: "cor_a",
		}); err != nil {
			t.Fatalf("record orgA: %v", err)
		}
		if err := recorder.Record(ctx, pool, audit.Entry{
			OrganizationID: orgB.ID, ActorType: "organization_user", ActorID: "ou_1",
			Action: "test.chain_append", ResourceType: "test_resource", ResourceID: idFor(i),
			RequestID: "req_b", CorrelationID: "cor_b",
		}); err != nil {
			t.Fatalf("record orgB: %v", err)
		}
	}

	resultA, err := audit.VerifyChain(ctx, pool, currentMonthPartition(orgA.ID))
	if err != nil || resultA.EntriesChecked != 3 || !resultA.Valid() {
		t.Fatalf("orgA chain: entries=%d valid=%v err=%v", resultA.EntriesChecked, resultA.Valid(), err)
	}
	resultB, err := audit.VerifyChain(ctx, pool, currentMonthPartition(orgB.ID))
	if err != nil || resultB.EntriesChecked != 3 || !resultB.Valid() {
		t.Fatalf("orgB chain: entries=%d valid=%v err=%v", resultB.EntriesChecked, resultB.Valid(), err)
	}
}

// TestAuditChain_DetectsModifiedRow and TestAuditChain_DetectsBrokenLink
// simulate the exact threat chaining exists for: a modification that
// bypasses the append-only trigger entirely (a database superuser running
// UPDATE directly, or a restored backup with an altered row). The trigger
// itself refuses ordinary UPDATE/DELETE (proven by
// TestAuditRecordsAreAppendOnly elsewhere in this package); these tests
// disable it for one transaction -- the same
// "SET LOCAL session_replication_role = replica" technique this test
// package's own resetTestData helper already uses for cleanup -- purely to
// construct the tampered state chaining must then detect.
func TestAuditChain_DetectsModifiedRow(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Audit Chain Tamper Org", "RC-CHAIN-TAMPER-1")
	recorder := audit.NewRecorder()

	if err := recorder.Record(ctx, pool, audit.Entry{
		OrganizationID: org.ID, ActorType: "organization_user", ActorID: "ou_1",
		Action: "test.chain_append", ResourceType: "test_resource", ResourceID: "res_1",
		RequestID: "req_x", CorrelationID: "cor_x",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	tamperRow(t, pool, org.ID, "test.chain_tampered_action")

	result, err := audit.VerifyChain(ctx, pool, currentMonthPartition(org.ID))
	if err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
	if result.Valid() {
		t.Fatal("expected VerifyChain to detect the tampered action field, got a valid chain")
	}
}

func TestAuditChain_DetectsBrokenLink(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Audit Chain Tamper Org 2", "RC-CHAIN-TAMPER-2")
	recorder := audit.NewRecorder()

	for i := 0; i < 2; i++ {
		if err := recorder.Record(ctx, pool, audit.Entry{
			OrganizationID: org.ID, ActorType: "organization_user", ActorID: "ou_1",
			Action: "test.chain_append", ResourceType: "test_resource", ResourceID: idFor(i),
			RequestID: "req_x", CorrelationID: "cor_x",
		}); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	tamperPreviousHash(t, pool, org.ID)

	result, err := audit.VerifyChain(ctx, pool, currentMonthPartition(org.ID))
	if err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
	if result.Valid() {
		t.Fatal("expected VerifyChain to detect the broken previous_entry_hash link, got a valid chain")
	}
}

// TestAuditChain_LegacyBoundaryRowsAreSkippedNotFlagged proves a
// pre-chaining row (hash_version = 0, the default for every row inserted
// before migration 000069 -- simulated here by inserting one directly with
// chain fields left NULL) is counted separately and never treated as a
// tamper break, even though it has no previous_entry_hash to link against.
func TestAuditChain_LegacyBoundaryRowsAreSkippedNotFlagged(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Audit Chain Legacy Org", "RC-CHAIN-LEGACY")
	recorder := audit.NewRecorder()

	insertLegacyRow(t, pool, org.ID)
	if err := recorder.Record(ctx, pool, audit.Entry{
		OrganizationID: org.ID, ActorType: "organization_user", ActorID: "ou_1",
		Action: "test.chain_append", ResourceType: "test_resource", ResourceID: "res_after_legacy",
		RequestID: "req_x", CorrelationID: "cor_x",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	result, err := audit.VerifyChain(ctx, pool, currentMonthPartition(org.ID))
	if err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
	if result.LegacyEntries != 1 {
		t.Fatalf("legacy entries = %d, want 1", result.LegacyEntries)
	}
	if result.EntriesChecked != 1 {
		t.Fatalf("entries checked = %d, want 1 (the real, chained row)", result.EntriesChecked)
	}
	if !result.Valid() {
		t.Fatalf("legacy row must not itself be reported as a break: %+v", result.Breaks)
	}
}
