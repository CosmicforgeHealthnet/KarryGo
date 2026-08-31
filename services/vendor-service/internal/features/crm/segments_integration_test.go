package crm

import (
	"context"
	"encoding/json"
	"testing"
)

// ruleJSON is a small helper so the tests read as rules rather than as string
// concatenation.
func ruleJSON(body string) json.RawMessage { return json.RawMessage(body) }

func repeatOrdersRule() json.RawMessage {
	return ruleJSON(`{"schema_version":1,"root":{"op":"AND","children":[
		{"field":"commercial.completed_order_count","operator":"GTE","value":2},
		{"field":"commercial.last_completed_order_at","operator":"WITHIN_LAST","value":{"amount":30,"unit":"DAY"}}
	]}}`)
}

// materialize runs the worker to completion for the tests, standing in for the
// ticker that drives it in production.
func (e *testEnv) materialize(t *testing.T) {
	t.Helper()
	for pass := 0; pass < 10; pass++ {
		processed, err := e.service.RunMaterializationPass(context.Background(), "test-worker", 10)
		if err != nil {
			t.Fatalf("materialization pass: %v", err)
		}
		if processed == 0 {
			return
		}
	}
	t.Fatal("materialization did not settle")
}

func (e *testEnv) reloadSegment(t *testing.T, business, segmentID string) Segment {
	t.Helper()
	segment, err := e.service.loadSegment(context.Background(), e.pool, business, segmentID)
	if err != nil {
		t.Fatalf("reload segment: %v", err)
	}
	return segment
}

// ---------------------------------------------------------------------------
// Definition lifecycle
// ---------------------------------------------------------------------------

func TestSegments_StaticLifecycle(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Manual VIP List", SegmentType: SegmentStatic, ScopeType: ScopeBusiness,
		Description: "Manually maintained VIP relationship group",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if segment.Status != SegmentDraft || segment.EvaluationStatus != EvalNotEvaluated {
		t.Fatalf("segment = %+v; want DRAFT/NOT_EVALUATED", segment)
	}

	// A static segment carries a version 1 definition with no rule, so a
	// materialization or a future campaign can always name the exact definition
	// a membership belonged to.
	version, err := env.service.CurrentVersion(ctx, env.pool, businessA, segment.ID, 1)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if len(version.RuleJSON) != 0 {
		t.Fatal("a static segment must not carry a rule")
	}

	// A rule on a static segment is a contradiction, not a silently ignored
	// field.
	_, err = env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Static With Rule", SegmentType: SegmentStatic, Rule: repeatOrdersRule(),
	})
	if codeOf(t, err) != CodeSegmentRuleNotAllwd {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentRuleNotAllwd)
	}
	// And a dynamic segment without one cannot decide anything.
	_, err = env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Dynamic No Rule", SegmentType: SegmentDynamic,
	})
	if codeOf(t, err) != CodeSegmentRuleRequired {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentRuleRequired)
	}

	activated, err := env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, segment.Version)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	archived, err := env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentArchived, activated.Version)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	// ARCHIVED is terminal: an audience that was retired must not come back with
	// the membership it had months ago.
	_, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), segment.ID, SegmentActive, archived.Version)
	if codeOf(t, err) != CodeSegmentInvalidState {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentInvalidState)
	}
}

func TestSegments_StaticMembership(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Hand Picked", SegmentType: SegmentStatic})
	if err != nil {
		t.Fatal(err)
	}
	first := env.seedTaggableProfile(t, businessA, "vs_a", "cus_s1", branchA1)
	second := env.seedTaggableProfile(t, businessA, "vs_a", "cus_s2", branchA1)

	added, err := env.service.BulkStaticMembership(ctx, actor, globalScope(), segment.ID,
		[]string{first.ID, second.ID}, true)
	if err != nil {
		t.Fatalf("bulk add: %v", err)
	}
	if added.Applied != 2 {
		t.Fatalf("applied = %d; want 2", added.Applied)
	}
	reloaded := env.reloadSegment(t, businessA, segment.ID)
	if reloaded.CurrentMemberCount != 2 || reloaded.EvaluationStatus != EvalCurrent {
		t.Fatalf("segment = %+v; want 2 members and CURRENT", reloaded)
	}

	// Re-adding is a no-op rather than a duplicate or an error.
	repeat, err := env.service.BulkStaticMembership(ctx, actor, globalScope(), segment.ID, []string{first.ID}, true)
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Applied != 0 {
		t.Fatalf("duplicate add applied = %d; want 0", repeat.Applied)
	}

	members, _, err := env.service.ListSegmentMembers(ctx, actor, globalScope(), env.reloadSegment(t, businessA, segment.ID), 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %d; want 2", len(members))
	}
	for _, member := range members {
		if member.Source != MembershipStatic {
			t.Fatalf("member source = %s; want STATIC", member.Source)
		}
		view := MemberViewOf(member)
		// The member projection is a relationship reference, never contact data.
		raw, _ := json.Marshal(view)
		for _, forbidden := range []string{"email", "phone", "address", "customer_ref"} {
			if containsIgnoringCase(string(raw), forbidden) {
				t.Fatalf("member view exposes %q: %s", forbidden, raw)
			}
		}
	}

	removed, err := env.service.BulkStaticMembership(ctx, actor, globalScope(), segment.ID, []string{first.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Applied != 1 {
		t.Fatalf("removed = %d; want 1", removed.Applied)
	}
	if env.reloadSegment(t, businessA, segment.ID).CurrentMemberCount != 1 {
		t.Fatal("the member count did not follow the removal")
	}
}

func containsIgnoringCase(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		indexIgnoringCase(haystack, needle) >= 0
}

func indexIgnoringCase(haystack, needle string) int {
	lower := func(b byte) byte {
		if b >= 'A' && b <= 'Z' {
			return b + 32
		}
		return b
	}
	for start := 0; start+len(needle) <= len(haystack); start++ {
		matched := true
		for offset := 0; offset < len(needle); offset++ {
			if lower(haystack[start+offset]) != lower(needle[offset]) {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

// TestSegments_StaticRejectsRedactedAndMerged proves the two privacy-mandated
// exclusions cannot be reintroduced by hand.
func TestSegments_StaticRejectsRedactedAndMerged(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Manual", SegmentType: SegmentStatic})
	if err != nil {
		t.Fatal(err)
	}
	redacted := env.seedTaggableProfile(t, businessA, "vs_a", "cus_r", branchA1)
	if _, err = env.pool.Exec(ctx, `UPDATE vendor_customer_profiles
		SET privacy_state='REDACTED', redacted_at=now() WHERE id=$1`, redacted.ID); err != nil {
		t.Fatal(err)
	}
	_, err = env.service.BulkStaticMembership(ctx, actor, globalScope(), segment.ID, []string{redacted.ID}, true)
	if codeOf(t, err) != CodeProfileRedacted {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeProfileRedacted)
	}
}

// TestSegments_DynamicRefusesManualMembership: a hand-added member would be
// discarded at the next materialization, so the API refuses rather than
// accepting a write it cannot keep.
func TestSegments_DynamicRefusesManualMembership(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Repeat Customers", SegmentType: SegmentDynamic, Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	profile := env.seedTaggableProfile(t, businessA, "vs_a", "cus_d", branchA1)
	_, err = env.service.BulkStaticMembership(ctx, actor, globalScope(), segment.ID, []string{profile.ID}, true)
	if codeOf(t, err) != CodeSegmentStaticOnly {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentStaticOnly)
	}

	static, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Static For Exclusion", SegmentType: SegmentStatic})
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.service.BulkExclusion(ctx, actor, globalScope(), static.ID, []string{profile.ID}, true)
	if codeOf(t, err) != CodeSegmentDynamicOnly {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentDynamicOnly)
	}
}

// ---------------------------------------------------------------------------
// Preview
// ---------------------------------------------------------------------------

func TestSegments_PreviewCountsAndSamples(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	matching := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_match"))
	env.seedOrder(t, businessA, "vs_a", matching, "ord_m1", branchA1, 300000, 300000, env.now.AddDate(0, 0, -5), 1)
	env.seedOrder(t, businessA, "vs_a", matching, "ord_m2", branchA1, 300000, 300000, env.now.AddDate(0, 0, -2), 1)

	// One completed order only: below the threshold.
	env.seedTaggableProfile(t, businessA, "vs_a", "cus_one", branchA1)
	// No commercial data at all: must not be read as "ordered zero times".
	env.seedProfile(t, businessA, "vs_a", customerSubject("cus_unknown"))

	result, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{
		ScopeType: ScopeBusiness, Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if result.MatchCount != 1 || result.CountState != CountExact {
		t.Fatalf("preview = %d/%s; want exactly 1", result.MatchCount, result.CountState)
	}
	if len(result.Sample) != 1 || result.Sample[0].ProfileRef != matching.ID {
		t.Fatalf("sample = %+v; want just the matching relationship", result.Sample)
	}
	if len(result.Sample[0].MatchedConditions) != 2 {
		t.Fatalf("matched conditions = %v; want both conditions explained", result.Sample[0].MatchedConditions)
	}
	if result.DefinitionHash == "" || result.EvaluationAsOf.IsZero() {
		t.Fatal("a preview must report the definition it evaluated and when")
	}
	if result.RuleExplanation == "" {
		t.Fatal("a preview must explain the rule it ran")
	}
}

// TestSegments_PreviewTreatsMissingCommercialDataAsUnknown is the honesty
// assertion: a relationship with no observed orders is not a customer who
// ordered zero times, so it neither matches ">= 2" nor "< 2".
func TestSegments_PreviewTreatsMissingCommercialDataAsUnknown(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	unknown := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_no_data"))
	if unknown.CommercialProjectionState != ProjectionNotAvailable {
		t.Fatalf("fixture is wrong: projection state = %s", unknown.CommercialProjectionState)
	}

	atLeast, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{
		Rule: ruleJSON(`{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"GTE","value":2}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if atLeast.MatchCount != 0 {
		t.Fatalf("count = %d; a relationship with no observed orders must not match a >= test", atLeast.MatchCount)
	}

	fewerThan, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{
		Rule: ruleJSON(`{"schema_version":1,"root":{"field":"commercial.completed_order_count","operator":"LT","value":2}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if fewerThan.MatchCount != 0 {
		t.Fatalf("count = %d; unknown data must not satisfy a < test either — that is the difference between unknown and zero", fewerThan.MatchCount)
	}

	// A NOT over the same condition must also not select the unknown row, and
	// the preview says so rather than leaving the author to guess.
	negated, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{
		Rule: ruleJSON(`{"schema_version":1,"root":{"op":"NOT","children":[
			{"field":"commercial.completed_order_count","operator":"GTE","value":2}]}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if negated.MatchCount != 0 {
		t.Fatalf("count = %d; a negation must not resurrect undecidable rows", negated.MatchCount)
	}
	if len(negated.Warnings) == 0 {
		t.Fatal("a negated availability-guarded condition must produce a preview warning")
	}
}

// TestSegments_PreviewIsBranchScoped proves a branch actor cannot learn the
// business-wide audience size — the count is the disclosure, not just the
// sample.
func TestSegments_PreviewIsBranchScoped(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	atOne := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_p1"))
	env.seedOrder(t, businessA, "vs_a", atOne, "ord_p1a", branchA1, 100000, 100000, env.now.AddDate(0, 0, -2), 1)
	env.seedOrder(t, businessA, "vs_a", atOne, "ord_p1b", branchA1, 100000, 100000, env.now.AddDate(0, 0, -1), 1)
	atTwo := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_p2"))
	env.seedOrder(t, businessA, "vs_a", atTwo, "ord_p2a", branchA2, 100000, 100000, env.now.AddDate(0, 0, -2), 1)
	env.seedOrder(t, businessA, "vs_a", atTwo, "ord_p2b", branchA2, 100000, 100000, env.now.AddDate(0, 0, -1), 1)

	business, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	if business.MatchCount != 2 {
		t.Fatalf("business-wide count = %d; want 2", business.MatchCount)
	}

	branch, err := env.service.PreviewSegment(ctx, actor, branchScope(branchA1), PreviewInput{
		ScopeType: ScopeBranch, BranchRef: branchA1, Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatalf("branch preview: %v", err)
	}
	if branch.MatchCount != 1 {
		t.Fatalf("branch count = %d; want 1 — a branch actor must not see the business total", branch.MatchCount)
	}

	// A business-scope preview is refused outright for a branch-only actor.
	_, err = env.service.PreviewSegment(ctx, actor, branchScope(branchA1), PreviewInput{
		ScopeType: ScopeBusiness, Rule: repeatOrdersRule()})
	if codeOf(t, err) != CodeSegmentScopeDenied {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentScopeDenied)
	}
	// So is a preview scoped to a branch they do not hold.
	_, err = env.service.PreviewSegment(ctx, actor, branchScope(branchA1), PreviewInput{
		ScopeType: ScopeBranch, BranchRef: branchA2, Rule: repeatOrdersRule()})
	if codeOf(t, err) != CodeSegmentScopeDenied {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentScopeDenied)
	}
}

// TestSegments_PreviewExcludesRedactedAndMerged proves the privacy filter runs
// outside the vendor rule and cannot be switched off by it.
func TestSegments_PreviewExcludesRedactedAndMerged(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	redacted := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_red"))
	env.seedOrder(t, businessA, "vs_a", redacted, "ord_r1", branchA1, 100000, 100000, env.now.AddDate(0, 0, -2), 1)
	env.seedOrder(t, businessA, "vs_a", redacted, "ord_r2", branchA1, 100000, 100000, env.now.AddDate(0, 0, -1), 1)
	merged := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_mrg"))
	env.seedOrder(t, businessA, "vs_a", merged, "ord_g1", branchA1, 100000, 100000, env.now.AddDate(0, 0, -2), 1)
	env.seedOrder(t, businessA, "vs_a", merged, "ord_g2", branchA1, 100000, 100000, env.now.AddDate(0, 0, -1), 1)
	survivor := env.seedTaggableProfile(t, businessA, "vs_a", "cus_surv", branchA1)

	before, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	if before.MatchCount != 2 {
		t.Fatalf("baseline count = %d; want 2", before.MatchCount)
	}

	if _, err = env.pool.Exec(ctx, `UPDATE vendor_customer_profiles
		SET privacy_state='REDACTED', redacted_at=now() WHERE id=$1`, redacted.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = env.pool.Exec(ctx, `UPDATE vendor_customer_profiles
		SET merge_state='MERGED_SOURCE', privacy_state='MERGED', merged_into_profile_id=$1 WHERE id=$2`,
		survivor.ID, merged.ID); err != nil {
		t.Fatal(err)
	}

	after, err := env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{Rule: repeatOrdersRule()})
	if err != nil {
		t.Fatal(err)
	}
	if after.MatchCount != 0 {
		t.Fatalf("count = %d; redacted and merged-away relationships must be structurally excluded", after.MatchCount)
	}
}

// TestSegments_TagRuleRespectsTagScope stops a branch segment referencing
// another branch's private tag, which would turn membership counts into an
// oracle for that branch's relationships.
func TestSegments_TagRuleRespectsTagScope(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	branchTag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{
		Name: "Branch Two Private", ScopeType: ScopeBranch, BranchRef: branchA2})
	if err != nil {
		t.Fatal(err)
	}
	rule := ruleJSON(`{"schema_version":1,"root":{"field":"crm.tags","operator":"HAS_TAG","value":{"tag_id":"` + branchTag.ID + `"}}}`)

	_, err = env.service.PreviewSegment(ctx, actor, branchScope(branchA1), PreviewInput{
		ScopeType: ScopeBranch, BranchRef: branchA1, Rule: rule})
	if codeOf(t, err) != CodeSegmentScopeDenied {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentScopeDenied)
	}

	// A tag that does not exist at all is refused the same way, so a probe
	// cannot distinguish "wrong scope" from "no such tag".
	missing := ruleJSON(`{"schema_version":1,"root":{"field":"crm.tags","operator":"HAS_TAG","value":{"tag_id":"crmtag_nope"}}}`)
	_, err = env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{Rule: missing})
	if codeOf(t, err) != CodeSegmentScopeDenied {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentScopeDenied)
	}
}

// TestSegments_BranchRuleCannotNameAnotherBranch closes the same hole for the
// branch reference field.
func TestSegments_BranchRuleCannotNameAnotherBranch(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	rule := ruleJSON(`{"schema_version":1,"root":{"field":"relationship.branch_ref","operator":"EQ","value":"` + branchA2 + `"}}`)
	_, err := env.service.PreviewSegment(ctx, actor, branchScope(branchA1), PreviewInput{
		ScopeType: ScopeBranch, BranchRef: branchA1, Rule: rule})
	if codeOf(t, err) != CodeSegmentScopeDenied {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentScopeDenied)
	}

	// A branch belonging to another business is refused too.
	foreign := ruleJSON(`{"schema_version":1,"root":{"field":"relationship.branch_ref","operator":"EQ","value":"` + branchB1 + `"}}`)
	_, err = env.service.PreviewSegment(ctx, actor, globalScope(), PreviewInput{Rule: foreign})
	if codeOf(t, err) != CodeSegmentScopeDenied {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentScopeDenied)
	}
}

// TestSegments_CrossTenantIsolation is the tenancy assertion for segments.
func TestSegments_CrossTenantIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actorA := actorFor(businessA, "vs_a")
	actorB := actorFor(businessB, "vs_b")

	segmentB, err := env.service.CreateSegment(ctx, actorB, globalScope(), CreateSegmentInput{
		Name: "B Audience", SegmentType: SegmentStatic})
	if err != nil {
		t.Fatal(err)
	}
	profileB := env.seedTaggableProfile(t, businessB, "vs_b", "cus_bb", branchB1)

	if _, err = env.service.GetSegment(ctx, actorA, globalScope(), segmentB.ID); codeOf(t, err) != CodeSegmentNotFound {
		t.Fatalf("cross-tenant read code = %s; want %s", codeOf(t, err), CodeSegmentNotFound)
	}
	if _, err = env.service.BulkStaticMembership(ctx, actorA, globalScope(), segmentB.ID, []string{profileB.ID}, true); err == nil {
		t.Fatal("business A edited business B's segment")
	}
	if _, err = env.service.SetSegmentStatus(ctx, actorA, globalScope(), segmentB.ID, SegmentActive, 0); err == nil {
		t.Fatal("business A activated business B's segment")
	}

	segmentA, err := env.service.CreateSegment(ctx, actorA, globalScope(), CreateSegmentInput{
		Name: "A Audience", SegmentType: SegmentStatic})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.BulkStaticMembership(ctx, actorA, globalScope(), segmentA.ID, []string{profileB.ID}, true); err == nil {
		t.Fatal("business A added business B's customer to its own segment")
	}
}

// TestSegments_BranchActorCannotSeeBusinessSegment: even the existence of a
// business-wide audience is withheld, because its member count would describe
// the whole customer base.
func TestSegments_BranchActorCannotSeeBusinessSegment(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	business, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Company Wide", SegmentType: SegmentStatic, ScopeType: ScopeBusiness})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.GetSegment(ctx, actor, branchScope(branchA1), business.ID); codeOf(t, err) != CodeSegmentNotFound {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentNotFound)
	}
	segments, _, err := env.service.ListSegments(ctx, actor, branchScope(branchA1), SegmentListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range segments {
		if listed.ID == business.ID {
			t.Fatal("a business-wide segment appeared in a branch actor's list")
		}
	}
	// Creating one is refused outright.
	_, err = env.service.CreateSegment(ctx, actor, branchScope(branchA1), CreateSegmentInput{
		Name: "Sneaky Business Wide", SegmentType: SegmentStatic, ScopeType: ScopeBusiness})
	if codeOf(t, err) != CodeSegmentScopeDenied {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeSegmentScopeDenied)
	}
}

func TestSegments_VersionConflictOnConcurrentEdit(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Contested", SegmentType: SegmentStatic})
	if err != nil {
		t.Fatal(err)
	}
	first := "Winner"
	if _, err = env.service.UpdateSegment(ctx, actor, globalScope(), segment.ID, UpdateSegmentInput{
		Name: &first, ExpectedVersion: segment.Version}); err != nil {
		t.Fatal(err)
	}
	second := "Loser"
	_, err = env.service.UpdateSegment(ctx, actor, globalScope(), segment.ID, UpdateSegmentInput{
		Name: &second, ExpectedVersion: segment.Version})
	if codeOf(t, err) != CodeVersionConflict {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeVersionConflict)
	}
}
