package crm

import (
	"context"
	"testing"
	"time"
)

// Consent is the property this whole section exists to get right. A prepared
// audience is evidence of what the authority said at one moment, and every
// branch that is not an explicit, fresh, unexpired ALLOW must produce an
// ineligible channel. These tests walk the whole matrix rather than the happy
// path, because the dangerous failures here all look like success.

// TestCampaignConsent_MatrixFailsClosed covers the four decision states a
// channel can be in and proves only one of them is permission.
func TestCampaignConsent_MatrixFailsClosed(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	allow := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_allow"))
	deny := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_deny"))
	unknown := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_unknown"))
	missing := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_missing"))
	expired := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_expired"))

	env.allowMarketing(t, businessA, "vs_a", allow.ID, ChannelEmail, 1)
	env.projectConsent(t, businessA, "vs_a", deny.ID, ChannelEmail, ConsentDeny, 1, nil)
	env.projectConsent(t, businessA, "vs_a", unknown.ID, ChannelEmail, ConsentUnknown, 1, nil)
	// An ALLOW that has lapsed is not an ALLOW.
	lapsed := env.now.Add(-time.Hour)
	env.projectConsent(t, businessA, "vs_a", expired.ID, ChannelEmail, ConsentAllow, 1, &lapsed)
	// `missing` deliberately gets no projection at all.

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone",
		allow.ID, deny.ID, unknown.ID, missing.ID, expired.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Consent matrix", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	assertFunnelReconciles(t, snapshot)

	// Everyone becomes a target — consent decides channel eligibility, not
	// membership. Only one of them is contactable.
	if snapshot.TargetCount != 5 {
		t.Fatalf("expected 5 targets, got %d", snapshot.TargetCount)
	}
	if snapshot.ConsentSuppressedCount != 4 {
		t.Fatalf("expected 4 relationships with no eligible channel, got %d", snapshot.ConsentSuppressedCount)
	}

	targets := env.targetsFor(t, businessA, snapshot.ID)
	expectations := []struct {
		name    string
		profile string
		state   string
		reason  string
	}{
		{"explicit allow", allow.ID, ChannelEligibleAtSnapshot, ReasonConsentAllowed},
		{"explicit deny", deny.ID, ChannelIneligibleConsent, ReasonConsentDenied},
		{"explicit unknown", unknown.ID, ChannelIneligibleConsent, ReasonConsentUnknown},
		{"no projection at all", missing.ID, ChannelIneligibleConsent, ReasonConsentUnknown},
		{"lapsed allow", expired.ID, ChannelIneligibleConsent, ReasonConsentUnknown},
	}
	for _, expectation := range expectations {
		t.Run(expectation.name, func(t *testing.T) {
			channels := env.channelsFor(t, businessA, targets[expectation.profile].ID)
			channel, present := channels["EMAIL"]
			if !present {
				t.Fatal("no EMAIL evidence was recorded")
			}
			if channel.SnapshotEligibilityState != expectation.state {
				t.Fatalf("state=%s want %s", channel.SnapshotEligibilityState, expectation.state)
			}
			if channel.ReasonCode != expectation.reason {
				t.Fatalf("reason=%s want %s", channel.ReasonCode, expectation.reason)
			}
			// Whatever the verdict, the recheck obligation is unconditional.
			if !channel.SendTimeRecheckRequired {
				t.Fatal("send-time recheck must be required on every channel row")
			}
			// An ineligible row must never claim ALLOW: the stored decision and
			// the stored verdict have to agree.
			if !channel.SnapshotConsentEligible && channel.SnapshotConsentDecision == string(ConsentAllow) {
				t.Fatal("an ineligible channel recorded an ALLOW decision")
			}
		})
	}
}

// TestCampaignConsent_NoAuthorityMeansNobodyIsEligible is the fail-closed test
// that matters most today: no consent publisher exists anywhere in the
// platform, and a campaign engine that treated that as permission would target
// everybody.
func TestCampaignConsent_NoAuthorityMeansNobodyIsEligible(t *testing.T) {
	flags := allFlagsOn()
	flags.ConsentIntegrationEnabled = false
	env := newTestEnv(t, flags)

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	// Even a stored ALLOW cannot rescue this: with no integrated authority
	// there is nothing to trust the projection against.
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("No authority", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	if snapshot.TargetCount != 1 {
		t.Fatalf("expected 1 target, got %d", snapshot.TargetCount)
	}
	if snapshot.ConsentSuppressedCount != 1 {
		t.Fatalf("with no consent authority nobody is contactable, got %d suppressed", snapshot.ConsentSuppressedCount)
	}
	targets := env.targetsFor(t, businessA, snapshot.ID)
	channel := env.channelsFor(t, businessA, targets[profile.ID].ID)["EMAIL"]
	if channel.SnapshotEligibilityState != ChannelIneligibleConsent {
		t.Fatalf("expected ineligible, got %s", channel.SnapshotEligibilityState)
	}
	if channel.ReasonCode != ReasonConsentUnavailable {
		t.Fatalf("expected CONSENT_SOURCE_UNAVAILABLE, got %s", channel.ReasonCode)
	}
}

// TestCampaignConsent_IsDecidedPerChannel proves a customer who allowed email
// and refused push is contactable on exactly one of them.
func TestCampaignConsent_IsDecidedPerChannel(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	env.projectConsent(t, businessA, "vs_a", profile.ID, ChannelPush, ConsentDeny, 1, nil)
	// SMS is left entirely unobserved.

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a",
		campaignInput("Multi channel", audience.ID, "EMAIL", "PUSH", "SMS"))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	targets := env.targetsFor(t, businessA, snapshot.ID)
	channels := env.channelsFor(t, businessA, targets[profile.ID].ID)
	if len(channels) != 3 {
		t.Fatalf("expected evidence for all three campaign channels, got %d", len(channels))
	}
	if channels["EMAIL"].SnapshotEligibilityState != ChannelEligibleAtSnapshot {
		t.Fatalf("email should be eligible: %+v", channels["EMAIL"])
	}
	if channels["PUSH"].SnapshotEligibilityState != ChannelIneligibleConsent {
		t.Fatalf("push should be ineligible: %+v", channels["PUSH"])
	}
	if channels["SMS"].SnapshotEligibilityState != ChannelIneligibleConsent {
		t.Fatalf("unobserved SMS should be ineligible: %+v", channels["SMS"])
	}
	// The relationship has at least one eligible channel, so it is not counted
	// as consent-suppressed.
	if snapshot.ConsentSuppressedCount != 0 {
		t.Fatalf("expected nobody fully suppressed, got %d", snapshot.ConsentSuppressedCount)
	}

	// The per-channel figures reported on the snapshot are derived from these
	// very rows, so they cannot drift from them.
	counts, err := env.service.SnapshotChannelCounts(context.Background(), businessA, snapshot.ID)
	if err != nil {
		t.Fatalf("channel counts: %v", err)
	}
	byChannel := map[string]CampaignChannelProjection{}
	for _, count := range counts {
		byChannel[count.Channel] = count
	}
	if byChannel["EMAIL"].EligibleCount != 1 || byChannel["PUSH"].EligibleCount != 0 || byChannel["SMS"].EligibleCount != 0 {
		t.Fatalf("derived channel counts disagree with the stored evidence: %+v", byChannel)
	}
}

// TestCampaignConsent_StoresTheAuthorityVersionAsEvidence proves the snapshot
// records what was observed, not merely that something was.
func TestCampaignConsent_StoresTheAuthorityVersionAsEvidence(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 7)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Evidence", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	targets := env.targetsFor(t, businessA, snapshot.ID)
	channel := env.channelsFor(t, businessA, targets[profile.ID].ID)["EMAIL"]
	if channel.ConsentSourceVersion == nil || *channel.ConsentSourceVersion != 7 {
		t.Fatalf("expected the authority version to be retained, got %v", channel.ConsentSourceVersion)
	}
	if channel.ConsentAsOf == nil {
		t.Fatal("expected the observation time to be retained")
	}
}

// TestCampaignConsent_ChangingAfterTheSnapshotDoesNotRewriteHistory is the
// dual-time rule. The frozen evidence stays exactly as it was, and the send-time
// recheck is what protects the customer who changed their mind.
func TestCampaignConsent_ChangingAfterTheSnapshotDoesNotRewriteHistory(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Mind changed", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	targets := env.targetsFor(t, businessA, snapshot.ID)
	before := env.channelsFor(t, businessA, targets[profile.ID].ID)["EMAIL"]
	if before.SnapshotEligibilityState != ChannelEligibleAtSnapshot {
		t.Fatalf("expected an eligible snapshot row: %+v", before)
	}

	// The customer withdraws consent after the audience was frozen.
	env.projectConsent(t, businessA, "vs_a", profile.ID, ChannelEmail, ConsentDeny, 2, nil)

	after := env.channelsFor(t, businessA, targets[profile.ID].ID)["EMAIL"]
	if after.SnapshotEligibilityState != ChannelEligibleAtSnapshot {
		t.Fatal("the historical snapshot evidence was rewritten by a later consent change")
	}
	if !after.SendTimeRecheckRequired {
		t.Fatal("the send-time recheck obligation disappeared")
	}
	// And a fresh preparation reflects the new decision, which is how the
	// change actually takes effect for a future audience.
	replacement := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	newTargets := env.targetsFor(t, businessA, replacement.ID)
	fresh := env.channelsFor(t, businessA, newTargets[profile.ID].ID)["EMAIL"]
	if fresh.SnapshotEligibilityState != ChannelIneligibleConsent {
		t.Fatalf("a re-prepared audience ignored the withdrawal: %+v", fresh)
	}
}

// TestCampaignConsent_ControlAssignmentDoesNotBypassConsent proves the holdout
// cohort is not a loophole. A control member still has to satisfy every
// audience-safety rule before it can be counted in an experiment.
func TestCampaignConsent_ControlAssignmentDoesNotBypassConsent(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	// Twenty relationships with no consent at all, so every cohort should be
	// fully ineligible whichever side of the split they land on.
	var ids []string
	for index := 0; index < 20; index++ {
		profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_"+itoa(index)))
		ids = append(ids, profile.ID)
	}
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", ids...)

	definition := campaignInput("Holdout", audience.ID)
	yes := true
	definition.ExperimentEnabled = &yes
	definition.Variants = []ExperimentVariantInput{
		{VariantKey: "CONTROL", DisplayName: "Holdout", AllocationBps: 5000, IsControl: true},
		{VariantKey: "A", DisplayName: "Treatment", AllocationBps: 5000},
	}
	campaign := env.createCampaign(t, businessA, "vs_a", definition)
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	assertFunnelReconciles(t, snapshot)

	if snapshot.ControlCount == 0 || snapshot.TreatmentCount == 0 {
		t.Fatalf("expected both cohorts to be populated, got control=%d treatment=%d",
			snapshot.ControlCount, snapshot.TreatmentCount)
	}
	if snapshot.ConsentSuppressedCount != snapshot.TargetCount {
		t.Fatalf("no relationship has consent, so all %d should be suppressed, got %d",
			snapshot.TargetCount, snapshot.ConsentSuppressedCount)
	}
	targets := env.targetsFor(t, businessA, snapshot.ID)
	for profileID, target := range targets {
		channel := env.channelsFor(t, businessA, target.ID)["EMAIL"]
		if channel.SnapshotEligibilityState != ChannelIneligibleConsent {
			t.Fatalf("%s (control=%t) became eligible without consent", profileID, target.IsControl)
		}
	}
}

// TestCampaignConsent_SegmentMembershipIsNotPermission states the rule plainly:
// being selected by a rule a vendor wrote says nothing about what the customer
// agreed to.
func TestCampaignConsent_SegmentMembershipIsNotPermission(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_1", branchA1, 5000, 5000, env.now.AddDate(0, 0, -2), 1)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_2", branchA1, 5000, 5000, env.now.AddDate(0, 0, -1), 1)

	audience := env.dynamicAudience(t, businessA, "vs_a", "Repeat buyers")
	if audience.CurrentMemberCount != 1 {
		t.Fatalf("expected the rule to select the relationship, got %d members", audience.CurrentMemberCount)
	}
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Rule selected", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	if snapshot.TargetCount != 1 {
		t.Fatalf("expected the rule's member to be a target, got %d", snapshot.TargetCount)
	}
	if snapshot.ConsentSuppressedCount != 1 {
		t.Fatal("a rule match granted contact permission it has no authority to grant")
	}
}
