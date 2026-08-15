package crm

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// These are real-PostgreSQL concurrency tests. The ordering in each is forced
// by database row locks and transaction boundaries, never by a sleep: a race
// test that passes because one goroutine happened to be slower proves nothing
// about production, where the timing will be different.

// waitForBlockedTransaction polls until some backend is waiting on a lock. It
// is a condition, not a delay: the test proceeds the instant the other
// transaction is genuinely blocked, and fails rather than continuing blind if
// that never happens.
func waitForBlockedTransaction(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := pool.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE wait_event_type='Lock' AND state='active')`).
			Scan(&blocked); err != nil {
			t.Fatalf("check for a blocked transaction: %v", err)
		}
		if blocked {
			return
		}
	}
	t.Fatal("no transaction ever blocked on a lock; the fence is not doing what this test assumes")
}

// runPreparationWithHook drives one claimed preparation through the same steps
// the worker does, but pauses at the certification boundary so a test can make
// something else happen at exactly that point.
func (e *testEnv) runPreparationWithHook(t *testing.T, snapshotID string, afterCertification func()) {
	t.Helper()
	ctx := context.Background()

	snapshot, claimed, err := e.service.claimCampaignPreparation(ctx, "test-hook-worker")
	if err != nil || !claimed {
		t.Fatalf("claim preparation: claimed=%t err=%v", claimed, err)
	}
	if snapshot.ID != snapshotID {
		t.Fatalf("claimed the wrong preparation: %s want %s", snapshot.ID, snapshotID)
	}
	version, err := e.service.CampaignVersionByID(ctx, e.pool, snapshot.BusinessID, snapshot.CampaignVersionID)
	if err != nil {
		t.Fatalf("load campaign version: %v", err)
	}
	generation := int64(0)
	if snapshot.SourceSegmentGeneration != nil {
		generation = *snapshot.SourceSegmentGeneration
	}
	query := e.service.buildAudienceQuery(snapshot.BusinessID, version, generation, Scope{Global: true},
		e.service.campaignEval.BatchSize)
	counts, err := e.service.writeSnapshotAudience(ctx, snapshot, version, query,
		newExperimentAllocator(version))
	if err != nil {
		t.Fatalf("write audience: %v", err)
	}
	actor := Actor{VendorID: snapshot.BusinessID, CorrelationID: "test-hook-worker"}
	if err = e.service.promoteSnapshot(ctx, actor, snapshot, version, counts, afterCertification); err != nil {
		t.Fatalf("promote snapshot: %v", err)
	}
}

// TestCampaignConcurrency_DuplicatePrepareCreatesOneRun drives several prepare
// requests at once and proves the database — not a process-local guard — is
// what stops a second audience being built.
func TestCampaignConcurrency_DuplicatePrepareCreatesOneRun(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Hammered", audience.ID))

	const callers = 8
	var wait sync.WaitGroup
	results := make([]string, callers)
	failures := make([]error, callers)
	wait.Add(callers)
	for index := 0; index < callers; index++ {
		go func(slot int) {
			defer wait.Done()
			snapshot, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID)
			if err != nil {
				failures[slot] = err
				return
			}
			results[slot] = snapshot.ID
		}(index)
	}
	wait.Wait()

	// Every caller either sees the one run or is told a preparation is already
	// in progress. Nobody starts a second one, and nobody gets an unexplained
	// error.
	winners := map[string]bool{}
	for index := 0; index < callers; index++ {
		if failures[index] != nil {
			if !isCRMCode(failures[index], CodeCampaignPreparing) {
				t.Fatalf("caller %d failed unexpectedly: %v", index, failures[index])
			}
			continue
		}
		winners[results[index]] = true
	}
	if len(winners) != 1 {
		t.Fatalf("expected all callers to converge on one preparation, got %d distinct runs", len(winners))
	}
	var snapshots int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_campaign_audience_snapshots
		WHERE business_id=$1 AND campaign_id=$2`, businessA, campaign.ID).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if snapshots != 1 {
		t.Fatalf("expected exactly one snapshot row, got %d", snapshots)
	}
}

// TestCampaignConcurrency_RedactionCommittingFirstIsSeenByCertification is the
// first of the two orderings. The privacy transaction takes the profile lock
// before the promotion can certify, so the promotion must see the redaction and
// remove the target before the snapshot ever becomes current.
func TestCampaignConcurrency_RedactionCommittingFirstIsSeenByCertification(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	keep := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_keep"))
	erased := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_erased"))
	env.allowMarketing(t, businessA, "vs_a", keep.ID, ChannelEmail, 1)
	env.allowMarketing(t, businessA, "vs_a", erased.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", keep.ID, erased.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Race", audience.ID))

	snapshot, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	// The privacy transaction opens and takes the profile lock first.
	privacyTx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := env.reload(t, businessA, erased.ID)
	if err = env.service.ApplyPrivacy(ctx, privacyTx, actor, &reloaded, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_erased",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		_ = privacyTx.Rollback(ctx)
		t.Fatalf("apply privacy: %v", err)
	}

	// The preparation now runs and must block on the fence rather than
	// certifying an audience the redaction is about to invalidate.
	done := make(chan struct{})
	go func() {
		defer close(done)
		env.runPreparations(t)
	}()
	waitForBlockedTransaction(t, env.pool)

	if err = privacyTx.Commit(ctx); err != nil {
		t.Fatalf("commit privacy: %v", err)
	}
	<-done

	settled := env.reloadSnapshot(t, businessA, snapshot.ID)
	if settled.Status != SnapshotCurrent {
		t.Fatalf("expected the preparation to complete, got %s (error=%v)", settled.Status, settled.ErrorCode)
	}
	assertFunnelReconciles(t, settled)
	targets := env.targetsFor(t, businessA, snapshot.ID)
	if _, present := targets[erased.ID]; present {
		t.Fatal("a relationship redacted during preparation survived certification")
	}
	if _, present := targets[keep.ID]; !present {
		t.Fatal("the unaffected relationship was dropped")
	}
	if settled.TargetCount != 1 {
		t.Fatalf("expected the counts to be recomputed from the surviving rows, got %d", settled.TargetCount)
	}
}

// TestCampaignConcurrency_RedactionCommittingSecondRevokesImmediately is the
// other ordering. The promotion wins the lock, so the snapshot legitimately
// contains the relationship — and the redaction, running immediately
// afterwards, revokes its actionability rather than leaving it contactable.
func TestCampaignConcurrency_RedactionCommittingSecondRevokesImmediately(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Race two", audience.ID))

	snapshot, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	redacted := make(chan error, 1)
	env.runPreparationWithHook(t, snapshot.ID, func() {
		// Inside the promotion transaction, after the fence. The redaction
		// starts here and cannot commit until the promotion does.
		go func() {
			reloaded := env.reload(t, businessA, profile.ID)
			redacted <- env.applyPrivacy(t, businessA, "vs_a", &reloaded, PrivacyInput{
				SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_1",
				Action: PrivacyActionRedact, ObservedAt: env.now,
			})
		}()
		waitForBlockedTransaction(t, env.pool)
	})
	if err = <-redacted; err != nil {
		t.Fatalf("redaction after promotion: %v", err)
	}

	settled := env.reloadSnapshot(t, businessA, snapshot.ID)
	if settled.Status != SnapshotCurrent {
		t.Fatalf("expected the promotion to win, got %s", settled.Status)
	}
	target := env.targetsFor(t, businessA, snapshot.ID)[profile.ID]
	if target.CurrentActionable {
		t.Fatal("a relationship redacted moments after promotion is still actionable")
	}
	if target.SuppressionReason == nil || *target.SuppressionReason != SuppressionRedacted {
		t.Fatalf("expected a redaction suppression reason, got %v", target.SuppressionReason)
	}
}

// TestCampaignConcurrency_MergeDuringPreparationLeavesOneActionableRow proves
// the same fence covers identity merges, which are the other way a relationship
// can stop being independently targetable mid-run.
func TestCampaignConcurrency_MergeDuringPreparationLeavesOneActionableRow(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	guest := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_1"))
	customer := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", guest.ID, ChannelEmail, 1)
	env.allowMarketing(t, businessA, "vs_a", customer.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", guest.ID, customer.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Merge race", audience.ID))

	snapshot, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	merged := make(chan error, 1)
	env.runPreparationWithHook(t, snapshot.ID, func() {
		go func() {
			merged <- env.applyLink(t, businessA, "vs_a",
				linkInput("evt_merge_race", guestSubject("gst_1"), customerSubject("cus_1"), 4))
		}()
		waitForBlockedTransaction(t, env.pool)
	})
	if err = <-merged; err != nil {
		t.Fatalf("merge after promotion: %v", err)
	}

	targets := env.targetsFor(t, businessA, snapshot.ID)
	actionable := 0
	for _, target := range targets {
		if target.CurrentActionable {
			actionable++
		}
	}
	if actionable != 1 {
		t.Fatalf("a merge during preparation left %d actionable rows for one identity", actionable)
	}
	if targets[guest.ID].CurrentActionable {
		t.Fatal("the merged-away relationship is still actionable")
	}
}

// TestCampaignConcurrency_ServiceRecoveryDuringPreparationSuppresses covers the
// third mid-run state change. The complaint is filed before certification, so
// the re-sweep must catch it.
func TestCampaignConcurrency_ServiceRecoveryDuringPreparationSuppresses(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	keep := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_keep"))
	complaining := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_complaint"))
	env.allowMarketing(t, businessA, "vs_a", keep.ID, ChannelEmail, 1)
	env.allowMarketing(t, businessA, "vs_a", complaining.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", keep.ID, complaining.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Complaint race", audience.ID))

	snapshot, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	// The complaint is filed after the preparation was queued but before the
	// worker runs, so the audience is written and then re-swept.
	env.openServiceRecovery(t, businessA, "vs_a", complaining.ID)
	env.runPreparations(t)

	settled := env.reloadSnapshot(t, businessA, snapshot.ID)
	assertFunnelReconciles(t, settled)
	targets := env.targetsFor(t, businessA, snapshot.ID)
	if _, present := targets[complaining.ID]; present {
		t.Fatal("a customer who entered service recovery mid-run became a target")
	}
	if settled.ServiceRecoverySuppressedCount != 1 {
		t.Fatalf("expected one service-recovery suppression, got %d", settled.ServiceRecoverySuppressedCount)
	}
}

// TestCampaignConcurrency_SegmentRebuildDoesNotStealAPinnedGeneration is the
// retention test. Section 8B releases a superseded generation's membership once
// nothing references it; a campaign preparing from that generation is exactly
// the reference that must stop it.
func TestCampaignConcurrency_SegmentRebuildDoesNotStealAPinnedGeneration(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_1", branchA1, 5000, 5000, env.now.AddDate(0, 0, -2), 1)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_2", branchA1, 5000, 5000, env.now.AddDate(0, 0, -1), 1)
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)

	audience := env.dynamicAudience(t, businessA, "vs_a", "Repeat buyers")
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Pinned", audience.ID))

	snapshot, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if snapshot.SourceSegmentGeneration == nil {
		t.Fatal("expected a pinned generation")
	}
	pinned := *snapshot.SourceSegmentGeneration

	// Rebuild the segment while the preparation is queued. Without the
	// retention port this is exactly where the pinned membership would be
	// deleted underneath the campaign.
	if _, err = env.service.RequestMaterialization(ctx, actor, globalScope(), audience.ID); err != nil {
		t.Fatalf("request a rebuild: %v", err)
	}
	env.materialize(t)

	var pinnedRows int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2 AND generation=$3`, businessA, audience.ID, pinned).Scan(&pinnedRows); err != nil {
		t.Fatal(err)
	}
	if pinnedRows == 0 {
		t.Fatal("the segment rebuild deleted the membership a queued campaign preparation was pinned to")
	}

	env.runPreparations(t)
	settled := env.reloadSnapshot(t, businessA, snapshot.ID)
	if settled.Status != SnapshotCurrent {
		t.Fatalf("expected the pinned preparation to complete, got %s (error=%v)", settled.Status, settled.ErrorCode)
	}
	if *settled.SourceSegmentGeneration != pinned {
		t.Fatalf("the snapshot drifted onto another generation: %d want %d", *settled.SourceSegmentGeneration, pinned)
	}
	if settled.TargetCount != 1 {
		t.Fatalf("expected the pinned audience to survive intact, got %d targets", settled.TargetCount)
	}

	// Once the snapshot is no longer building or current, the generation is
	// releasable again — the pin is a lease, not a leak.
	if _, err = env.pool.Exec(ctx, `UPDATE crm_campaign_audience_snapshots SET status='SUPERSEDED'
		WHERE business_id=$1 AND id=$2`, businessA, snapshot.ID); err != nil {
		t.Fatal(err)
	}
	retained, err := (campaignGenerationRetainer{service: env.service}).Retained(ctx, businessA, audience.ID, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if retained {
		t.Fatal("a superseded snapshot is still pinning a segment generation")
	}
}
