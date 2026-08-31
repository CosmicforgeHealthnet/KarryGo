package crm

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Section 8E test helpers. They deliberately drive the real service methods
// rather than inserting rows directly, so a test that says "prepare this
// campaign" exercises the same path production does.

// prepareAndRun enqueues a preparation and drives the worker to completion,
// standing in for the ticker.
func (e *testEnv) prepareAndRun(t *testing.T, business, staff, campaignID string) CampaignSnapshot {
	t.Helper()
	ctx := context.Background()
	snapshot, err := e.service.PrepareCampaign(ctx, actorFor(business, staff), globalScope(), campaignID)
	if err != nil {
		t.Fatalf("prepare campaign: %v", err)
	}
	e.runPreparations(t)
	return e.reloadSnapshot(t, business, snapshot.ID)
}

// runPreparations drives the campaign preparation worker to quiescence.
func (e *testEnv) runPreparations(t *testing.T) {
	t.Helper()
	for pass := 0; pass < 10; pass++ {
		processed, err := e.service.RunCampaignPreparationPass(context.Background(), "test-campaign-worker", 10)
		if err != nil {
			t.Fatalf("campaign preparation pass: %v", err)
		}
		if processed == 0 {
			return
		}
	}
	t.Fatal("campaign preparation did not settle")
}

func (e *testEnv) reloadSnapshot(t *testing.T, business, snapshotID string) CampaignSnapshot {
	t.Helper()
	snapshot, err := scanSnapshot(e.pool.QueryRow(context.Background(),
		`SELECT `+snapshotCols+` FROM crm_campaign_audience_snapshots WHERE business_id=$1 AND id=$2`,
		business, snapshotID))
	if err != nil {
		t.Fatalf("reload snapshot: %v", err)
	}
	return snapshot
}

func (e *testEnv) reloadCampaign(t *testing.T, business, campaignID string) Campaign {
	t.Helper()
	campaign, err := e.service.loadCampaign(context.Background(), e.pool, business, campaignID)
	if err != nil {
		t.Fatalf("reload campaign: %v", err)
	}
	return campaign
}

// staticAudience creates an ACTIVE static segment holding exactly the given
// relationships, which is the simplest possible campaign source.
func (e *testEnv) staticAudience(t *testing.T, business, staff, name string, profileIDs ...string) Segment {
	t.Helper()
	ctx := context.Background()
	actor := actorFor(business, staff)
	segment, err := e.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: name, SegmentType: SegmentStatic, ScopeType: ScopeBusiness,
	})
	if err != nil {
		t.Fatalf("create static audience %s: %v", name, err)
	}
	// Membership is added in bulk-API-sized chunks, because the accepted
	// Section 8B ceiling is a real limit a test fixture has to respect rather
	// than route around.
	for start := 0; start < len(profileIDs); start += MaxBulkBatch {
		end := start + MaxBulkBatch
		if end > len(profileIDs) {
			end = len(profileIDs)
		}
		if _, err = e.service.BulkStaticMembership(ctx, actor, globalScope(), segment.ID, profileIDs[start:end], true); err != nil {
			t.Fatalf("seed static audience %s: %v", name, err)
		}
	}
	activated, err := e.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, 0)
	if err != nil {
		t.Fatalf("activate static audience %s: %v", name, err)
	}
	return activated
}

// branchAudience is the branch-scoped equivalent.
func (e *testEnv) branchAudience(t *testing.T, business, staff, name, branchID string, profileIDs ...string) Segment {
	t.Helper()
	ctx := context.Background()
	actor := actorFor(business, staff)
	segment, err := e.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: name, SegmentType: SegmentStatic, ScopeType: ScopeBranch, BranchRef: branchID,
	})
	if err != nil {
		t.Fatalf("create branch audience %s: %v", name, err)
	}
	if len(profileIDs) > 0 {
		if _, err = e.service.BulkStaticMembership(ctx, actor, globalScope(), segment.ID, profileIDs, true); err != nil {
			t.Fatalf("seed branch audience %s: %v", name, err)
		}
	}
	activated, err := e.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, 0)
	if err != nil {
		t.Fatalf("activate branch audience %s: %v", name, err)
	}
	return activated
}

// dynamicAudience creates an ACTIVE dynamic segment and materializes it, so a
// campaign has a CURRENT generation to bind to.
func (e *testEnv) dynamicAudience(t *testing.T, business, staff, name string) Segment {
	t.Helper()
	ctx := context.Background()
	actor := actorFor(business, staff)
	segment, err := e.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: name, SegmentType: SegmentDynamic, ScopeType: ScopeBusiness, Rule: repeatOrdersRule(),
	})
	if err != nil {
		t.Fatalf("create dynamic audience %s: %v", name, err)
	}
	if _, err = e.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, 0); err != nil {
		t.Fatalf("activate dynamic audience %s: %v", name, err)
	}
	e.materialize(t)
	return e.reloadSegment(t, business, segment.ID)
}

// campaignInput is the smallest valid campaign definition over one segment.
func campaignInput(name, segmentID string, channels ...string) CampaignDefinitionInput {
	if len(channels) == 0 {
		channels = []string{string(ChannelEmail)}
	}
	return CampaignDefinitionInput{
		Name: name, Objective: ObjectiveGeneralMarketing, ScopeType: ScopeBusiness,
		Channels: channels, SourceSegmentRef: segmentID,
	}
}

// createCampaign is the happy-path definition helper.
func (e *testEnv) createCampaign(t *testing.T, business, staff string, in CampaignDefinitionInput) Campaign {
	t.Helper()
	campaign, _, err := e.service.CreateCampaign(context.Background(), actorFor(business, staff), globalScope(), in)
	if err != nil {
		t.Fatalf("create campaign %s: %v", in.Name, err)
	}
	return campaign
}

// allowMarketing projects an authoritative ALLOW for one channel, which is the
// only way a target can become eligible.
func (e *testEnv) allowMarketing(t *testing.T, business, staff, profileID string, channel ConsentChannel, version int64) {
	t.Helper()
	e.projectConsent(t, business, staff, profileID, channel, ConsentAllow, version, nil)
}

func (e *testEnv) projectConsent(t *testing.T, business, staff, profileID string, channel ConsentChannel,
	decision ConsentDecision, version int64, expiresAt *time.Time) {
	t.Helper()
	if err := e.tx(t, func(tx pgx.Tx) error {
		return e.service.ApplyConsent(context.Background(), tx, actorFor(business, staff), profileID, ConsentInput{
			SourceService: "customer-service", SourceVersion: version, Purpose: ConsentPurposeMarketing,
			Channel: channel, Decision: decision, ObservedAt: e.now, ExpiresAt: expiresAt,
		})
	}); err != nil {
		t.Fatalf("project consent %s/%s: %v", profileID, channel, err)
	}
}

// openServiceRecovery records the exact Vendor-local signal Section 8E treats
// as service recovery: an open task linked to a service-follow-up interaction.
// Anything less specific would not suppress, which is the point of the test.
func (e *testEnv) openServiceRecovery(t *testing.T, business, staff, profileID string) (Interaction, Task) {
	t.Helper()
	ctx := context.Background()
	actor := actorFor(business, staff)
	interaction, err := e.service.CreateInteraction(ctx, actor, globalScope(), CreateInteractionInput{
		CustomerProfileRef: profileID, Type: InteractionServiceFollowUp, Direction: DirectionInbound,
		Outcome: OutcomeUnresolved, Subject: "Late delivery complaint", Note: "Customer reported a late delivery.",
		Visibility: VisibilityBusiness, OccurredAt: e.now,
	})
	if err != nil {
		t.Fatalf("create service-follow-up interaction: %v", err)
	}
	task, err := e.service.CreateTask(ctx, actor, globalScope(), CreateTaskInput{
		CustomerProfileRef: profileID, Title: "Resolve late delivery", Description: "Call the customer back.",
		Visibility: VisibilityBusiness, Priority: PriorityHigh, SourceInteractionRef: interaction.ID,
	})
	if err != nil {
		t.Fatalf("create service-recovery task: %v", err)
	}
	return interaction, task
}

// openOrdinaryTask records a task that is NOT service recovery, so a test can
// prove an ordinary follow-up does not suppress a marketing audience.
func (e *testEnv) openOrdinaryTask(t *testing.T, business, staff, profileID string) Task {
	t.Helper()
	task, err := e.service.CreateTask(context.Background(), actorFor(business, staff), globalScope(), CreateTaskInput{
		CustomerProfileRef: profileID, Title: "Send the catering quote", Description: "Follow up on the quote.",
		Visibility: VisibilityBusiness, Priority: PriorityNormal,
	})
	if err != nil {
		t.Fatalf("create ordinary task: %v", err)
	}
	return task
}

// targetsFor reads a snapshot's targets straight from the database, so a test
// can assert on what was stored rather than on what a read path chose to show.
func (e *testEnv) targetsFor(t *testing.T, business, snapshotID string) map[string]CampaignTarget {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), `SELECT id, customer_profile_id, experiment_variant_key,
		is_control, experiment_bucket, current_actionable, suppression_reason_code
		FROM crm_campaign_targets WHERE business_id=$1 AND snapshot_id=$2`, business, snapshotID)
	if err != nil {
		t.Fatalf("read targets: %v", err)
	}
	defer rows.Close()
	out := map[string]CampaignTarget{}
	for rows.Next() {
		var target CampaignTarget
		if err = rows.Scan(&target.ID, &target.ProfileID, &target.VariantKey, &target.IsControl,
			&target.Bucket, &target.CurrentActionable, &target.SuppressionReason); err != nil {
			t.Fatalf("scan target: %v", err)
		}
		out[target.ProfileID] = target
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("read targets: %v", err)
	}
	return out
}

// channelsFor reads one target's stored consent evidence.
func (e *testEnv) channelsFor(t *testing.T, business, targetID string) map[string]CampaignTargetChannel {
	t.Helper()
	channels, err := e.service.channelsForTargets(context.Background(), business, []string{targetID})
	if err != nil {
		t.Fatalf("read target channels: %v", err)
	}
	out := map[string]CampaignTargetChannel{}
	for _, channel := range channels[targetID] {
		out[channel.Channel] = channel
	}
	return out
}

// assertFunnelReconciles is the invariant every snapshot must satisfy: the
// funnel adds up and the cohorts partition the targets. A snapshot whose counts
// did not reconcile would be a report about an audience other than the one it
// holds.
func assertFunnelReconciles(t *testing.T, snapshot CampaignSnapshot) {
	t.Helper()
	remaining := snapshot.SourceCount - snapshot.SegmentExclusionCount -
		snapshot.PrivacySuppressedCount - snapshot.ServiceRecoverySuppressedCount
	if remaining != snapshot.TargetCount {
		t.Fatalf("funnel does not reconcile: source=%d exclusion=%d privacy=%d recovery=%d target=%d",
			snapshot.SourceCount, snapshot.SegmentExclusionCount, snapshot.PrivacySuppressedCount,
			snapshot.ServiceRecoverySuppressedCount, snapshot.TargetCount)
	}
	if snapshot.ControlCount+snapshot.TreatmentCount != snapshot.TargetCount {
		t.Fatalf("cohorts do not partition targets: control=%d treatment=%d target=%d",
			snapshot.ControlCount, snapshot.TreatmentCount, snapshot.TargetCount)
	}
	for name, value := range map[string]int{
		"source": snapshot.SourceCount, "exclusion": snapshot.SegmentExclusionCount,
		"privacy": snapshot.PrivacySuppressedCount, "recovery": snapshot.ServiceRecoverySuppressedCount,
		"consent": snapshot.ConsentSuppressedCount, "target": snapshot.TargetCount,
		"control": snapshot.ControlCount, "treatment": snapshot.TreatmentCount,
	} {
		if value < 0 {
			t.Fatalf("negative %s count: %d", name, value)
		}
	}
}
