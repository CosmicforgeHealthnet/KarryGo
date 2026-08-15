package crm

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Section 8H segmentation, analytics, branch-scope and security tests.

// TestIntelligenceRegistry_AddsExactlyEightFields pins the registry growth.
// The final completion fields are present in both baselines; intelligence adds
// exactly eight fields on top of them.
func TestIntelligenceRegistry_AddsExactlyEightFields(t *testing.T) {
	withIntelligence := NewSegmentFieldRegistry(allFlagsOn())
	off := allFlagsOn()
	off.IntelligenceEnabled = false
	without := NewSegmentFieldRegistry(off)

	if len(without.Fields()) != 32 {
		t.Fatalf("the accepted Section 8 completion registry holds %d fields; the baseline is 32", len(without.Fields()))
	}
	if len(withIntelligence.Fields()) != 40 {
		t.Fatalf("the registry holds %d fields with intelligence on; want exactly 40", len(withIntelligence.Fields()))
	}
	want := []string{
		"intelligence.rfm.state", "intelligence.rfm.recency_score", "intelligence.rfm.frequency_score",
		"intelligence.rfm.monetary_score", "intelligence.rfm.composite_score",
		"intelligence.churn.state", "intelligence.churn.risk_score", "intelligence.churn.risk_band",
	}
	for _, key := range want {
		field, ok := withIntelligence.Field(key)
		if !ok {
			t.Fatalf("field %s is not registered", key)
		}
		if field.Source != SourceIntelligence {
			t.Errorf("field %s has source %s", key, field.Source)
		}
		if _, present := without.Field(key); present {
			t.Errorf("field %s is registered while intelligence is disabled", key)
		}
	}
	// No field invites a rule to assert a score.
	for _, field := range withIntelligence.Fields() {
		if field.Source != SourceIntelligence {
			continue
		}
		for _, operator := range field.Operators {
			if operator == "SET" || operator == "ASSIGN" {
				t.Errorf("field %s exposes a mutating operator %s", field.Key, operator)
			}
		}
	}
}

func (e *testEnv) intelligenceSegment(t *testing.T, business, staff, name, rule string) Segment {
	t.Helper()
	segment, err := e.service.CreateSegment(context.Background(), actorFor(business, staff), globalScope(),
		CreateSegmentInput{Name: name, SegmentType: SegmentDynamic, ScopeType: ScopeBusiness,
			Rule: ruleJSON(rule)})
	if err != nil {
		t.Fatalf("create segment %s: %v", name, err)
	}
	activated, err := e.service.SetSegmentStatus(context.Background(), actorFor(business, staff),
		globalScope(), segment.ID, SegmentActive, segment.Version)
	if err != nil {
		t.Fatalf("activate segment %s: %v", name, err)
	}
	return activated
}

func (e *testEnv) segmentMembers(t *testing.T, business, segmentID string) []string {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), `SELECT m.profile_id
		FROM crm_segment_memberships m JOIN crm_segments g
		  ON g.business_id=m.business_id AND g.id=m.segment_id
		 AND g.current_evaluation_id IS NOT DISTINCT FROM m.evaluation_id
		WHERE m.business_id=$1 AND m.segment_id=$2 ORDER BY m.profile_id`, business, segmentID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	return out
}

// TestIntelligenceSegments_UnavailableScoresNeverSatisfyANumericPredicate is
// the trap this section exists to avoid: a relationship that was never scored
// must not match `composite_score LT 5` because a default of zero says so.
func TestIntelligenceSegments_UnavailableScoresNeverSatisfyANumericPredicate(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	scored := map[string]Profile{}
	for index := 0; index < 6; index++ {
		ref := fmt.Sprintf("cus_seg%d", index)
		scored[ref] = env.scoredCustomer(t, businessA, "vs_a", ref, 3+index*3, int64(50000*(index+1)),
			300*24*time.Hour, time.Duration(index+1)*15*24*time.Hour)
	}
	unscored := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_seg_unscored"))
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
	env.runIntelligence(t, businessA, "vs_a", ModelChurn, IntelligenceScopeBusiness, "")

	// "Weak relationships" — the query an operator would actually write.
	weak := env.intelligenceSegment(t, businessA, "vs_a", "Weak RFM", `{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"intelligence.rfm.composite_score","operator":"LTE","value":6}]}}`)
	env.materialize(t)
	for _, member := range env.segmentMembers(t, businessA, weak.ID) {
		if member == unscored.ID {
			t.Fatal("a relationship with no score matched a low-composite rule; " +
				"unavailable must never read as a low number")
		}
	}

	// The same for a churn threshold, which is the more dangerous direction:
	// an unscored customer must not appear on a high-risk follow-up list.
	risky := env.intelligenceSegment(t, businessA, "vs_a", "High churn", `{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"intelligence.churn.risk_score","operator":"GT","value":0}]}}`)
	env.materialize(t)
	for _, member := range env.segmentMembers(t, businessA, risky.ID) {
		if member == unscored.ID {
			t.Fatal("a relationship with no score matched a churn-risk rule")
		}
	}

	// A rule can still deliberately ask about availability, and that one does
	// match the unscored relationship — the distinction is the whole point.
	missing := env.intelligenceSegment(t, businessA, "vs_a", "No RFM", `{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"intelligence.rfm.state","operator":"EQ","value":"NOT_AVAILABLE"}]}}`)
	env.materialize(t)
	found := false
	for _, member := range env.segmentMembers(t, businessA, missing.ID) {
		if member == unscored.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("an availability rule did not match the relationship it describes")
	}
}

// TestIntelligenceSegments_ScoreChangeRestakesOnlyDependentSegments is
// dependency-scoped invalidation. A churn rebuild must not restake an RFM-only
// audience, or a busy tenant would rematerialize everything constantly.
func TestIntelligenceSegments_ScoreChangeRestakesOnlyDependentSegments(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	for index := 0; index < 6; index++ {
		env.scoredCustomer(t, businessA, "vs_a", fmt.Sprintf("cus_dep%d", index), 3+index, 100000,
			300*24*time.Hour, time.Duration(index+1)*15*24*time.Hour)
	}
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
	env.runIntelligence(t, businessA, "vs_a", ModelChurn, IntelligenceScopeBusiness, "")

	rfmOnly := env.intelligenceSegment(t, businessA, "vs_a", "RFM only", `{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"intelligence.rfm.composite_score","operator":"GTE","value":3}]}}`)
	statusOnly := env.intelligenceSegment(t, businessA, "vs_a", "Active only", `{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"profile.status","operator":"EQ","value":"ACTIVE"}]}}`)
	env.materialize(t)

	statusOf := func(segmentID string) string {
		var status string
		if err := env.pool.QueryRow(ctx, `SELECT evaluation_status FROM crm_segments
			WHERE business_id=$1 AND id=$2`, businessA, segmentID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		return status
	}
	if statusOf(rfmOnly.ID) != EvalCurrent || statusOf(statusOnly.ID) != EvalCurrent {
		t.Fatalf("segments did not materialize: rfm=%s status=%s",
			statusOf(rfmOnly.ID), statusOf(statusOnly.ID))
	}

	// A churn-only rebuild restakes neither of these, because neither reads a
	// churn field.
	env.runIntelligence(t, businessA, "vs_a", ModelChurn, IntelligenceScopeBusiness, "")
	if statusOf(rfmOnly.ID) != EvalCurrent {
		t.Fatalf("a churn rebuild restaked an RFM-only segment (status=%s)", statusOf(rfmOnly.ID))
	}

	// An RFM rebuild restakes the RFM segment and leaves the unrelated one
	// alone.
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
	if statusOf(rfmOnly.ID) != EvalStale {
		t.Fatalf("an RFM rebuild left its dependent segment %s; want STALE", statusOf(rfmOnly.ID))
	}
	if statusOf(statusOnly.ID) != EvalCurrent {
		t.Fatalf("an RFM rebuild restaked an unrelated segment (status=%s)", statusOf(statusOnly.ID))
	}
}

// TestIntelligenceSegments_HighChurnDoesNotBypassConsent is the invariant the
// whole section is subordinate to: a score is not permission.
func TestIntelligenceSegments_HighChurnDoesNotBypassConsent(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	var lapsed Profile
	for index := 0; index < 6; index++ {
		profile := env.scoredCustomer(t, businessA, "vs_a", fmt.Sprintf("cus_consent%d", index),
			12, 100000, 500*24*time.Hour, time.Duration(150+index*10)*24*time.Hour)
		if index == 0 {
			lapsed = profile
		}
	}
	env.runIntelligence(t, businessA, "vs_a", ModelChurn, IntelligenceScopeBusiness, "")

	score, ok := env.scoreOf(t, businessA, lapsed.ID, ModelChurn, IntelligenceScopeBusiness, "")
	if !ok || score.State != ScoreCurrent || *score.RiskScore == 0 {
		t.Fatalf("the subject is not a scored risk: %+v", score)
	}

	// The customer explicitly refuses marketing.
	if err := env.applyConsent(t, lapsed.ID,
		consentInput(ChannelEmail, ConsentDeny, 1, env.now)); err != nil {
		t.Fatalf("apply consent: %v", err)
	}

	// A rule that combines a high risk score with marketing eligibility selects
	// nobody who said no. The score raised the customer's priority; it did not
	// grant permission to contact them.
	targetable := env.intelligenceSegment(t, businessA, "vs_a", "At-risk and contactable", `{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"intelligence.churn.risk_score","operator":"GTE","value":1},
		{"field":"consent.marketing.email.eligible","operator":"EQ","value":true}]}}`)
	env.materialize(t)
	for _, member := range env.segmentMembers(t, businessA, targetable.ID) {
		if member == lapsed.ID {
			t.Fatal("a customer who denied marketing consent was selected because their churn score was high; " +
				"a score must never be permission to contact")
		}
	}

	// And the consent projection itself is unchanged by scoring.
	summary, err := env.service.ConsentSummaryFor(ctx, actorFor(businessA, "vs_a"), lapsed)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Marketing[ChannelEmail].Eligible {
		t.Fatal("scoring changed a consent decision")
	}
}

// TestIntelligence_BranchScopeNeverLeaksTheBusinessBook is the security
// property that decides whether branch intelligence can be shown at all.
func TestIntelligence_BranchScopeNeverLeaksTheBusinessBook(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	for index := 0; index < 6; index++ {
		env.scoredCustomer(t, businessA, "vs_a", fmt.Sprintf("cus_branch%d", index), 3+index, 100000,
			300*24*time.Hour, time.Duration(index+1)*15*24*time.Hour)
	}
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")

	branchScope := Scope{BranchIDs: []string{branchA1}}
	actor := actorFor(businessA, "vs_a")

	// A branch actor cannot ask for the business scope.
	if _, err := env.service.IntelligenceOverviewFor(ctx, actor, branchScope, ""); err == nil {
		// It resolves to their own branch instead, which must not carry the
		// business figures.
		overview, _ := env.service.IntelligenceOverviewFor(ctx, actor, branchScope, "")
		if overview.Scope != IntelligenceScopeBranch {
			t.Fatalf("a branch actor received %s-scope intelligence", overview.Scope)
		}
		for _, model := range overview.Models {
			if model.Status == IntelligenceScopeCurrent {
				t.Fatalf("a branch actor was shown CURRENT business-derived figures: %+v", model)
			}
		}
	}
	// Explicitly naming another branch is refused.
	if _, err := env.service.IntelligenceOverviewFor(ctx, actor, branchScope, branchA2); err == nil {
		t.Fatal("a branch actor read another branch's intelligence")
	}
	// And a rebuild for a branch they do not hold is refused.
	if _, err := env.service.RequestIntelligenceRebuild(ctx, actor, branchScope,
		RebuildIntelligenceInput{ModelKey: ModelRFM, ScopeType: IntelligenceScopeBranch,
			BranchRef: branchA2}); err == nil {
		t.Fatal("a branch actor rebuilt another branch's intelligence")
	}
	// A business-scope rebuild is refused outright: a branch actor must not be
	// able to cause, or read, a figure covering branches they cannot see.
	if _, err := env.service.RequestIntelligenceRebuild(ctx, actor, branchScope,
		RebuildIntelligenceInput{ModelKey: ModelRFM, ScopeType: IntelligenceScopeBusiness}); err == nil {
		t.Fatal("a branch actor ordered a business-wide rebuild")
	}

	// Their own branch works, and produces branch-shaped figures.
	if _, err := env.service.RequestIntelligenceRebuild(ctx, actor, branchScope,
		RebuildIntelligenceInput{ModelKey: ModelRFM, ScopeType: IntelligenceScopeBranch,
			BranchRef: branchA1}); err != nil {
		t.Fatalf("a branch actor could not rebuild their own branch: %v", err)
	}
	if _, err := env.service.RunIntelligenceMaterializer(ctx, "test-worker", 5); err != nil {
		t.Fatal(err)
	}
	overview, err := env.service.IntelligenceOverviewFor(ctx, actor, branchScope, branchA1)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Scope != IntelligenceScopeBranch || overview.BranchRef == nil || *overview.BranchRef != branchA1 {
		t.Fatalf("branch overview is not branch-shaped: %+v", overview)
	}
}

// TestIntelligence_TenantIsolation proves one business cannot see, rebuild or
// infer another's intelligence.
func TestIntelligence_TenantIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	for index := 0; index < 6; index++ {
		env.scoredCustomer(t, businessA, "vs_a", fmt.Sprintf("cus_iso%d", index), 3+index, 100000,
			300*24*time.Hour, time.Duration(index+1)*15*24*time.Hour)
	}
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")

	// Business B sees nothing, not an error revealing that A has data.
	overview, err := env.service.IntelligenceOverviewFor(ctx, actorFor(businessB, "vs_b"),
		globalScope(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range overview.Models {
		if model.Status != IntelligenceScopeNotAvailable || model.ProfileCount != 0 {
			t.Fatalf("business B sees business A's intelligence: %+v", model)
		}
	}
	distribution, err := env.service.RFMDistributionFor(ctx, actorFor(businessB, "vs_b"), globalScope(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(distribution.Composite) != 0 {
		t.Fatalf("business B read business A's distribution: %+v", distribution.Composite)
	}
	// B naming A's branch is refused rather than answered.
	if _, err = env.service.IntelligenceOverviewFor(ctx, actorFor(businessB, "vs_b"),
		Scope{BranchIDs: []string{branchB1}}, branchA1); err == nil {
		t.Fatal("business B read intelligence for business A's branch")
	}
}

// TestIntelligenceAnalytics_ReportsUnavailabilityRatherThanZero is the
// analytics half of the missing-data invariant.
func TestIntelligenceAnalytics_ReportsUnavailabilityRatherThanZero(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	// A tenant with relationships but no commercial facts at all — the real
	// state of this platform today.
	for index := 0; index < 5; index++ {
		env.seedProfile(t, businessA, "vs_a", customerSubject(fmt.Sprintf("cus_blind%d", index)))
	}
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")

	distribution, err := env.service.RFMDistributionFor(ctx, actor, globalScope(), "")
	if err != nil {
		t.Fatal(err)
	}
	if distribution.Availability.ProfileCount != 5 {
		t.Fatalf("profile count = %d; the relationships exist and must be counted",
			distribution.Availability.ProfileCount)
	}
	if distribution.Availability.AvailableCount != 0 || distribution.Availability.UnavailableCount != 5 {
		t.Fatalf("availability %d/%d; want 0 scored and 5 unavailable",
			distribution.Availability.AvailableCount, distribution.Availability.UnavailableCount)
	}
	if len(distribution.Composite) != 0 {
		t.Fatalf("a distribution was reported over nothing: %+v", distribution.Composite)
	}
	// The reasons say why, which is what makes the empty dashboard actionable
	// rather than mysterious.
	found := false
	for _, reason := range distribution.Unavailable {
		if reason.Key == ReasonSourceNotAvailable && reason.Count == 5 {
			found = true
		}
	}
	if !found {
		t.Fatalf("unavailable reasons did not explain the gap: %+v", distribution.Unavailable)
	}

	// With real facts, the distribution appears and sums to the scored cohort.
	for index := 0; index < 6; index++ {
		env.scoredCustomer(t, businessA, "vs_a", fmt.Sprintf("cus_dist%d", index), 3+index, 100000,
			300*24*time.Hour, time.Duration(index+1)*15*24*time.Hour)
	}
	env.runIntelligence(t, businessA, "vs_a", ModelRFM, IntelligenceScopeBusiness, "")
	env.runIntelligence(t, businessA, "vs_a", ModelChurn, IntelligenceScopeBusiness, "")

	distribution, err = env.service.RFMDistributionFor(ctx, actor, globalScope(), "")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, bucket := range distribution.Composite {
		total += bucket.Count
	}
	if total != distribution.Availability.AvailableCount {
		t.Fatalf("the composite histogram sums to %d but %d relationships are scored",
			total, distribution.Availability.AvailableCount)
	}
	churn, err := env.service.ChurnDistributionFor(ctx, actor, globalScope(), "")
	if err != nil {
		t.Fatal(err)
	}
	bands := 0
	for _, bucket := range churn.Bands {
		switch bucket.Key {
		case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		default:
			t.Fatalf("churn distribution contains an unknown band %q", bucket.Key)
		}
		bands += bucket.Count
	}
	if bands != churn.Availability.AvailableCount {
		t.Fatalf("the band histogram sums to %d but %d relationships are scored",
			bands, churn.Availability.AvailableCount)
	}
}
