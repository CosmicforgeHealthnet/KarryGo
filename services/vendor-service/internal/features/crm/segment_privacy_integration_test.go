package crm

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// applyRedaction drives the Section 8A privacy pipeline for one subject, which
// is the only way a redaction ever reaches Section 8B's handlers.
func (e *testEnv) applyRedaction(t *testing.T, profile Profile) {
	t.Helper()
	actor := actorFor(businessA, "vs_a")
	if err := e.tx(t, func(tx pgx.Tx) error {
		reloaded := e.reload(t, businessA, profile.ID)
		return e.service.ApplyPrivacy(context.Background(), tx, actor, &reloaded, PrivacyInput{
			SourceService:     "customer-service",
			SourceVersion:     1,
			PrivacySubjectRef: profile.PrimarySubjectRef(),
			Action:            PrivacyActionRedact,
			ObservedAt:        e.now,
		})
	}); err != nil {
		t.Fatalf("apply redaction: %v", err)
	}
}

// TestPrivacy_RedactionIsRegisteredForSection8BDomains proves 8B extended what a
// redaction covers through the 8A registry rather than by editing the redaction
// engine — the property that lets 8C onwards do the same.
func TestPrivacy_RedactionIsRegisteredForSection8BDomains(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	domains := env.service.privacy.Domains()
	for _, required := range []string{"tags", "segments"} {
		found := false
		for _, domain := range domains {
			if domain == required {
				found = true
			}
		}
		if !found {
			t.Fatalf("domain %q is not registered; redaction would silently miss it. registered=%v", required, domains)
		}
	}
	// The reported coverage is what makes the guarantee auditable, so it must
	// name the new domains too.
	summary, err := env.service.PrivacySummaryFor(context.Background(), actorFor(businessA, "vs_a"),
		env.seedProfile(t, businessA, "vs_a", customerSubject("cus_domains")))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.CoveredDomains) != len(domains) {
		t.Fatalf("covered domains = %v; want the full registry", summary.CoveredDomains)
	}
}

// TestPrivacy_RedactionRemovesTagsAndMembershipImmediately is the core privacy
// assertion for Section 8B: redaction takes effect inside the same transaction
// that records it, not at the next scheduled rebuild.
func TestPrivacy_RedactionRemovesTagsAndMembershipImmediately(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	doomed := env.seedRepeatCustomer(t, "cus_privacy", branchA1)
	survivor := env.seedRepeatCustomer(t, "cus_privacy_ok", branchA1)

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Targeted"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{doomed.ID, survivor.ID}); err != nil {
		t.Fatal(err)
	}
	static, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Manual Audience", SegmentType: SegmentStatic})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.BulkStaticMembership(ctx, actor, globalScope(), static.ID,
		[]string{doomed.ID, survivor.ID}, true); err != nil {
		t.Fatal(err)
	}
	dynamic := env.activeDynamicSegment(t, "Derived Audience")
	if dynamic.CurrentMemberCount != 2 {
		t.Fatalf("dynamic baseline = %d; want 2", dynamic.CurrentMemberCount)
	}
	if _, err = env.service.BulkExclusion(ctx, actor, globalScope(), dynamic.ID, []string{doomed.ID}, true); err != nil {
		t.Fatal(err)
	}

	env.applyRedaction(t, doomed)

	var tagMemberships int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_tag_memberships WHERE profile_id=$1`,
		doomed.ID).Scan(&tagMemberships); err != nil {
		t.Fatal(err)
	}
	if tagMemberships != 0 {
		t.Fatalf("tag memberships = %d; a redacted relationship must not stay selectable by tag", tagMemberships)
	}
	var segmentMemberships int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_memberships WHERE profile_id=$1`,
		doomed.ID).Scan(&segmentMemberships); err != nil {
		t.Fatal(err)
	}
	if segmentMemberships != 0 {
		t.Fatalf("segment memberships = %d; want 0 immediately after redaction", segmentMemberships)
	}
	var exclusions int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_exclusions WHERE profile_id=$1`,
		doomed.ID).Scan(&exclusions); err != nil {
		t.Fatal(err)
	}
	if exclusions != 0 {
		t.Fatalf("exclusions = %d; a customer-specific decision must not outlive the relationship", exclusions)
	}

	// The other relationship is untouched.
	if tags, tagErr := env.service.TagsForProfile(ctx, actor, globalScope(), survivor.ID); tagErr != nil || len(tags) != 1 {
		t.Fatalf("survivor tags = %v err=%v; want the unrelated relationship untouched", tags, tagErr)
	}
	if env.reloadSegment(t, businessA, static.ID).CurrentMemberCount != 1 {
		t.Fatal("the static count did not follow the redaction")
	}
	// The dynamic segment is marked stale so its count is rebuilt honestly.
	if got := env.reloadSegment(t, businessA, dynamic.ID).EvaluationStatus; got != EvalStale {
		t.Fatalf("dynamic status = %s; want STALE after a redaction changed the answer", got)
	}

	// A rebuild cannot bring the redacted relationship back.
	if _, err = env.service.RequestMaterialization(ctx, actor, globalScope(), dynamic.ID); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)
	rebuilt := env.reloadSegment(t, businessA, dynamic.ID)
	if rebuilt.CurrentMemberCount != 1 {
		t.Fatalf("rebuilt count = %d; a redacted relationship must never rejoin an audience", rebuilt.CurrentMemberCount)
	}
	ids := env.memberIDs(t, rebuilt)
	if len(ids) != 1 || ids[0] != survivor.ID {
		t.Fatalf("members = %v; want only %s", ids, survivor.ID)
	}
}

func TestPrivacy_EligibilityChangesInvalidateAndRemoveCurrentMembership(t *testing.T) {
	for _, action := range []PrivacyAction{PrivacyActionRestrict, PrivacyActionMarketingSuppress} {
		t.Run(string(action), func(t *testing.T) {
			env := newTestEnv(t, allFlagsOn())
			ctx := context.Background()
			actor := actorFor(businessA, "vs_a")
			profile := env.seedTaggableProfile(t, businessA, "vs_a", "cus_privacy_"+strings.ToLower(string(action)), branchA1)
			if err := env.tx(t, func(tx pgx.Tx) error {
				return env.service.ApplyConsent(ctx, tx, actor, profile.ID, ConsentInput{
					SourceService: "customer-service", SourceVersion: 1, Purpose: ConsentPurposeMarketing,
					Channel: ChannelEmail, Decision: ConsentAllow, ObservedAt: env.now,
				})
			}); err != nil {
				t.Fatal(err)
			}

			eligible, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
				Name: "Eligible before " + string(action), SegmentType: SegmentDynamic,
				Rule: ruleJSON(`{"schema_version":1,"root":{"field":"consent.marketing.email.eligible","operator":"EQ","value":true}}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), eligible.ID, SegmentActive, eligible.Version); err != nil {
				t.Fatal(err)
			}
			unrelated, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
				Name: "Unrelated before " + string(action), SegmentType: SegmentDynamic,
				Rule: ruleJSON(`{"schema_version":1,"root":{"field":"profile.status","operator":"EQ","value":"ACTIVE"}}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = env.service.SetSegmentStatus(ctx, actor, globalScope(), unrelated.ID, SegmentActive, unrelated.Version); err != nil {
				t.Fatal(err)
			}
			env.materialize(t)
			if got := env.reloadSegment(t, businessA, eligible.ID); got.CurrentMemberCount != 1 {
				t.Fatalf("eligible baseline count = %d; want 1", got.CurrentMemberCount)
			}

			if err = env.tx(t, func(tx pgx.Tx) error {
				reloaded := env.reload(t, businessA, profile.ID)
				return env.service.ApplyPrivacy(ctx, tx, actor, &reloaded, PrivacyInput{
					SourceService: "customer-service", SourceVersion: 1,
					PrivacySubjectRef: profile.PrimarySubjectRef(), Action: action, ObservedAt: env.now,
				})
			}); err != nil {
				t.Fatal(err)
			}

			stale := env.reloadSegment(t, businessA, eligible.ID)
			if stale.EvaluationStatus != EvalStale || stale.CurrentMemberCount != 0 {
				t.Fatalf("post-privacy status/count = %s/%d; want STALE/0", stale.EvaluationStatus, stale.CurrentMemberCount)
			}
			if ids := env.memberIDs(t, stale); len(ids) != 0 {
				t.Fatalf("privacy-ineligible profile remained externally targetable: %v", ids)
			}
			if got := env.reloadSegment(t, businessA, unrelated.ID); got.EvaluationStatus != EvalCurrent {
				t.Fatalf("unrelated segment = %s; privacy invalidation was not dependency-scoped", got.EvaluationStatus)
			}
			summary, err := env.service.ConsentSummaryFor(ctx, actor, env.reload(t, businessA, profile.ID))
			if err != nil {
				t.Fatal(err)
			}
			if email := summary.Marketing[ChannelEmail]; email.Decision != ConsentAllow || email.Eligible {
				t.Fatalf("consent authority changed or suppression failed: %+v", email)
			}

			env.materialize(t)
			current := env.reloadSegment(t, businessA, eligible.ID)
			if current.EvaluationStatus != EvalCurrent || current.CurrentMemberCount != 0 {
				t.Fatalf("rebuilt status/count = %s/%d; want CURRENT/0", current.EvaluationStatus, current.CurrentMemberCount)
			}
		})
	}
}

// TestPrivacy_TagAssignmentCannotRaceRedaction: whichever transaction commits
// first, the final state never has a redacted relationship carrying a tag.
func TestPrivacy_TagAssignmentCannotRaceRedaction(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedRepeatCustomer(t, "cus_race_tag", branchA1)
	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Race"})
	if err != nil {
		t.Fatal(err)
	}

	// Redaction first: the assignment must be refused outright.
	env.applyRedaction(t, profile)
	_, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{profile.ID})
	if codeOf(t, err) != CodeProfileRedacted {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeProfileRedacted)
	}

	// Assignment first, redaction after: the redaction handler removes what it
	// finds, so the end state is identical.
	second := env.seedRepeatCustomer(t, "cus_race_tag_2", branchA1)
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{second.ID}); err != nil {
		t.Fatal(err)
	}
	env.applyRedaction(t, second)
	var memberships int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_tag_memberships WHERE profile_id=$1`,
		second.ID).Scan(&memberships); err != nil {
		t.Fatal(err)
	}
	if memberships != 0 {
		t.Fatalf("memberships = %d; redaction must win whichever order the two commit in", memberships)
	}
}

// TestPrivacy_StaticMembershipCannotRaceRedaction is the same guarantee for
// explicit audience membership.
func TestPrivacy_StaticMembershipCannotRaceRedaction(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	segment, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Race Static", SegmentType: SegmentStatic})
	if err != nil {
		t.Fatal(err)
	}
	added := env.seedRepeatCustomer(t, "cus_race_static", branchA1)
	if _, err = env.service.BulkStaticMembership(ctx, actor, globalScope(), segment.ID, []string{added.ID}, true); err != nil {
		t.Fatal(err)
	}
	env.applyRedaction(t, added)

	reloaded := env.reloadSegment(t, businessA, segment.ID)
	if reloaded.CurrentMemberCount != 0 {
		t.Fatalf("count = %d; a redacted relationship must not remain an explicit member", reloaded.CurrentMemberCount)
	}
	members, _, err := env.service.ListSegmentMembers(ctx, actor, globalScope(), reloaded, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("members = %+v; want none", members)
	}
}

// applyMerge drives the Section 8A identity-link pipeline, which is the only
// way a merge ever reaches Section 8B's handlers.
func (e *testEnv) applyMerge(t *testing.T, source, target Profile, evidence string) {
	t.Helper()
	actor := actorFor(businessA, "vs_a")
	if err := e.tx(t, func(tx pgx.Tx) error {
		return e.service.ApplyIdentityLink(context.Background(), tx, actor, IdentityLinkInput{
			AuthorityService:   "customer-service",
			AuthorityReference: "merge_" + evidence,
			AuthorityVersion:   1,
			EvidenceEventID:    evidence,
			Source:             SubjectRef{Type: source.SubjectType, Ref: source.PrimarySubjectRef()},
			Target:             SubjectRef{Type: target.SubjectType, Ref: target.PrimarySubjectRef()},
		})
	}); err != nil {
		t.Fatalf("apply merge: %v", err)
	}
}

// TestMerge_TagsAndStaticMembershipTransfer proves a vendor's own classification
// follows the surviving relationship, while derived membership is recomputed
// rather than copied.
func TestMerge_TagsAndStaticMembershipTransfer(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	source := env.seedRepeatCustomer(t, "cus_merge_src", branchA1)
	target := env.seedRepeatCustomer(t, "cus_merge_tgt", branchA1)

	shared, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Shared"})
	if err != nil {
		t.Fatal(err)
	}
	sourceOnly, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Source Only"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), shared.ID, []string{source.ID, target.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), sourceOnly.ID, []string{source.ID}); err != nil {
		t.Fatal(err)
	}

	static, err := env.service.CreateSegment(ctx, actor, globalScope(), CreateSegmentInput{
		Name: "Merge Static", SegmentType: SegmentStatic})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.BulkStaticMembership(ctx, actor, globalScope(), static.ID, []string{source.ID}, true); err != nil {
		t.Fatal(err)
	}

	env.applyMerge(t, source, target, "evt_merge_1")

	// The survivor holds the union, deduplicated: the shared tag is not doubled.
	tags, err := env.service.TagsForProfile(ctx, actor, globalScope(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("survivor tags = %d; want the deduplicated union of both relationships", len(tags))
	}
	var sourceTags int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_tag_memberships WHERE profile_id=$1`,
		source.ID).Scan(&sourceTags); err != nil {
		t.Fatal(err)
	}
	if sourceTags != 0 {
		t.Fatalf("merged-away relationship still holds %d tags; it is no longer an independent subject", sourceTags)
	}

	// Explicit membership transfers; the merged-away relationship stops being a
	// member in its own right.
	reloaded := env.reloadSegment(t, businessA, static.ID)
	if reloaded.CurrentMemberCount != 1 {
		t.Fatalf("static count = %d; want 1", reloaded.CurrentMemberCount)
	}
	ids := env.memberIDs(t, reloaded)
	if len(ids) != 1 || ids[0] != target.ID {
		t.Fatalf("static members = %v; want the survivor %s", ids, target.ID)
	}
}

// TestMerge_ExclusionsTransferSoTheDecisionSurvives: a vendor who excluded the
// source must not find the survivor quietly back in the audience.
func TestMerge_ExclusionsTransferSoTheDecisionSurvives(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	source := env.seedRepeatCustomer(t, "cus_excl_src", branchA1)
	target := env.seedRepeatCustomer(t, "cus_excl_tgt", branchA1)
	segment := env.activeDynamicSegment(t, "Excluding Audience")

	if _, err := env.service.BulkExclusion(ctx, actor, globalScope(), segment.ID, []string{source.ID}, true); err != nil {
		t.Fatal(err)
	}
	env.applyMerge(t, source, target, "evt_merge_excl")

	var excluded bool
	if err := env.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_segment_exclusions
		WHERE business_id=$1 AND segment_id=$2 AND profile_id=$3)`,
		businessA, segment.ID, target.ID).Scan(&excluded); err != nil {
		t.Fatal(err)
	}
	if !excluded {
		t.Fatal("the exclusion did not follow the merge; the survivor would silently rejoin the audience")
	}

	if _, err := env.service.RequestMaterialization(ctx, actor, globalScope(), segment.ID); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)
	rebuilt := env.reloadSegment(t, businessA, segment.ID)
	for _, id := range env.memberIDs(t, rebuilt) {
		if id == target.ID {
			t.Fatal("the survivor of a merge with an excluded source became a member")
		}
	}
}

// TestMerge_DynamicMembershipIsRecomputedNotCopied proves derived membership is
// re-decided by the rule rather than inherited, so a merge never asserts a match
// the rule was not asked about.
func TestMerge_DynamicMembershipIsRecomputedNotCopied(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	source := env.seedRepeatCustomer(t, "cus_dyn_src", branchA1)
	// The target has only one completed order, so it does not qualify on its own.
	target := env.seedTaggableProfile(t, businessA, "vs_a", "cus_dyn_tgt", branchA1)

	segment := env.activeDynamicSegment(t, "Recomputed Audience")
	ids := env.memberIDs(t, segment)
	if len(ids) != 1 || ids[0] != source.ID {
		t.Fatalf("baseline members = %v; want only %s", ids, source.ID)
	}

	env.applyMerge(t, source, target, "evt_merge_dyn")

	stale := env.reloadSegment(t, businessA, segment.ID)
	if stale.EvaluationStatus != EvalStale {
		t.Fatalf("status = %s; a merge must mark the derived audience stale", stale.EvaluationStatus)
	}
	var sourceRows int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_segment_memberships WHERE profile_id=$1`,
		source.ID).Scan(&sourceRows); err != nil {
		t.Fatal(err)
	}
	if sourceRows != 0 {
		t.Fatalf("merged-away relationship still has %d membership rows", sourceRows)
	}

	if _, err := env.service.RequestMaterialization(ctx, actor, globalScope(), segment.ID); err != nil {
		t.Fatal(err)
	}
	env.materialize(t)
	rebuilt := env.reloadSegment(t, businessA, segment.ID)
	// The survivor inherited the source's order facts, so it now qualifies on
	// its own terms — decided by the rule, not by the merge.
	rebuiltIDs := env.memberIDs(t, rebuilt)
	if len(rebuiltIDs) != 1 || rebuiltIDs[0] != target.ID {
		t.Fatalf("members after rebuild = %v; want the survivor %s judged by the rule", rebuiltIDs, target.ID)
	}
}

// TestCustomer360_ExposesTagsAndSegments proves Section 8B contributes to the
// composed view through the extension point rather than by rewriting it.
func TestCustomer360_ExposesTagsAndSegments(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedRepeatCustomer(t, "cus_360", branchA1)
	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Gold"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{profile.ID}); err != nil {
		t.Fatal(err)
	}
	segment := env.activeDynamicSegment(t, "Customer 360 Audience")

	view, err := env.service.Customer360For(ctx, actor, globalScope(), env.reload(t, businessA, profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	if view.Modules["tags"] != ModuleAvailable || view.Modules["segments"] != ModuleAvailable {
		t.Fatalf("modules = %v; tags and segments are operational and must say so", view.Modules)
	}
	if len(view.Tags) != 1 || view.Tags[0].TagRef != tag.ID {
		t.Fatalf("tags = %+v; want the assigned tag", view.Tags)
	}
	if len(view.Segments) != 1 || view.Segments[0].SegmentRef != segment.ID {
		t.Fatalf("segments = %+v; want the audience this relationship belongs to", view.Segments)
	}
	if view.Segments[0].EvaluationStatus != EvalCurrent {
		t.Fatalf("segment freshness = %s; a membership must be reported alongside how current it is",
			view.Segments[0].EvaluationStatus)
	}
	// Campaigns and corporate remain composed by their own sections rather than
	// by Section 8B, and intelligence became operational in Section 8H — so the
	// guard now pins the modules that are genuinely still unbuilt.
	for _, module := range []string{"campaigns", "corporate"} {
		if view.Modules[module] != ModuleNotImplemented {
			t.Fatalf("module %s = %s; unbuilt modules must not claim to be available", module, view.Modules[module])
		}
	}
	if view.Modules["intelligence"] != ModuleAvailable {
		t.Fatalf("module intelligence = %s; Section 8H composes it", view.Modules["intelligence"])
	}
	if view.Modules["interactions"] != ModuleAvailable || view.Modules["tasks"] != ModuleAvailable {
		t.Fatalf("Section 8C modules did not report availability: %v", view.Modules)
	}
	if view.Modules["loyalty"] != ModuleAvailable {
		t.Fatalf("Section 8D module did not report availability: %v", view.Modules)
	}
}

// TestCustomer360_BranchActorSeesOnlyBranchClassification stops a branch
// manager learning how the rest of the business has classified a customer.
func TestCustomer360_BranchActorSeesOnlyBranchClassification(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedRepeatCustomer(t, "cus_360_branch", branchA1)
	branchTag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{
		Name: "Branch One Note", ScopeType: ScopeBranch, BranchRef: branchA1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), branchTag.ID, []string{profile.ID}); err != nil {
		t.Fatal(err)
	}
	// A tag private to another branch must not be disclosed: its presence would
	// reveal that branch's relationship with this customer.
	otherBranchTag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{
		Name: "Branch Two Note", ScopeType: ScopeBranch, BranchRef: branchA2})
	if err != nil {
		t.Fatal(err)
	}
	env.seedOrder(t, businessA, "vs_a", profile, "ord_360_b2", branchA2, 100000, 100000, env.now.AddDate(0, 0, -1), 1)
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), otherBranchTag.ID, []string{profile.ID}); err != nil {
		t.Fatal(err)
	}
	businessTag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Company Note"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), businessTag.ID, []string{profile.ID}); err != nil {
		t.Fatal(err)
	}
	businessSegment := env.activeDynamicSegment(t, "National Top Customers")

	view, err := env.service.Customer360For(ctx, actor, branchScope(branchA1), env.reload(t, businessA, profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Tags) != 1 || view.Tags[0].TagRef != branchTag.ID {
		t.Fatalf("branch classifications = %+v; want only branch A tag %s", view.Tags, branchTag.ID)
	}
	if view.Tags[0].ScopeType != ScopeBranch || view.Tags[0].MemberCount != nil {
		t.Fatalf("branch tag leaked global classification metadata: %+v", view.Tags[0])
	}
	if len(view.Segments) != 0 {
		t.Fatalf("business-wide audience classification leaked to branch actor: %+v", view.Segments)
	}
	_ = otherBranchTag
	_ = businessTag
	_ = businessSegment
}

// TestCustomer360_SegmentationDisabledOmitsBlocks proves a disabled feature
// reports itself rather than returning an empty list that reads as "none".
func TestCustomer360_SegmentationDisabledOmitsBlocks(t *testing.T) {
	flags := allFlagsOn()
	flags.SegmentationEnabled = false
	flags.SegmentMaterializationEnabled = false
	env := newTestEnv(t, flags)
	ctx := context.Background()

	profile := env.seedRepeatCustomer(t, "cus_360_off", branchA1)
	view, err := env.service.Customer360For(ctx, actorFor(businessA, "vs_a"), globalScope(), profile)
	if err != nil {
		t.Fatal(err)
	}
	if view.Modules["tags"] != ModuleNotImplemented || view.Modules["segments"] != ModuleNotImplemented {
		t.Fatalf("modules = %v; a disabled feature must not report itself as available", view.Modules)
	}
	if view.Tags != nil || view.Segments != nil {
		t.Fatal("a disabled feature must omit its blocks rather than return empty lists")
	}
}
