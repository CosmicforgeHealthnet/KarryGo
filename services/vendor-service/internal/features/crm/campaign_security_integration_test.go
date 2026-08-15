package crm

import (
	"context"
	"testing"
	"time"
)

// Every read and every command in Section 8E resolves its tenant from the
// server-side session, never from the path or the body. These tests probe the
// boundary from the outside: one tenant holding a valid session tries to reach
// another tenant's campaigns, and a branch-scoped actor tries to reach beyond
// their branch. Both must fail, and must fail in a way that discloses nothing.

func TestCampaignSecurity_TenantIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actorA := actorFor(businessA, "vs_a")
	actorB := actorFor(businessB, "vs_b")

	profileB := env.seedProfile(t, businessB, "vs_b", customerSubject("cus_b"))
	env.allowMarketing(t, businessB, "vs_b", profileB.ID, ChannelEmail, 1)
	audienceB := env.staticAudience(t, businessB, "vs_b", "B customers", profileB.ID)
	campaignB := env.createCampaign(t, businessB, "vs_b", campaignInput("B campaign", audienceB.ID))
	snapshotB := env.prepareAndRun(t, businessB, "vs_b", campaignB.ID)

	t.Run("cannot read another tenant's campaign", func(t *testing.T) {
		if _, err := env.service.GetCampaign(ctx, actorA, globalScope(), campaignB.ID); err == nil {
			t.Fatal("tenant A read tenant B's campaign")
		} else if !isCRMCode(err, CodeCampaignNotFound) {
			// Not-found rather than forbidden: a forbidden would confirm the id
			// exists somewhere.
			t.Fatalf("expected a not-found response, got %v", err)
		}
	})

	t.Run("cannot see another tenant's campaign in a list", func(t *testing.T) {
		campaigns, _, err := env.service.ListCampaigns(ctx, actorA, globalScope(), CampaignListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, campaign := range campaigns {
			if campaign.ID == campaignB.ID {
				t.Fatal("tenant B's campaign appeared in tenant A's list")
			}
		}
	})

	t.Run("cannot update, archive, preview or prepare another tenant's campaign", func(t *testing.T) {
		if _, _, err := env.service.UpdateCampaign(ctx, actorA, globalScope(), campaignB.ID, UpdateCampaignInput{
			Definition: campaignInput("Hijacked", audienceB.ID), ExpectedVersion: 1}); err == nil {
			t.Fatal("tenant A updated tenant B's campaign")
		}
		if _, err := env.service.ArchiveCampaign(ctx, actorA, globalScope(), campaignB.ID, 0); err == nil {
			t.Fatal("tenant A archived tenant B's campaign")
		}
		if _, err := env.service.PreviewCampaign(ctx, actorA, globalScope(), campaignB.ID, CampaignPreviewInput{}); err == nil {
			t.Fatal("tenant A previewed tenant B's campaign")
		}
		if _, err := env.service.PrepareCampaign(ctx, actorA, globalScope(), campaignB.ID); err == nil {
			t.Fatal("tenant A prepared tenant B's campaign")
		}
	})

	t.Run("cannot read another tenant's snapshots or targets", func(t *testing.T) {
		if _, _, err := env.service.ListSnapshots(ctx, actorA, globalScope(), campaignB.ID, 0, 0); err == nil {
			t.Fatal("tenant A listed tenant B's snapshots")
		}
		if _, err := env.service.GetSnapshot(ctx, actorA, globalScope(), campaignB.ID, snapshotB.ID); err == nil {
			t.Fatal("tenant A read tenant B's snapshot")
		}
		if _, _, err := env.service.ListSnapshotTargets(ctx, actorA, globalScope(), campaignB.ID, snapshotB.ID,
			TargetListOptions{}); err == nil {
			t.Fatal("tenant A listed tenant B's campaign targets")
		}
	})

	t.Run("cannot build a campaign over another tenant's audience", func(t *testing.T) {
		if _, _, err := env.service.CreateCampaign(ctx, actorA, globalScope(),
			campaignInput("Cross tenant", audienceB.ID)); err == nil {
			t.Fatal("tenant A built a campaign over tenant B's segment")
		}
		// Nor as an exclusion, which would otherwise be a subtler leak: the
		// resulting counts would describe tenant B's membership.
		definition := campaignInput("Cross tenant exclusion", audienceB.ID)
		audienceA := env.staticAudience(t, businessA, "vs_a", "A customers")
		definition.SourceSegmentRef = audienceA.ID
		definition.ExclusionSegmentRefs = []string{audienceB.ID}
		if _, _, err := env.service.CreateCampaign(ctx, actorA, globalScope(), definition); err == nil {
			t.Fatal("tenant A used tenant B's segment as an exclusion")
		}
	})

	t.Run("tenant B is unaffected", func(t *testing.T) {
		if _, err := env.service.GetCampaign(ctx, actorB, globalScope(), campaignB.ID); err != nil {
			t.Fatalf("tenant B lost access to its own campaign: %v", err)
		}
	})
}

func TestCampaignSecurity_BranchIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	owner := actorFor(businessA, "vs_a")

	// One relationship at each branch, each contactable.
	atOne := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_branch_1"))
	atTwo := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_branch_2"))
	env.seedOrder(t, businessA, "vs_a", atOne, "ord_b1", branchA1, 5000, 5000, env.now.AddDate(0, 0, -1), 1)
	env.seedOrder(t, businessA, "vs_a", atTwo, "ord_b2", branchA2, 5000, 5000, env.now.AddDate(0, 0, -1), 1)
	env.allowMarketing(t, businessA, "vs_a", atOne.ID, ChannelEmail, 1)
	env.allowMarketing(t, businessA, "vs_a", atTwo.ID, ChannelEmail, 1)

	audienceOne := env.branchAudience(t, businessA, "vs_a", "Branch one regulars", branchA1, atOne.ID)
	audienceTwo := env.branchAudience(t, businessA, "vs_a", "Branch two regulars", branchA2, atTwo.ID)

	branchOneCampaign := campaignInput("Branch one offer", audienceOne.ID)
	branchOneCampaign.ScopeType = ScopeBranch
	branchOneCampaign.BranchRef = branchA1
	campaignOne := env.createCampaign(t, businessA, "vs_a", branchOneCampaign)

	branchTwoCampaign := campaignInput("Branch two offer", audienceTwo.ID)
	branchTwoCampaign.ScopeType = ScopeBranch
	branchTwoCampaign.BranchRef = branchA2
	campaignTwo := env.createCampaign(t, businessA, "vs_a", branchTwoCampaign)

	businessWide := env.staticAudience(t, businessA, "vs_a", "Everyone", atOne.ID, atTwo.ID)
	businessCampaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Business offer", businessWide.ID))

	branchOne := branchScope(branchA1)

	t.Run("a branch actor sees only their own branch's campaigns", func(t *testing.T) {
		campaigns, _, err := env.service.ListCampaigns(ctx, owner, branchOne, CampaignListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(campaigns) != 1 || campaigns[0].ID != campaignOne.ID {
			t.Fatalf("expected only the branch-one campaign, got %d", len(campaigns))
		}
	})

	t.Run("another branch's campaign is not found", func(t *testing.T) {
		if _, err := env.service.GetCampaign(ctx, owner, branchOne, campaignTwo.ID); err == nil {
			t.Fatal("a branch actor read another branch's campaign")
		}
		if _, _, err := env.service.ListSnapshots(ctx, owner, branchOne, campaignTwo.ID, 0, 0); err == nil {
			t.Fatal("a branch actor listed another branch's snapshots")
		}
	})

	t.Run("a business-wide campaign is not visible at branch scope", func(t *testing.T) {
		if _, err := env.service.GetCampaign(ctx, owner, branchOne, businessCampaign.ID); err == nil {
			t.Fatal("a branch actor read a business-wide campaign")
		}
		// The count is the disclosure, so preview is refused too.
		if _, err := env.service.PreviewCampaign(ctx, owner, branchOne, businessCampaign.ID,
			CampaignPreviewInput{}); err == nil {
			t.Fatal("a branch actor previewed a business-wide audience")
		}
	})

	t.Run("a branch actor can work within their own branch", func(t *testing.T) {
		preview, err := env.service.PreviewCampaign(ctx, owner, branchOne, campaignOne.ID, CampaignPreviewInput{})
		if err != nil {
			t.Fatalf("a branch actor could not preview their own campaign: %v", err)
		}
		// Exactly their branch's relationship, and nobody else's.
		if preview.SourceCount != 1 || preview.TargetCount != 1 {
			t.Fatalf("expected the branch's single relationship, got source=%d target=%d",
				preview.SourceCount, preview.TargetCount)
		}
		for _, sample := range preview.Sample {
			if sample.ProfileRef == atTwo.ID {
				t.Fatal("another branch's relationship appeared in a branch preview sample")
			}
		}
	})

	t.Run("a branch snapshot holds only that branch's relationships", func(t *testing.T) {
		snapshot := env.prepareAndRun(t, businessA, "vs_a", campaignOne.ID)
		targets := env.targetsFor(t, businessA, snapshot.ID)
		if len(targets) != 1 {
			t.Fatalf("expected one target, got %d", len(targets))
		}
		if _, leaked := targets[atTwo.ID]; leaked {
			t.Fatal("another branch's relationship reached a branch campaign snapshot")
		}
		listed, _, err := env.service.ListSnapshotTargets(ctx, owner, branchOne, campaignOne.ID, snapshot.ID,
			TargetListOptions{})
		if err != nil {
			t.Fatalf("a branch actor could not list their own targets: %v", err)
		}
		if len(listed) != 1 || listed[0].ProfileID != atOne.ID {
			t.Fatalf("unexpected branch target list: %+v", listed)
		}
	})

	t.Run("filtering by another branch is refused rather than answered", func(t *testing.T) {
		if _, _, err := env.service.ListCampaigns(ctx, owner, branchOne,
			CampaignListOptions{BranchRef: branchA2}); err == nil {
			t.Fatal("a branch actor filtered by another branch")
		}
	})
}

// TestCampaignSecurity_NoTableHoldsContactData is the schema-level disclosure
// guarantee. Section 8E cannot leak a contact detail because it has nowhere to
// put one.
func TestCampaignSecurity_NoTableHoldsContactData(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	var columns int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name LIKE 'crm_campaign%'
		  AND column_name ~ '(email|phone|address|first_name|last_name|password|device_token|push_token|msisdn)'`).
		Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 0 {
		t.Fatalf("the campaign schema holds %d contact-data columns", columns)
	}

	// The Section 8E-owned intelligence tables still hold no delivery machinery;
	// Section 8F adds that machinery in its own run/delivery tables.
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name IN
		 ('crm_campaigns','crm_campaign_versions','crm_campaign_experiment_variants',
		  'crm_campaign_audience_snapshots','crm_campaign_targets','crm_campaign_target_channels')
		  AND column_name ~ '(scheduled_at|sent_at|delivered_at|provider|template|message_body|delivery|dispatch)'`).
		Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 0 {
		t.Fatalf("the campaign schema holds %d delivery columns", columns)
	}
}

// TestCampaignSecurity_AuditRecordsMetadataNotAudiences proves the audit trail
// explains what happened without becoming a second copy of the customer list.
func TestCampaignSecurity_AuditRecordsMetadataNotAudiences(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_audit"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Audited", audience.ID))
	env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	rows, err := env.pool.Query(ctx, `SELECT action, COALESCE(after_data::text,'') FROM vendor_audit_log
		WHERE vendor_id=$1 AND aggregate_type='crm_campaign' ORDER BY id`, businessA)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	actions := map[string]bool{}
	for rows.Next() {
		var action, after string
		if err = rows.Scan(&action, &after); err != nil {
			t.Fatal(err)
		}
		actions[action] = true
		// The audit payload must never name a customer.
		if after != "" && containsFold(after, profile.ID) {
			t.Fatalf("audit entry %s names a customer relationship: %s", action, after)
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"vendor.crm.campaign.created",
		"vendor.crm.campaign.preparation.started",
		"vendor.crm.campaign.prepared",
	} {
		if !actions[want] {
			t.Fatalf("expected an audit entry for %s, got %v", want, actions)
		}
	}
}

// TestCampaignSecurity_OutboxCarriesIdentityNotMembership is the same rule for
// the event bus, where a target list would be an unrequested customer export.
func TestCampaignSecurity_OutboxCarriesIdentityNotMembership(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	// The shared harness has no publisher; wire a recording one so the event
	// payloads themselves can be inspected rather than merely assumed clean.
	published := &recordingEvents{}
	env.service = NewService(Options{DB: env.pool, Scopes: fixedScopes{"vs_a": {Global: true}}, Audit: env.audit,
		Events: published, Flags: allFlagsOn(), Now: func() time.Time { return env.now }})

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_outbox"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Published", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	dump := published.dump(t)
	// No customer reference reaches the event bus. A target list published as
	// an event would be a customer export nobody asked for.
	if containsFold(dump, profile.ID) {
		t.Fatalf("a campaign event carries a customer reference: %s", dump)
	}
	// And nothing that would read as a delivery having happened.
	for _, forbidden := range []string{"campaign.sent", "notification.sent", "sms.sent", "email.sent", "scheduled"} {
		if containsFold(dump, forbidden) {
			t.Fatalf("Section 8E published a delivery event: %s", dump)
		}
	}

	emitted := map[string]bool{}
	for _, eventType := range published.types {
		emitted[eventType] = true
	}
	for _, want := range []string{"vendor.crm.campaign.created", "vendor.crm.campaign.prepared"} {
		if !emitted[want] {
			t.Fatalf("expected a %s event, got %v", want, published.types)
		}
	}
	// The prepared event carries the audience's identity and size, which is
	// what a downstream consumer legitimately needs, plus the standing warning
	// that preparation is not permission.
	for _, want := range []string{snapshot.ID, "target_count", "send_time_recheck_required"} {
		if !containsFold(dump, want) {
			t.Fatalf("the prepared event omits %s: %s", want, dump)
		}
	}
}
