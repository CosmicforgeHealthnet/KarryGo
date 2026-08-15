package crm

import (
	"context"
	"strings"
	"testing"
)

// A deliberately modest fixture: 1,000 relationships, one source segment, one
// exclusion segment, three campaigns and one prepared snapshot over the full
// 1,000 candidates. It is large enough to cross the 500-row batch boundary
// twice, to make an unindexed scan obvious in the query plan, and to prove
// pagination is complete — and small enough that it does not turn the suite
// into a benchmark. The exact size is recorded here so a later reader knows
// what was and was not demonstrated.
const (
	scaleFixtureProfiles = 1000
	scaleExcluded        = 100
)

func seedScaleFixture(t *testing.T, env *testEnv) (source Segment, exclusion Segment, profileIDs []string) {
	t.Helper()
	ctx := context.Background()

	// Bulk-insert the relationships directly: this fixture is about the
	// campaign engine's behaviour at size, not about re-proving Section 8A's
	// ingestion path a thousand times.
	ids := make([]string, 0, scaleFixtureProfiles)
	refs := make([]string, 0, scaleFixtureProfiles)
	for index := 0; index < scaleFixtureProfiles; index++ {
		ids = append(ids, "vcp_scale_"+pad(index))
		refs = append(refs, "cus_scale_"+pad(index))
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO vendor_customer_profiles
		(id,business_id,subject_type,customer_ref,safe_display_label,status,privacy_state,merge_state,
		 commercial_projection_state)
		SELECT s.id, $1, 'CUSTOMER', s.ref, 'Customer '||s.ref, 'ACTIVE', 'ACTIVE', 'NONE', 'NOT_AVAILABLE'
		FROM unnest($2::text[], $3::text[]) AS s(id, ref)`, businessA, ids, refs); err != nil {
		t.Fatalf("seed relationships: %v", err)
	}
	// Half of them have an authoritative marketing ALLOW, so the per-channel
	// figures have to be computed rather than being uniformly one value.
	consentIDs := make([]string, 0, scaleFixtureProfiles/2)
	consentProfiles := make([]string, 0, scaleFixtureProfiles/2)
	for index := 0; index < scaleFixtureProfiles; index += 2 {
		consentIDs = append(consentIDs, "crmc_scale_"+pad(index))
		consentProfiles = append(consentProfiles, ids[index])
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO crm_consent_projections
		(id,business_id,profile_id,purpose,channel,decision,source_service,source_version,observed_at)
		SELECT c.id, $1, c.profile_id, 'MARKETING', 'EMAIL', 'ALLOW', 'customer-service', 1, $2
		FROM unnest($3::text[], $4::text[]) AS c(id, profile_id)`,
		businessA, env.now, consentIDs, consentProfiles); err != nil {
		t.Fatalf("seed consent: %v", err)
	}

	source = env.staticAudience(t, businessA, "vs_a", "Scale source", ids...)
	exclusion = env.staticAudience(t, businessA, "vs_a", "Scale exclusion", ids[:scaleExcluded]...)
	return source, exclusion, ids
}

// TestCampaignScale_PreparationBatchesPaginatesAndReconciles is the performance
// and correctness check at size. It proves the batch boundary is handled, the
// funnel still reconciles over a thousand candidates, and paging the target
// list returns every row exactly once.
func TestCampaignScale_PreparationBatchesPaginatesAndReconciles(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	source, exclusion, _ := seedScaleFixture(t, env)
	definition := campaignInput("Scale campaign", source.ID)
	definition.ExclusionSegmentRefs = []string{exclusion.ID}
	yes := true
	definition.ExperimentEnabled = &yes
	definition.Variants = []ExperimentVariantInput{
		{VariantKey: "CONTROL", DisplayName: "Holdout", AllocationBps: 1000, IsControl: true},
		{VariantKey: "A", DisplayName: "Treatment A", AllocationBps: 4500},
		{VariantKey: "B", DisplayName: "Treatment B", AllocationBps: 4500},
	}
	campaign := env.createCampaign(t, businessA, "vs_a", definition)

	// Two more campaigns exist so the list read is not trivially a single row.
	env.createCampaign(t, businessA, "vs_a", campaignInput("Scale campaign two", source.ID))
	env.createCampaign(t, businessA, "vs_a", campaignInput("Scale campaign three", source.ID))

	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	if snapshot.Status != SnapshotCurrent {
		t.Fatalf("expected CURRENT, got %s (error=%v)", snapshot.Status, snapshot.ErrorCode)
	}
	assertFunnelReconciles(t, snapshot)
	if snapshot.SourceCount != scaleFixtureProfiles {
		t.Fatalf("expected %d source rows, got %d", scaleFixtureProfiles, snapshot.SourceCount)
	}
	if snapshot.SegmentExclusionCount != scaleExcluded {
		t.Fatalf("expected %d exclusions, got %d", scaleExcluded, snapshot.SegmentExclusionCount)
	}
	if snapshot.TargetCount != scaleFixtureProfiles-scaleExcluded {
		t.Fatalf("expected %d targets, got %d", scaleFixtureProfiles-scaleExcluded, snapshot.TargetCount)
	}

	// Every target has exactly one channel row, so the write path did not skip
	// or duplicate at a batch boundary.
	var channelRows int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_campaign_target_channels
		WHERE business_id=$1 AND snapshot_id=$2`, businessA, snapshot.ID).Scan(&channelRows); err != nil {
		t.Fatal(err)
	}
	if channelRows != snapshot.TargetCount {
		t.Fatalf("expected one channel row per target (%d), got %d", snapshot.TargetCount, channelRows)
	}

	// Paging the target list returns every row exactly once. An off-by-one in
	// the has-more boundary would show up here as a duplicate or a gap.
	seen := map[string]bool{}
	offset := 0
	for page := 0; page < 20; page++ {
		targets, hasMore, err := env.service.ListSnapshotTargets(ctx, actor, globalScope(), campaign.ID, snapshot.ID,
			TargetListOptions{Limit: MaxPageSize, Offset: offset})
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		for _, target := range targets {
			if seen[target.ProfileID] {
				t.Fatalf("relationship %s appeared on more than one page", target.ProfileID)
			}
			seen[target.ProfileID] = true
		}
		offset += len(targets)
		if !hasMore {
			break
		}
	}
	if len(seen) != snapshot.TargetCount {
		t.Fatalf("paging returned %d of %d targets", len(seen), snapshot.TargetCount)
	}

	// The per-channel figures are derived from the stored evidence and must
	// match it exactly at this size too.
	counts, err := env.service.SnapshotChannelCounts(ctx, businessA, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 1 {
		t.Fatalf("expected one channel projection, got %d", len(counts))
	}
	if counts[0].EligibleCount+counts[0].IneligibleCount != snapshot.TargetCount {
		t.Fatalf("channel figures (%d + %d) do not account for %d targets",
			counts[0].EligibleCount, counts[0].IneligibleCount, snapshot.TargetCount)
	}
	if counts[0].EligibleCount == 0 || counts[0].EligibleCount == snapshot.TargetCount {
		t.Fatalf("expected a genuine mix of eligible and ineligible, got %d of %d",
			counts[0].EligibleCount, snapshot.TargetCount)
	}

	// Cohorts partition the audience and every one of the three is populated,
	// so the deterministic allocator really does spread a realistic audience.
	perVariant := map[string]int{}
	rows, err := env.pool.Query(ctx, `SELECT experiment_variant_key, count(*) FROM crm_campaign_targets
		WHERE business_id=$1 AND snapshot_id=$2 GROUP BY experiment_variant_key`, businessA, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key *string
		var count int
		if err = rows.Scan(&key, &count); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		perVariant[derefOr(key)] = count
	}
	rows.Close()
	for _, key := range []string{"CONTROL", "A", "B"} {
		if perVariant[key] == 0 {
			t.Fatalf("cohort %s is empty over %d targets: %v", key, snapshot.TargetCount, perVariant)
		}
	}
}

// TestCampaignScale_HotQueriesAreIndexCovered checks that each hot access path
// has an index that actually covers it.
//
// It deliberately does not assert which plan PostgreSQL picks. At fixture size
// a sequential scan over a thousand rows is often the correct choice, and a
// test that failed on it would be asserting a bad plan. What matters — and what
// stays true on a tenant with a million relationships — is that an index exists
// which serves the query shape, so the planner has the option. Disabling
// sequential scans is how that is asked without pretending to know the
// production statistics.
func TestCampaignScale_HotQueriesAreIndexCovered(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	source, _, _ := seedScaleFixture(t, env)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Plan check", source.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	for _, table := range []string{"crm_campaigns", "crm_campaign_targets", "crm_campaign_target_channels",
		"crm_campaign_audience_snapshots"} {
		if _, err := env.pool.Exec(ctx, "ANALYZE "+table); err != nil {
			t.Fatalf("analyze %s: %v", table, err)
		}
	}

	plans := []struct {
		name  string
		sql   string
		args  []any
		index string
	}{
		{"target listing", `SELECT id FROM crm_campaign_targets
			WHERE business_id=$1 AND snapshot_id=$2 ORDER BY customer_profile_id LIMIT 50`,
			[]any{businessA, snapshot.ID}, "crm_campaign_targets_snapshot_idx"},
		{"channel eligibility", `SELECT count(*) FROM crm_campaign_target_channels
			WHERE business_id=$1 AND snapshot_id=$2 AND channel='EMAIL'
			  AND snapshot_eligibility_state='ELIGIBLE_AT_SNAPSHOT'`,
			[]any{businessA, snapshot.ID}, "crm_campaign_target_channels_eligibility_idx"},
		{"campaign listing", `SELECT id FROM crm_campaigns
			WHERE business_id=$1 AND status='PREPARED' ORDER BY updated_at DESC LIMIT 50`,
			[]any{businessA}, "crm_campaigns_status_idx"},
		{"snapshot history", `SELECT id FROM crm_campaign_audience_snapshots
			WHERE business_id=$1 AND campaign_id=$2 ORDER BY created_at DESC LIMIT 50`,
			[]any{businessA, campaign.ID}, "crm_campaign_snapshots_campaign_idx"},
		{"generation retention", `SELECT 1 FROM crm_campaign_audience_snapshots
			WHERE business_id=$1 AND source_segment_id=$2 AND source_segment_generation=1
			  AND status IN ('BUILDING','CURRENT')`,
			[]any{businessA, source.ID}, "crm_campaign_snapshots_retention_idx"},
		{"worker claim", `SELECT id FROM crm_campaign_audience_snapshots
			WHERE status='BUILDING' AND next_attempt_at <= now() ORDER BY next_attempt_at LIMIT 1`,
			nil, "crm_campaign_snapshots_due_idx"},
	}
	for _, plan := range plans {
		t.Run(plan.name, func(t *testing.T) {
			tx, err := env.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if _, err = tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
				t.Fatal(err)
			}
			rows, err := tx.Query(ctx, "EXPLAIN "+plan.sql, plan.args...)
			if err != nil {
				t.Fatalf("explain: %v", err)
			}
			var explained strings.Builder
			for rows.Next() {
				var line string
				if err = rows.Scan(&line); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				explained.WriteString(line)
				explained.WriteString("\n")
			}
			rows.Close()
			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}
			// With sequential scans disabled, PostgreSQL still falls back to one
			// when no index can serve the query at all — so a Seq Scan here
			// means the access path is genuinely uncovered. Which of the
			// covering indexes the planner prefers is its business, not this
			// test's: at fixture size it may reasonably choose a different one
			// from the intended index, and asserting otherwise would pin a plan
			// rather than a property.
			if strings.Contains(explained.String(), "Seq Scan") {
				t.Fatalf("%s has no covering index (expected something like %s):\n%s",
					plan.name, plan.index, explained.String())
			}
		})
	}
}
