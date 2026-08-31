package crm

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Preview
// ---------------------------------------------------------------------------

func TestCampaignPreview_CountsAreExactAndPIIFree(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	allowed := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_allow"))
	denied := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_deny"))
	silent := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_unknown"))
	env.allowMarketing(t, businessA, "vs_a", allowed.ID, ChannelEmail, 1)
	env.projectConsent(t, businessA, "vs_a", denied.ID, ChannelEmail, ConsentDeny, 1, nil)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", allowed.ID, denied.ID, silent.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Preview me", audience.ID))

	preview, err := env.service.PreviewCampaign(ctx, actor, globalScope(), campaign.ID, CampaignPreviewInput{})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.CountState != CountExact {
		t.Fatalf("expected an exact count, got %s", preview.CountState)
	}
	if preview.SourceCount != 3 || preview.TargetCount != 3 {
		t.Fatalf("expected 3 source and 3 targets, got %d/%d", preview.SourceCount, preview.TargetCount)
	}
	if len(preview.Channels) != 1 || preview.Channels[0].Channel != "EMAIL" {
		t.Fatalf("expected one EMAIL channel projection, got %+v", preview.Channels)
	}
	if preview.Channels[0].EligibleCount != 1 || preview.Channels[0].IneligibleCount != 2 {
		t.Fatalf("expected 1 eligible and 2 ineligible, got %d/%d",
			preview.Channels[0].EligibleCount, preview.Channels[0].IneligibleCount)
	}
	// Two of the three relationships have no eligible channel at all.
	if preview.ConsentSuppressedCount != 2 {
		t.Fatalf("expected 2 relationships with no eligible channel, got %d", preview.ConsentSuppressedCount)
	}
	if !preview.Channels[0].SendTimeRecheckRequired {
		t.Fatal("a preview must always report that consent is rechecked at send time")
	}

	// The sample carries a relationship reference and a reason, never a contact
	// detail — there is no field on it that could hold one.
	if len(preview.Sample) == 0 {
		t.Fatal("expected a bounded sample")
	}
	for _, sample := range preview.Sample {
		if sample.ProfileRef == "" || sample.ReasonCode == "" {
			t.Fatalf("sample row is incomplete: %+v", sample)
		}
	}

	// Preview must not persist anything.
	var snapshots, targets int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_campaign_audience_snapshots WHERE business_id=$1`,
		businessA).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_campaign_targets WHERE business_id=$1`,
		businessA).Scan(&targets); err != nil {
		t.Fatal(err)
	}
	if snapshots != 0 || targets != 0 {
		t.Fatalf("preview persisted state: snapshots=%d targets=%d", snapshots, targets)
	}
	if env.reloadCampaign(t, businessA, campaign.ID).Status != CampaignDraft {
		t.Fatal("preview changed the campaign lifecycle")
	}
}

func TestCampaignPreview_ReportsPolicyRechecksAndStaleAudience(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	audience := env.dynamicAudience(t, businessA, "vs_a", "Repeat buyers")
	definition := campaignInput("Policy heavy", audience.ID)
	cap, window := 2, 1440
	definition.FrequencyCapCount, definition.FrequencyCapWindowMinutes = &cap, &window
	yes := true
	definition.QuietHoursEnabled = &yes
	definition.QuietHoursStart, definition.QuietHoursEnd = "22:00", "07:00"
	definition.QuietHoursTimezone = "Africa/Lagos"
	campaign := env.createCampaign(t, businessA, "vs_a", definition)

	sendAt := mustTime(t, "2026-08-12T21:30:00Z")
	preview, err := env.service.PreviewCampaign(ctx, actor, globalScope(), campaign.ID,
		CampaignPreviewInput{HypotheticalSendAt: &sendAt})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	// The frequency cap is stored, and honestly reported as unevaluable: there
	// is no delivery history in this service, and an empty history would make
	// every cap pass.
	if !preview.Frequency.Enabled || preview.Frequency.EvaluationState != string(ProjectionNotAvailable) {
		t.Fatalf("frequency should be enabled but unevaluable: %+v", preview.Frequency)
	}
	if !preview.Frequency.RecheckRequired || !preview.QuietHours.RecheckRequired {
		t.Fatal("both policies must be marked for send-time recheck")
	}
	if !preview.QuietHours.CrossesMidnight {
		t.Fatal("22:00-07:00 crosses midnight")
	}
	if preview.QuietHours.WouldBlock == nil || !*preview.QuietHours.WouldBlock {
		t.Fatalf("22:30 Lagos time should fall inside quiet hours: %+v", preview.QuietHours)
	}
	// Asking without an instant answers nothing about a hypothetical send,
	// rather than guessing one.
	plain, err := env.service.PreviewCampaign(ctx, actor, globalScope(), campaign.ID, CampaignPreviewInput{})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if plain.QuietHours.WouldBlock != nil {
		t.Fatal("without a hypothetical instant there is no quiet-hours verdict to give")
	}

	// Make the dynamic source stale and prove the preview says so and counts
	// nobody, rather than quietly reusing the previous membership.
	if _, err = env.pool.Exec(ctx, `UPDATE crm_segments SET evaluation_status='STALE' WHERE business_id=$1 AND id=$2`,
		businessA, audience.ID); err != nil {
		t.Fatal(err)
	}
	stale, err := env.service.PreviewCampaign(ctx, actor, globalScope(), campaign.ID, CampaignPreviewInput{})
	if err != nil {
		t.Fatalf("preview a stale audience: %v", err)
	}
	if stale.SourceCount != 0 || stale.TargetCount != 0 {
		t.Fatalf("a stale audience must count nobody, got %d/%d", stale.SourceCount, stale.TargetCount)
	}
	if len(stale.Warnings) == 0 {
		t.Fatal("a stale audience must produce a warning")
	}
	if stale.SourceSegmentState != EvalStale {
		t.Fatalf("expected the segment state to travel with the preview, got %s", stale.SourceSegmentState)
	}
}

// TestCampaignPreview_BranchActorNeverSeesBusinessWideCounts is the disclosure
// test. A branch manager must not be able to learn the size of the
// business-wide customer base through a campaign they cannot see.
func TestCampaignPreview_BranchActorNeverSeesBusinessWideCounts(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	businessWide := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Business blast", businessWide.ID))

	if _, err := env.service.PreviewCampaign(ctx, actor, branchScope(branchA1), campaign.ID, CampaignPreviewInput{}); err == nil {
		t.Fatal("a branch actor previewed a business-wide campaign")
	} else if !isCRMCode(err, CodeCampaignNotFound) {
		// Not-found rather than forbidden: a forbidden would confirm the
		// campaign exists.
		t.Fatalf("expected a not-found response, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Preparation
// ---------------------------------------------------------------------------

func TestCampaignPrepare_FreezesAnImmutableSnapshot(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	allowed := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_allow"))
	denied := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_deny"))
	env.allowMarketing(t, businessA, "vs_a", allowed.ID, ChannelEmail, 1)
	env.projectConsent(t, businessA, "vs_a", denied.ID, ChannelEmail, ConsentDeny, 1, nil)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", allowed.ID, denied.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Freeze me", audience.ID))

	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	if snapshot.Status != SnapshotCurrent {
		t.Fatalf("expected a CURRENT snapshot, got %s (error=%v)", snapshot.Status, snapshot.ErrorCode)
	}
	assertFunnelReconciles(t, snapshot)
	if snapshot.SourceCount != 2 || snapshot.TargetCount != 2 {
		t.Fatalf("expected 2 source and 2 targets, got %d/%d", snapshot.SourceCount, snapshot.TargetCount)
	}
	if snapshot.ConsentSuppressedCount != 1 {
		t.Fatalf("the denied relationship should have no eligible channel, got %d", snapshot.ConsentSuppressedCount)
	}
	if snapshot.AudienceHash == nil || *snapshot.AudienceHash == "" {
		t.Fatal("a completed snapshot should carry an audience hash")
	}
	// The snapshot is bound to the exact definition it was built from.
	if snapshot.CampaignDefinitionVersion != 1 {
		t.Fatalf("expected definition version 1, got %d", snapshot.CampaignDefinitionVersion)
	}

	reloaded := env.reloadCampaign(t, businessA, campaign.ID)
	if reloaded.Status != CampaignPrepared {
		t.Fatalf("expected a PREPARED campaign, got %s", reloaded.Status)
	}
	if reloaded.CurrentSnapshotID == nil || *reloaded.CurrentSnapshotID != snapshot.ID {
		t.Fatalf("the campaign does not point at its new snapshot: %+v", reloaded.CurrentSnapshotID)
	}

	targets := env.targetsFor(t, businessA, snapshot.ID)
	if len(targets) != 2 {
		t.Fatalf("expected 2 target rows, got %d", len(targets))
	}
	allowedChannels := env.channelsFor(t, businessA, targets[allowed.ID].ID)
	if allowedChannels["EMAIL"].SnapshotEligibilityState != ChannelEligibleAtSnapshot {
		t.Fatalf("an ALLOW should be eligible at snapshot: %+v", allowedChannels["EMAIL"])
	}
	if !allowedChannels["EMAIL"].SendTimeRecheckRequired {
		t.Fatal("an eligible channel must still require a send-time recheck")
	}
	deniedChannels := env.channelsFor(t, businessA, targets[denied.ID].ID)
	if deniedChannels["EMAIL"].SnapshotEligibilityState != ChannelIneligibleConsent {
		t.Fatalf("a DENY should be ineligible: %+v", deniedChannels["EMAIL"])
	}
	if deniedChannels["EMAIL"].ReasonCode != ReasonConsentDenied {
		t.Fatalf("expected CONSENT_DENIED, got %s", deniedChannels["EMAIL"].ReasonCode)
	}
}

// TestCampaignPrepare_IsIdempotent proves a user pressing the button twice sees
// the same preparation rather than two.
func TestCampaignPrepare_IsIdempotent(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Twice", audience.ID))

	first, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID)
	if err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	second, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID)
	if err != nil {
		t.Fatalf("second prepare: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("a duplicate prepare created a second run: %s vs %s", first.ID, second.ID)
	}
	env.runPreparations(t)

	var snapshots int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_campaign_audience_snapshots
		WHERE business_id=$1 AND campaign_id=$2`, businessA, campaign.ID).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if snapshots != 1 {
		t.Fatalf("expected exactly one snapshot, got %d", snapshots)
	}
}

func TestCampaignPrepare_ExclusionsAndOverlapAreSubtracted(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	actor := actorFor(businessA, "vs_a")

	keep := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_keep"))
	drop := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_drop"))
	env.allowMarketing(t, businessA, "vs_a", keep.ID, ChannelEmail, 1)
	env.allowMarketing(t, businessA, "vs_a", drop.ID, ChannelEmail, 1)

	source := env.staticAudience(t, businessA, "vs_a", "Everyone", keep.ID, drop.ID)
	// The excluded relationship is deliberately in BOTH lists, so the test
	// pins the precedence: an exclusion wins over membership.
	exclusion := env.staticAudience(t, businessA, "vs_a", "Recently complained", drop.ID)

	definition := campaignInput("With exclusions", source.ID)
	definition.ExclusionSegmentRefs = []string{exclusion.ID}
	campaign := env.createCampaign(t, businessA, "vs_a", definition)

	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	assertFunnelReconciles(t, snapshot)
	if snapshot.SourceCount != 2 || snapshot.SegmentExclusionCount != 1 || snapshot.TargetCount != 1 {
		t.Fatalf("expected 2/1/1, got %d/%d/%d",
			snapshot.SourceCount, snapshot.SegmentExclusionCount, snapshot.TargetCount)
	}
	targets := env.targetsFor(t, businessA, snapshot.ID)
	if _, excluded := targets[drop.ID]; excluded {
		t.Fatal("an excluded relationship became a target")
	}
	if _, kept := targets[keep.ID]; !kept {
		t.Fatal("the included relationship is missing")
	}
	_ = actor
}

func TestCampaignPrepare_DynamicExclusionUsesOnlyCurrentGeneration(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_current_exclusion_only"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_exclusion_old_1", branchA1, 5000, 5000, env.now.Add(-48*time.Hour), 1)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_exclusion_old_2", branchA1, 5000, 5000, env.now.Add(-24*time.Hour), 1)
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)

	source := env.staticAudience(t, businessA, "vs_a", "Current exclusion source", profile.ID)
	exclusion := env.dynamicAudience(t, businessA, "vs_a", "Current exclusion dynamic")
	if exclusion.CurrentEvaluationID == nil {
		t.Fatal("dynamic exclusion has no current evaluation")
	}
	var oldGeneration int64
	if err := env.pool.QueryRow(ctx, `SELECT generation FROM crm_segment_evaluations WHERE id=$1`, *exclusion.CurrentEvaluationID).Scan(&oldGeneration); err != nil {
		t.Fatal(err)
	}
	var historicalMembership int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2 AND profile_id=$3 AND generation=$4`,
		businessA, exclusion.ID, profile.ID, oldGeneration).Scan(&historicalMembership); err != nil || historicalMembership != 1 {
		t.Fatalf("historical exclusion membership=%d err=%v", historicalMembership, err)
	}

	// Publish a new exact CURRENT generation with no membership for this
	// profile, retaining generation N to prove it is not consulted.
	newEvaluation := newID("crmse_")
	if _, err := env.pool.Exec(ctx, `INSERT INTO crm_segment_evaluations
		(id,business_id,segment_id,segment_definition_version,definition_hash,generation,status,evaluation_as_of,completed_at,member_count)
		VALUES($1,$2,$3,$4,'current-exclusion-test',$5,'SUCCESS',$6,$6,0)`,
		newEvaluation, businessA, exclusion.ID, exclusion.CurrentDefinitionVersion, oldGeneration+1, env.now); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `UPDATE crm_segments SET current_evaluation_id=$1,evaluation_status='CURRENT'
		WHERE business_id=$2 AND id=$3`, newEvaluation, businessA, exclusion.ID); err != nil {
		t.Fatal(err)
	}

	definition := campaignInput("Current dynamic exclusion", source.ID)
	definition.ExclusionSegmentRefs = []string{exclusion.ID}
	campaign := env.createCampaign(t, businessA, "vs_a", definition)
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	if snapshot.SourceCount != 1 || snapshot.SegmentExclusionCount != 0 || snapshot.TargetCount != 1 {
		t.Fatalf("source/excluded/target=%d/%d/%d; historical generation incorrectly excluded the profile",
			snapshot.SourceCount, snapshot.SegmentExclusionCount, snapshot.TargetCount)
	}
}

// TestCampaignPrepare_BindsToTheExactSegmentGeneration proves the audience is
// pinned rather than re-read, which is what stops two segment rebuilds being
// mixed into one snapshot.
func TestCampaignPrepare_BindsToTheExactSegmentGeneration(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_1", branchA1, 5000, 5000, env.now.Add(-24*3600e9), 1)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_2", branchA1, 5000, 5000, env.now.Add(-12*3600e9), 1)
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)

	audience := env.dynamicAudience(t, businessA, "vs_a", "Repeat buyers")
	if audience.EvaluationStatus != EvalCurrent {
		t.Fatalf("expected a CURRENT audience, got %s", audience.EvaluationStatus)
	}
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Dynamic source", audience.ID))

	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	if snapshot.Status != SnapshotCurrent {
		t.Fatalf("expected CURRENT, got %s (error=%v)", snapshot.Status, snapshot.ErrorCode)
	}
	if snapshot.SourceSegmentGeneration == nil {
		t.Fatal("a dynamic source must pin an exact generation")
	}
	if snapshot.SourceSegmentEvaluationID == nil {
		t.Fatal("a dynamic source must record the evaluation it came from")
	}
	var generation int64
	if err := env.pool.QueryRow(ctx, `SELECT generation FROM crm_segment_evaluations WHERE business_id=$1 AND id=$2`,
		businessA, *snapshot.SourceSegmentEvaluationID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if generation != *snapshot.SourceSegmentGeneration {
		t.Fatalf("pinned generation %d does not match its evaluation %d", *snapshot.SourceSegmentGeneration, generation)
	}
	_ = actor
}

// TestCampaignPrepare_RefusesAStaleOrFailedDynamicAudience is the honesty rule.
// A segment that admits its membership is out of date must not be silently
// frozen into a snapshot that looks authoritative.
func TestCampaignPrepare_RefusesAStaleOrFailedDynamicAudience(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	audience := env.dynamicAudience(t, businessA, "vs_a", "Repeat buyers")
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Stale source", audience.ID))

	for _, state := range []string{EvalStale, EvalFailed, EvalQueued, EvalNotEvaluated} {
		if _, err := env.pool.Exec(ctx, `UPDATE crm_segments SET evaluation_status=$1 WHERE business_id=$2 AND id=$3`,
			state, businessA, audience.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID); err == nil {
			t.Fatalf("preparing from a %s audience should be refused", state)
		} else if !isCRMCode(err, CodeCampaignAudienceStale) {
			t.Fatalf("expected an audience-not-current rejection for %s, got %v", state, err)
		}
	}
	if env.reloadCampaign(t, businessA, campaign.ID).Status != CampaignDraft {
		t.Fatal("a refused preparation must not move the campaign lifecycle")
	}
}

// TestCampaignPrepare_LastGoodSnapshotSurvivesAFailure is the safety property
// Section 8B established and this section inherits: a failed rebuild must never
// empty a working audience.
func TestCampaignPrepare_LastGoodSnapshotSurvivesAFailure(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_1", branchA1, 5000, 5000, env.now.Add(-24*3600e9), 1)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_2", branchA1, 5000, 5000, env.now.Add(-12*3600e9), 1)
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	audience := env.dynamicAudience(t, businessA, "vs_a", "Repeat buyers")
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Survivor", audience.ID))

	good := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	if good.Status != SnapshotCurrent || good.TargetCount != 1 {
		t.Fatalf("expected a good first snapshot: %s target=%d", good.Status, good.TargetCount)
	}

	// Start a second preparation, then take its pinned generation away — the
	// exact situation where Section 8B's cleanup won the race. Retrying can
	// never fix it, so the worker must give up rather than back off forever,
	// and it must do so without disturbing the audience that already works.
	second, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID)
	if err != nil {
		t.Fatalf("second prepare: %v", err)
	}
	if second.SourceSegmentGeneration == nil {
		t.Fatal("a dynamic source should have pinned a generation")
	}
	if _, err = env.pool.Exec(ctx, `UPDATE crm_segment_evaluations SET status='SUPERSEDED'
		WHERE business_id=$1 AND segment_id=$2 AND generation=$3`,
		businessA, audience.ID, *second.SourceSegmentGeneration); err != nil {
		t.Fatal(err)
	}
	env.runPreparations(t)

	failed := env.reloadSnapshot(t, businessA, second.ID)
	if failed.Status != SnapshotFailed {
		t.Fatalf("a preparation over a released generation should fail, got %s", failed.Status)
	}
	if !failed.DeadLettered {
		t.Fatal("a failure retrying cannot fix should be dead-lettered immediately, not retried forever")
	}
	if failed.ErrorCode == nil || *failed.ErrorCode != CodeCampaignAudienceStale {
		t.Fatalf("expected an audience-not-current error code, got %v", failed.ErrorCode)
	}
	if len(env.targetsFor(t, businessA, second.ID)) != 0 {
		t.Fatal("a failed preparation left target rows behind")
	}

	// The previous audience is untouched, and the campaign still points at it.
	stillGood := env.reloadSnapshot(t, businessA, good.ID)
	if stillGood.Status != SnapshotCurrent || stillGood.TargetCount != 1 {
		t.Fatalf("the last good snapshot was disturbed: %s target=%d", stillGood.Status, stillGood.TargetCount)
	}
	if len(env.targetsFor(t, businessA, good.ID)) != 1 {
		t.Fatal("the last good audience lost its members")
	}
	reloaded := env.reloadCampaign(t, businessA, campaign.ID)
	if reloaded.Status != CampaignPreparationFailed {
		t.Fatalf("expected PREPARATION_FAILED, got %s", reloaded.Status)
	}
	if reloaded.CurrentSnapshotID == nil || *reloaded.CurrentSnapshotID != good.ID {
		t.Fatal("a failed preparation moved the campaign off its last good snapshot")
	}

	// And the operator hook can put it back in the queue.
	if err = env.service.ReplayCampaignPreparation(ctx, businessA, second.ID); err != nil {
		t.Fatalf("replay a dead-lettered preparation: %v", err)
	}
	if replayed := env.reloadSnapshot(t, businessA, second.ID); replayed.Status != SnapshotBuilding || replayed.DeadLettered {
		t.Fatalf("replay should return the run to the queue: %s dead=%t", replayed.Status, replayed.DeadLettered)
	}
}

// TestCampaignPrepare_SupersedesThePreviousSnapshotAtomically proves the
// hand-over: the new audience becomes current and the old one is retired in the
// same transaction, so there is never a moment with two current snapshots or
// none.
func TestCampaignPrepare_SupersedesThePreviousSnapshotAtomically(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	first := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", first.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", first.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Rolling", audience.ID))

	original := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	second := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_2"))
	env.allowMarketing(t, businessA, "vs_a", second.ID, ChannelEmail, 1)
	if _, err := env.service.BulkStaticMembership(ctx, actorFor(businessA, "vs_a"), globalScope(),
		audience.ID, []string{second.ID}, true); err != nil {
		t.Fatalf("grow the audience: %v", err)
	}
	replacement := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	if replacement.TargetCount != 2 {
		t.Fatalf("the replacement audience should hold 2, got %d", replacement.TargetCount)
	}
	if retired := env.reloadSnapshot(t, businessA, original.ID); retired.Status != SnapshotSuperseded {
		t.Fatalf("the previous snapshot should be SUPERSEDED, got %s", retired.Status)
	}
	var current int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_campaign_audience_snapshots
		WHERE business_id=$1 AND campaign_id=$2 AND status='CURRENT'`, businessA, campaign.ID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != 1 {
		t.Fatalf("expected exactly one current snapshot, got %d", current)
	}
	// The superseded snapshot keeps its membership: it is the evidence of what
	// the earlier campaign audience actually was.
	if len(env.targetsFor(t, businessA, original.ID)) != 1 {
		t.Fatal("a superseded snapshot lost its historical membership")
	}
}

// TestCampaignPrepare_EditDuringPreparationDoesNotMixDefinitions proves version
// binding: a campaign edited while version N is preparing finishes against
// version N, and the campaign's own definition moves on independently.
func TestCampaignPrepare_EditDuringPreparationDoesNotMixDefinitions(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelPush, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Edited mid-flight", audience.ID, "EMAIL"))

	snapshot, err := env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	// The edit lands while the preparation is queued.
	reloaded := env.reloadCampaign(t, businessA, campaign.ID)
	updated, _, err := env.service.UpdateCampaign(ctx, actor, globalScope(), campaign.ID, UpdateCampaignInput{
		Definition: campaignInput("Edited mid-flight", audience.ID, "EMAIL", "PUSH"), ExpectedVersion: reloaded.Version})
	if err != nil {
		t.Fatalf("edit during preparation: %v", err)
	}
	if updated.CurrentDefinitionVersion != 2 {
		t.Fatalf("the edit should have minted version 2, got %d", updated.CurrentDefinitionVersion)
	}

	env.runPreparations(t)
	settled := env.reloadSnapshot(t, businessA, snapshot.ID)
	if settled.CampaignDefinitionVersion != 1 {
		t.Fatalf("the snapshot drifted onto the new definition: version %d", settled.CampaignDefinitionVersion)
	}
	if settled.Status != SnapshotCurrent {
		t.Fatalf("expected the pinned run to complete, got %s (error=%v)", settled.Status, settled.ErrorCode)
	}
	// It was built for the single-channel definition, so exactly one channel
	// row exists per target.
	targets := env.targetsFor(t, businessA, settled.ID)
	channels := env.channelsFor(t, businessA, targets[profile.ID].ID)
	if len(channels) != 1 {
		t.Fatalf("expected one channel from definition version 1, got %d", len(channels))
	}
	if _, present := channels["EMAIL"]; !present {
		t.Fatalf("expected the EMAIL channel, got %+v", channels)
	}
}
