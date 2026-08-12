package integration_test

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	searchrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/repositories"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
)

// Section 29 WP21: "search-index rebuild" and "audit chain remains
// verifiable" — the two DR-replay/rebuild properties this session found no
// existing dedicated test for. Every other named property in WP21
// (outbox replay, duplicate-consumer-delivery idempotency, marketplace/
// provider/wallet/media projection reconciliation, stale job-lease and
// scheduler-lease recovery, scheduled-task duplicate-occurrence prevention)
// already has real, passing coverage elsewhere in this package — see this
// session's final evidence report for the exact test names cited as
// evidence rather than duplicated here.

// TestSearchProjection_RebuildsAfterLoss proves search's own documented
// architecture (see search/usecases/service.go's package doc: "a
// read-side projection... indexing is a best-effort, separate call a
// feature makes after its own commit succeeds") actually supports the DR
// property that architecture exists for: the projection table can be wiped
// entirely (simulating the search index being lost independently of the
// authoritative data it was derived from) and re-derived by replaying the
// same idempotent Index() call every feature already makes after its own
// commit — no new "rebuild" mechanism is invented here because
// Index()'s upsert-by-(org,type,reference) semantics already are one.
func TestSearchProjection_RebuildsAfterLoss(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Search Rebuild Test Co", "RC-SRCHRB-1")

	roles := staffrepos.NewRoleRepository(pool)
	authorizer := accesscontrol.NewAuthorizer(roles, nil, 0)
	repo := searchrepos.NewRepository(pool)
	service := searchusecases.NewService(searchusecases.Options{
		Pool: pool, Repository: repo, Authorizer: authorizer,
		Metrics: observability.NewMetrics(), Flags: searchusecases.Flags{Enabled: true},
	})

	input := searchusecases.IndexInput{
		SubjectType: "ORDER", SubjectReference: "ord_rebuild_probe", Title: "Rebuild probe order ORD-REBUILD",
		RequiredPermission: "search.read", SourceVersion: 1,
	}
	if _, err := service.Index(ctx, org.ID, input); err != nil {
		t.Fatalf("initial index: %v", err)
	}

	var before int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_search_documents WHERE organization_id=$1 AND subject_reference='ord_rebuild_probe'`, org.ID).Scan(&before); err != nil {
		t.Fatalf("count before loss: %v", err)
	}
	if before != 1 {
		t.Fatalf("expected exactly 1 indexed document before loss, got %d", before)
	}

	// Simulate the projection being lost independently of its source data
	// (a corrupted/dropped search index, distinct from a full database
	// disaster — the authoritative order this document represents is
	// untouched).
	if _, err := pool.Exec(ctx, `DELETE FROM fleet_partner_search_documents WHERE organization_id=$1`, org.ID); err != nil {
		t.Fatalf("simulate index loss: %v", err)
	}
	var lost int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_search_documents WHERE organization_id=$1`, org.ID).Scan(&lost); err != nil {
		t.Fatalf("count after loss: %v", err)
	}
	if lost != 0 {
		t.Fatalf("expected the index to be empty after simulated loss, got %d rows", lost)
	}

	// Rebuild: the owning feature replays the same Index() call it already
	// makes after every commit. A real rebuild sweep would do this for
	// every source-of-truth record; this proves the call itself is a safe,
	// idempotent rebuild primitive, not that a bulk-rebuild HTTP endpoint
	// exists (none does — see the final report's disclosed scope).
	if _, err := service.Index(ctx, org.ID, input); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	var after int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_search_documents WHERE organization_id=$1 AND subject_reference='ord_rebuild_probe'`, org.ID).Scan(&after); err != nil {
		t.Fatalf("count after rebuild: %v", err)
	}
	if after != 1 {
		t.Fatalf("expected exactly 1 document after rebuild, got %d", after)
	}
}

// TestAuditChain_RemainsVerifiable proves audit.VerifyChain (the Section 28
// hash-chain integrity check) still runs and reports a clean chain after
// ordinary write activity — the DR property this stands in for is "the
// audit chain remains verifiable" after any procedure that touches the
// audit_records table (backup/restore, replay). This test exercises the
// verification function directly against real, freshly-written chained
// records rather than reconstructing a full restore scenario a second time
// (see TestBackupAndRestore_RealDisposableDatabase for the restore-specific
// proof that the chain's own protective trigger survives restore).
func TestAuditChain_RemainsVerifiable(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Audit Chain Verify Co", "RC-AUDCHN-1")

	recorder := audit.NewRecorder()
	for i := 0; i < 3; i++ {
		if err := recorder.Record(ctx, pool, audit.Entry{
			OrganizationID: org.ID, ActorType: "system", ActorID: "audit-chain-test",
			Action: "test.audit_chain_probe", ResourceType: "probe", ResourceID: org.ID,
			NewValues: map[string]any{"i": i},
		}); err != nil {
			t.Fatalf("record audit entry %d: %v", i, err)
		}
	}

	// The chain partitions by organization + calendar month (see chain.go's
	// own chainPartition doc comment) — never construct this by hand in
	// production code, but a test verifying today's freshly written entries
	// has no other way to name the partition it just wrote into.
	partition := org.ID + ":" + time.Now().UTC().Format("2006-01")
	result, err := audit.VerifyChain(ctx, pool, partition)
	if err != nil {
		t.Fatalf("verify chain: %v", err)
	}
	if !result.Valid() {
		t.Fatalf("audit chain for partition %s reported breaks: %+v", partition, result.Breaks)
	}
}
