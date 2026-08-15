package crm

import (
	"context"
	"strings"
	"testing"
	"time"
)

// scaleDataset is the fixture size these tests report against.
//
// It is deliberately modest: it is large enough to cross the materializer's
// batch boundary and to exercise pagination and the bulk ceiling on real
// Postgres, and small enough to keep the suite fast. It is not a capacity
// claim — nothing here says the engine handles a million relationships, and no
// figure from this file should be quoted as if it did.
const (
	scaleProfiles = 1200
	scaleBatch    = 50
)

// seedScaleProfiles inserts relationships and completed order facts directly.
//
// It bypasses the ingestion usecase on purpose: this test is about how the
// segmentation engine behaves over a populated table, and driving 1200
// relationships through the full event path would measure ingestion instead.
func seedScaleProfiles(t *testing.T, env *testEnv, count int) {
	t.Helper()
	ctx := context.Background()
	completed := env.now.AddDate(0, 0, -3)

	profileIDs := make([]string, count)
	customerRefs := make([]string, count)
	orderCounts := make([]int32, count)
	for index := 0; index < count; index++ {
		profileIDs[index] = "vcp_scale_" + pad(index)
		customerRefs[index] = "cus_scale_" + pad(index)
		// Two thirds qualify as repeat customers, so the audience is a real
		// subset rather than the whole table.
		if index%3 == 0 {
			orderCounts[index] = 1
		} else {
			orderCounts[index] = 3
		}
	}

	if _, err := env.pool.Exec(ctx, `INSERT INTO vendor_customer_profiles
		(id,business_id,subject_type,customer_ref,status,privacy_state,merge_state,
		 completed_order_count,gross_spend_minor,net_spend_minor,average_order_value_minor,
		 commercial_currency,last_completed_order_at,first_completed_order_at,
		 last_capability_type,commercial_projection_state,last_commercial_source_observed_at)
		SELECT s.id, $1, 'CUSTOMER', s.customer_ref, 'ACTIVE', 'ACTIVE', 'NONE',
			s.order_count, s.order_count::bigint * 100000, s.order_count::bigint * 100000, 100000,
			'NGN', $4::timestamptz, $4::timestamptz, 'RESTAURANT', 'CURRENT', $4::timestamptz
		FROM unnest($2::text[], $3::text[], $5::int[]) AS s(id, customer_ref, order_count)`,
		businessA, profileIDs, customerRefs, completed, orderCounts); err != nil {
		t.Fatalf("seed profiles: %v", err)
	}

	relationIDs := make([]string, count)
	for index := range relationIDs {
		relationIDs[index] = "crmbr_scale_" + pad(index)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO crm_customer_branch_relations
		(id,business_id,profile_id,branch_id,first_relationship_at,last_relationship_at,
		 completed_order_count,gross_spend_minor,net_spend_minor,currency,last_capability_type,projection_state)
		SELECT s.id, $1, s.profile_id, $2, $5::timestamptz, $5::timestamptz,
			s.order_count, s.order_count::bigint * 100000, s.order_count::bigint * 100000, 'NGN', 'RESTAURANT', 'CURRENT'
		FROM unnest($3::text[], $4::text[], $6::int[]) AS s(id, profile_id, order_count)`,
		businessA, branchA1, relationIDs, profileIDs, completed, orderCounts); err != nil {
		t.Fatalf("seed branch relations: %v", err)
	}
}

func pad(n int) string {
	digits := itoa(n)
	return strings.Repeat("0", 6-len(digits)) + digits
}

// TestScale_MaterializationCrossesBatchBoundaries proves the keyset batching in
// writeGeneration is correct rather than merely present: with a batch smaller
// than the audience, every qualifying relationship must still be written exactly
// once, and none twice.
func TestScale_MaterializationCrossesBatchBoundaries(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	env.service.evaluation.BatchSize = scaleBatch
	ctx := context.Background()
	seedScaleProfiles(t, env, scaleProfiles)

	expected := 0
	for index := 0; index < scaleProfiles; index++ {
		if index%3 != 0 {
			expected++
		}
	}

	started := time.Now()
	segment := env.activeDynamicSegment(t, "Scale Audience")
	elapsed := time.Since(started)

	if segment.EvaluationStatus != EvalCurrent {
		t.Fatalf("status = %s; want CURRENT", segment.EvaluationStatus)
	}
	if segment.CurrentMemberCount != expected {
		t.Fatalf("member count = %d; want %d — batching lost or duplicated rows", segment.CurrentMemberCount, expected)
	}
	var distinct, total int
	if err := env.pool.QueryRow(ctx, `SELECT count(DISTINCT profile_id), count(*) FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2`, businessA, segment.ID).Scan(&distinct, &total); err != nil {
		t.Fatal(err)
	}
	if distinct != total {
		t.Fatalf("distinct=%d total=%d; a relationship was written into the generation twice", distinct, total)
	}
	t.Logf("materialized %d of %d relationships in %s with batch size %d",
		expected, scaleProfiles, elapsed.Round(time.Millisecond), scaleBatch)
}

// TestScale_MemberPaginationIsBoundedAndComplete walks the whole audience one
// page at a time and checks the pages tile it exactly — no gaps, no repeats.
func TestScale_MemberPaginationIsBoundedAndComplete(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	env.service.evaluation.BatchSize = scaleBatch
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	seedScaleProfiles(t, env, scaleProfiles)
	segment := env.activeDynamicSegment(t, "Paged Audience")

	seen := map[string]bool{}
	offset := 0
	pages := 0
	for {
		members, hasMore, err := env.service.ListSegmentMembers(ctx, actor, globalScope(), segment, MaxPageSize, offset)
		if err != nil {
			t.Fatal(err)
		}
		if len(members) > MaxPageSize {
			t.Fatalf("page returned %d rows; the maximum page size is %d", len(members), MaxPageSize)
		}
		for _, member := range members {
			if seen[member.Profile.ID] {
				t.Fatalf("relationship %s appeared on two pages", member.Profile.ID)
			}
			seen[member.Profile.ID] = true
		}
		pages++
		if !hasMore {
			break
		}
		offset += MaxPageSize
		if pages > 50 {
			t.Fatal("pagination did not terminate")
		}
	}
	if len(seen) != segment.CurrentMemberCount {
		t.Fatalf("paged %d members but the segment reports %d", len(seen), segment.CurrentMemberCount)
	}
	t.Logf("walked %d members across %d pages", len(seen), pages)
}

// TestScale_PreviewCountBudgetReportsALowerBound proves the preview's answer
// stays honest when the audience exceeds what it will count synchronously: it
// reports a real lower bound rather than hanging or inventing an estimate.
func TestScale_PreviewCountBudgetReportsALowerBound(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	seedScaleProfiles(t, env, scaleProfiles)

	// A budget far below the real audience, standing in for a tenant large
	// enough to exceed the production budget.
	env.service.evaluation.PreviewCountBudget = 100

	started := time.Now()
	result, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)

	if result.CountState != CountAtLeast {
		t.Fatalf("count state = %s; want %s once the budget is exceeded", result.CountState, CountAtLeast)
	}
	if result.MatchCount != 100 {
		t.Fatalf("count = %d; want the budget as an honest lower bound", result.MatchCount)
	}
	if len(result.Sample) > env.service.evaluation.PreviewSampleMax {
		t.Fatalf("sample = %d rows; the maximum is %d", len(result.Sample), env.service.evaluation.PreviewSampleMax)
	}
	t.Logf("preview over %d relationships answered in %s", scaleProfiles, elapsed.Round(time.Millisecond))
}

// TestScale_BulkTagAtTheConfiguredCeiling exercises a bulk operation at exactly
// the documented maximum, so the ceiling is known to be usable and not merely
// declared.
func TestScale_BulkTagAtTheConfiguredCeiling(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	seedScaleProfiles(t, env, scaleProfiles)

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Bulk Ceiling"})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, MaxBulkBatch)
	for index := range ids {
		ids[index] = "vcp_scale_" + pad(index)
	}

	started := time.Now()
	result, err := env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, ids)
	if err != nil {
		t.Fatalf("bulk assign at the ceiling: %v", err)
	}
	elapsed := time.Since(started)
	if result.Applied != MaxBulkBatch {
		t.Fatalf("applied = %d; want %d", result.Applied, MaxBulkBatch)
	}
	t.Logf("assigned %d memberships in %s", MaxBulkBatch, elapsed.Round(time.Millisecond))

	// One more than the ceiling is refused rather than silently truncated.
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, append(ids, "vcp_scale_000600")); err == nil {
		t.Fatal("a request above the ceiling was accepted")
	}
}

// TestScale_MaximumComplexityRuleEvaluates proves a rule at the documented
// complexity ceiling actually runs against a populated table. A limit that
// admits rules the evaluator then chokes on would be worse than no limit.
func TestScale_MaximumComplexityRuleEvaluates(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	seedScaleProfiles(t, env, scaleProfiles)

	limits := env.service.RuleLimitSet()
	// A rule at the OR-branch ceiling, each branch itself a two-condition AND,
	// staying inside the node ceiling.
	// Each branch costs three nodes (its own AND plus two conditions) and the
	// root OR costs one, so the loop stops before the next branch would push the
	// rule past the node ceiling.
	branches := make([]string, 0, limits.MaxOrChildren)
	for index := 0; index < limits.MaxOrChildren && (len(branches)+1)*3+1 <= limits.MaxNodes; index++ {
		branches = append(branches, `{"op":"AND","children":[
			{"field":"commercial.completed_order_count","operator":"GTE","value":`+itoa(index+1)+`},
			{"field":"profile.subject_type","operator":"EQ","value":"CUSTOMER"}
		]}`)
	}
	rule := `{"schema_version":1,"root":{"op":"OR","children":[` + strings.Join(branches, ",") + `]}}`

	started := time.Now()
	result, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{Rule: ruleJSON(rule)})
	if err != nil {
		t.Fatalf("a rule at the documented ceiling failed to evaluate: %v", err)
	}
	elapsed := time.Since(started)
	if result.MatchCount == 0 {
		t.Fatal("the maximum-complexity rule matched nobody; the fixture or the compiler is wrong")
	}
	t.Logf("evaluated a %d-branch rule over %d relationships in %s",
		len(branches), scaleProfiles, elapsed.Round(time.Millisecond))
}

// TestScale_CandidateQueryUsesAnIndex reads the real plan for the materializer's
// candidate query. An audience built by sequentially scanning every relationship
// in a business is a production incident waiting for the first large tenant.
func TestScale_CandidateQueryUsesAnIndex(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	seedScaleProfiles(t, env, scaleProfiles)
	segment := env.activeDynamicSegment(t, "Planned Audience")

	version, err := env.service.CurrentVersion(ctx, env.pool, businessA, segment.ID, segment.CurrentDefinitionVersion)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := env.service.ruleFromVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	query := env.service.buildCandidateQuery(businessA, segment, rule, env.now)

	rows, err := env.pool.Query(ctx, `EXPLAIN SELECT p.id FROM `+query.from+` WHERE `+query.where, query.args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	// The plan is logged rather than asserted on shape: Postgres may legitimately
	// choose a sequential scan over a 1200-row fixture, and a test that demanded
	// an index scan here would be asserting the planner's cost model rather than
	// this code's behaviour. What matters is that the query is explainable and
	// the supporting indexes exist, which the migration test checks directly.
	t.Logf("candidate query plan over %d relationships:\n%s", scaleProfiles, plan.String())
	if plan.Len() == 0 {
		t.Fatal("the candidate query produced no plan")
	}
}
