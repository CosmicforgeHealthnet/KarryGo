package crm

import (
	"context"
	"testing"
)

// Privacy in a campaign is two obligations, not one: the customer must stop
// being contactable immediately, and the record of why a past campaign targeted
// whom it targeted must survive. Section 8E keeps them apart deliberately —
// suppression revokes actionability, and the frozen evidence is never rewritten.

// TestCampaignPrivacy_RedactedRelationshipsNeverBecomeTargets is the strongest
// form of the guarantee: a redacted customer has no row in the only table
// Section 8F will read, so no future query mistake can reach them.
func TestCampaignPrivacy_RedactedRelationshipsNeverBecomeTargets(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	keep := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_keep"))
	erased := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_erased"))
	env.allowMarketing(t, businessA, "vs_a", keep.ID, ChannelEmail, 1)
	env.allowMarketing(t, businessA, "vs_a", erased.ID, ChannelEmail, 1)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", keep.ID, erased.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &erased, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_erased",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatalf("redact: %v", err)
	}

	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("After erasure", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	assertFunnelReconciles(t, snapshot)

	targets := env.targetsFor(t, businessA, snapshot.ID)
	if _, present := targets[erased.ID]; present {
		t.Fatal("a redacted relationship became a campaign target")
	}
	if _, present := targets[keep.ID]; !present {
		t.Fatal("the remaining relationship is missing")
	}
	// Redaction already removed the relationship from the segment, so it never
	// reaches the campaign funnel at all. Either way it cannot be targeted; the
	// counts simply have to reconcile with whichever path it took.
	if snapshot.TargetCount != 1 {
		t.Fatalf("expected exactly one target, got %d", snapshot.TargetCount)
	}
}

// TestCampaignPrivacy_StructuralSuppressionIsCountedNotTargeted covers the
// non-consent reasons a relationship cannot be marketed to. They are aggregate
// counts, never target rows.
func TestCampaignPrivacy_StructuralSuppressionIsCountedNotTargeted(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	contactable := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_ok"))
	suppressed := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_suppressed"))
	restricted := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_restricted"))
	for _, profile := range []Profile{contactable, suppressed, restricted} {
		env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	}
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone",
		contactable.ID, suppressed.ID, restricted.ID)

	if err := env.applyPrivacy(t, businessA, "vs_a", &suppressed, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_suppressed",
		Action: PrivacyActionMarketingSuppress, ObservedAt: env.now,
	}); err != nil {
		t.Fatalf("marketing suppress: %v", err)
	}
	if err := env.applyPrivacy(t, businessA, "vs_a", &restricted, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_restricted",
		Action: PrivacyActionRestrict, ObservedAt: env.now,
	}); err != nil {
		t.Fatalf("restrict: %v", err)
	}

	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Structural", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	assertFunnelReconciles(t, snapshot)

	targets := env.targetsFor(t, businessA, snapshot.ID)
	for name, profileID := range map[string]string{"marketing-suppressed": suppressed.ID, "restricted": restricted.ID} {
		if _, present := targets[profileID]; present {
			t.Fatalf("a %s relationship became a campaign target", name)
		}
	}
	if _, present := targets[contactable.ID]; !present {
		t.Fatal("the contactable relationship is missing")
	}

	// Whatever route each one took out of the funnel, the arithmetic must hold
	// and no target row may exist for them.
	var stray int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_campaign_targets
		WHERE business_id=$1 AND customer_profile_id = ANY($2)`,
		businessA, []string{suppressed.ID, restricted.ID}).Scan(&stray); err != nil {
		t.Fatal(err)
	}
	if stray != 0 {
		t.Fatalf("suppressed relationships hold %d target rows", stray)
	}
}

// TestCampaignPrivacy_RedactionAfterTheSnapshotRevokesActionability is the
// post-snapshot rule: the frozen evidence survives for audit, and the customer
// stops being contactable the moment the redaction commits — not at some future
// delivery-time discovery.
func TestCampaignPrivacy_RedactionAfterTheSnapshotRevokesActionability(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Erased later", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	targets := env.targetsFor(t, businessA, snapshot.ID)
	target := targets[profile.ID]
	if !target.CurrentActionable {
		t.Fatal("the target should start actionable")
	}
	beforeChannel := env.channelsFor(t, businessA, target.ID)["EMAIL"]

	reloaded := env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &reloaded, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_1",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatalf("redact: %v", err)
	}

	after := env.targetsFor(t, businessA, snapshot.ID)[profile.ID]
	if after.CurrentActionable {
		t.Fatal("a redacted relationship is still an actionable campaign target")
	}
	if after.SuppressionReason == nil || *after.SuppressionReason != SuppressionRedacted {
		t.Fatalf("expected a PRIVACY_REDACTED suppression reason, got %v", after.SuppressionReason)
	}
	// The historical evidence is untouched: the campaign remains explicable.
	afterChannel := env.channelsFor(t, businessA, target.ID)["EMAIL"]
	if afterChannel.SnapshotEligibilityState != beforeChannel.SnapshotEligibilityState {
		t.Fatal("the frozen snapshot evidence was rewritten by a redaction")
	}
	// The snapshot itself keeps its counts: it is a record of a past decision.
	if env.reloadSnapshot(t, businessA, snapshot.ID).TargetCount != 1 {
		t.Fatal("a redaction rewrote the historical snapshot counts")
	}
	// And the target row still carries no personal data of any kind.
	var columns int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name IN ('crm_campaign_targets','crm_campaign_target_channels')
		  AND column_name ~ '(email|phone|address|first_name|last_name|token)'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 0 {
		t.Fatalf("campaign target tables hold %d personal-data columns", columns)
	}
}

// TestCampaignPrivacy_MergeStopsTheSourceBeingIndependentlyActionable covers the
// duplicate-send risk. The merged-away relationship is revoked rather than
// moved: moving it would assert the survivor was selected by a definition that
// never evaluated them.
func TestCampaignPrivacy_MergeStopsTheSourceBeingIndependentlyActionable(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	guest := env.seedProfile(t, businessA, "vs_a", guestSubject("gst_1"))
	customer := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", guest.ID, ChannelEmail, 1)
	env.allowMarketing(t, businessA, "vs_a", customer.ID, ChannelEmail, 1)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", guest.ID, customer.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Before merge", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	before := env.targetsFor(t, businessA, snapshot.ID)
	if len(before) != 2 {
		t.Fatalf("expected both relationships to be targeted, got %d", len(before))
	}

	if err := env.applyLink(t, businessA, "vs_a",
		linkInput("evt_merge_1", guestSubject("gst_1"), customerSubject("cus_1"), 4)); err != nil {
		t.Fatalf("merge: %v", err)
	}

	after := env.targetsFor(t, businessA, snapshot.ID)
	if after[guest.ID].CurrentActionable {
		t.Fatal("the merged-away relationship is still independently actionable")
	}
	if after[guest.ID].SuppressionReason == nil || *after[guest.ID].SuppressionReason != SuppressionIdentityMerged {
		t.Fatalf("expected an IDENTITY_MERGED suppression reason, got %v", after[guest.ID].SuppressionReason)
	}
	if !after[customer.ID].CurrentActionable {
		t.Fatal("the surviving relationship should remain actionable exactly once")
	}
	// Exactly one of the two rows is actionable, which is what stops the merged
	// human being contacted twice from this audience.
	actionable := 0
	for _, target := range after {
		if target.CurrentActionable {
			actionable++
		}
	}
	if actionable != 1 {
		t.Fatalf("expected one actionable row for the merged identity, got %d", actionable)
	}
	// The historical membership is intact: both rows still exist as evidence.
	if len(after) != 2 {
		t.Fatalf("the merge rewrote historical snapshot membership: %d rows", len(after))
	}
}

// TestCampaignPrivacy_CannotBeDisabledByADefinition states the policy: privacy
// is engine behaviour, not a campaign setting. There is no field a vendor could
// set to opt out, and the closest thing they can control — service-recovery
// suppression — does not touch privacy at all.
func TestCampaignPrivacy_CannotBeDisabledByADefinition(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	erased := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_erased"))
	env.allowMarketing(t, businessA, "vs_a", erased.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", erased.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &erased, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_erased",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatalf("redact: %v", err)
	}

	// Turn off everything a vendor is allowed to turn off.
	definition := campaignInput("Maximum reach", audience.ID)
	no := false
	definition.ServiceRecoverySuppression = &no
	campaign := env.createCampaign(t, businessA, "vs_a", definition)
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	if len(env.targetsFor(t, businessA, snapshot.ID)) != 0 {
		t.Fatal("a campaign definition reached a redacted relationship")
	}
}

// ---------------------------------------------------------------------------
// Service recovery
// ---------------------------------------------------------------------------

// TestCampaignServiceRecovery_SuppressesOnlyTheRealSignal pins the exact
// Vendor-local rule. An open task linked to a service-follow-up interaction is
// service recovery; an ordinary open task is not, and treating every task as a
// complaint would suppress most of a busy vendor's audience for no reason.
func TestCampaignServiceRecovery_SuppressesOnlyTheRealSignal(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	complaining := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_complaint"))
	busy := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_busy"))
	quiet := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_quiet"))
	for _, profile := range []Profile{complaining, busy, quiet} {
		env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	}

	env.openServiceRecovery(t, businessA, "vs_a", complaining.ID)
	env.openOrdinaryTask(t, businessA, "vs_a", busy.ID)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", complaining.ID, busy.ID, quiet.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Mid complaint", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	assertFunnelReconciles(t, snapshot)

	if snapshot.ServiceRecoverySuppressedCount != 1 {
		t.Fatalf("expected exactly one service-recovery suppression, got %d", snapshot.ServiceRecoverySuppressedCount)
	}
	targets := env.targetsFor(t, businessA, snapshot.ID)
	if _, present := targets[complaining.ID]; present {
		t.Fatal("a customer in active service recovery became a marketing target")
	}
	if _, present := targets[busy.ID]; !present {
		t.Fatal("an ordinary open task suppressed a marketing target")
	}
	if _, present := targets[quiet.ID]; !present {
		t.Fatal("a customer with no tasks was suppressed")
	}
}

// TestCampaignServiceRecovery_EndsWhenTheWorkflowCloses proves the signal is a
// state and not a permanent mark.
func TestCampaignServiceRecovery_EndsWhenTheWorkflowCloses(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	_, task := env.openServiceRecovery(t, businessA, "vs_a", profile.ID)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Recovery", audience.ID))

	suppressed := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	if suppressed.TargetCount != 0 || suppressed.ServiceRecoverySuppressedCount != 1 {
		t.Fatalf("expected suppression while the complaint is open: target=%d recovery=%d",
			suppressed.TargetCount, suppressed.ServiceRecoverySuppressedCount)
	}

	if _, err := env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCompleted, task.Version); err != nil {
		t.Fatalf("complete the recovery task: %v", err)
	}
	resumed := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	if resumed.TargetCount != 1 || resumed.ServiceRecoverySuppressedCount != 0 {
		t.Fatalf("a resolved complaint should stop suppressing: target=%d recovery=%d",
			resumed.TargetCount, resumed.ServiceRecoverySuppressedCount)
	}
}

// TestCampaignServiceRecovery_StartingAfterTheSnapshotRevokesActionability
// covers the race the section explicitly calls out. The frozen snapshot is
// unchanged; the customer stops being contactable now rather than at delivery.
func TestCampaignServiceRecovery_StartingAfterTheSnapshotRevokesActionability(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Complaint later", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	targets, _, err := env.service.ListSnapshotTargets(ctx, actor, globalScope(), campaign.ID, snapshot.ID,
		TargetListOptions{})
	if err != nil {
		t.Fatalf("list targets: %v", err)
	}
	if len(targets) != 1 || !targets[0].Actionable() {
		t.Fatalf("expected one actionable target, got %+v", targets)
	}

	// The complaint arrives after the audience was frozen.
	env.openServiceRecovery(t, businessA, "vs_a", profile.ID)

	after, _, err := env.service.ListSnapshotTargets(ctx, actor, globalScope(), campaign.ID, snapshot.ID,
		TargetListOptions{})
	if err != nil {
		t.Fatalf("list targets: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("the historical membership changed: %d rows", len(after))
	}
	if !after[0].ServiceRecoveryActive || after[0].Actionable() {
		t.Fatalf("a customer who entered service recovery is still actionable: %+v", after[0])
	}
	// The stored snapshot evidence is untouched — only current actionability
	// moved.
	if !after[0].CurrentActionable {
		t.Fatal("service recovery should be evaluated live, not written into the frozen row")
	}

	// And the actionable-only filter honours it.
	filtered, _, err := env.service.ListSnapshotTargets(ctx, actor, globalScope(), campaign.ID, snapshot.ID,
		TargetListOptions{ActionableOnly: true})
	if err != nil {
		t.Fatalf("list actionable targets: %v", err)
	}
	if len(filtered) != 0 {
		t.Fatalf("the actionable filter returned a suppressed target: %+v", filtered)
	}
}

// TestCampaignServiceRecovery_IsTenantLocal proves one business's complaints
// cannot suppress another business's audience.
func TestCampaignServiceRecovery_IsTenantLocal(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	profileA := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_shared"))
	profileB := env.seedProfile(t, businessB, "vs_b", customerSubject("cus_shared"))
	env.allowMarketing(t, businessA, "vs_a", profileA.ID, ChannelEmail, 1)
	env.allowMarketing(t, businessB, "vs_b", profileB.ID, ChannelEmail, 1)

	// The same human is mid-complaint at business B and perfectly happy at A.
	env.openServiceRecovery(t, businessB, "vs_b", profileB.ID)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profileA.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Tenant local", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	if snapshot.ServiceRecoverySuppressedCount != 0 || snapshot.TargetCount != 1 {
		t.Fatalf("another tenant's complaint suppressed this audience: recovery=%d target=%d",
			snapshot.ServiceRecoverySuppressedCount, snapshot.TargetCount)
	}
}

// TestCampaignServiceRecovery_CanBeTurnedOffDeliberately proves the policy is a
// definition-level choice, and that turning it off changes only this one thing.
func TestCampaignServiceRecovery_CanBeTurnedOffDeliberately(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	env.openServiceRecovery(t, businessA, "vs_a", profile.ID)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	definition := campaignInput("Opted out", audience.ID)
	no := false
	definition.ServiceRecoverySuppression = &no
	campaign := env.createCampaign(t, businessA, "vs_a", definition)
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	if snapshot.ServiceRecoverySuppressedCount != 0 || snapshot.TargetCount != 1 {
		t.Fatalf("an explicit opt-out was ignored: recovery=%d target=%d",
			snapshot.ServiceRecoverySuppressedCount, snapshot.TargetCount)
	}
}
