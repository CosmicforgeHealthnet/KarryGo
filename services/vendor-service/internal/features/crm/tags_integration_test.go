package crm

import (
	"context"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
)

// globalScope and branchScope are the two shapes the Section 5 evaluator hands
// the CRM feature: a vendor-wide grant, or a grant limited to named branches.
func globalScope() Scope { return Scope{Global: true} }

func branchScope(branchIDs ...string) Scope { return Scope{BranchIDs: branchIDs} }

// seedTaggableProfile creates a relationship with a completed order at a branch,
// which is what makes it visible to a branch-scoped actor.
func (e *testEnv) seedTaggableProfile(t *testing.T, business, staff, ref, branch string) Profile {
	t.Helper()
	profile := e.seedProfile(t, business, staff, customerSubject(ref))
	e.seedOrder(t, business, staff, profile, "ord_"+ref, branch, 500000, 500000, e.now.AddDate(0, 0, -3), 1)
	return e.reload(t, business, profile.ID)
}

func statusOf(t *testing.T, err error) int {
	t.Helper()
	appErr := apperrors.From(err)
	if appErr == nil {
		t.Fatalf("expected an application error, got %v", err)
	}
	return appErr.Status
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Details == nil {
		return ""
	}
	code, _ := appErr.Details["code"].(string)
	return code
}

func TestTags_CreateNormalizeAndReject(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "  VIP  ", ScopeType: ScopeBusiness})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tag.Name != "VIP" || tag.NormalizedName != "vip" {
		t.Fatalf("tag = %+v; want display VIP normalised vip", tag)
	}

	// Case and surrounding whitespace must not produce a second tag a human
	// would read as the same one. Internal spacing is collapsed but not removed,
	// so "V I P" stays a genuinely different name from "VIP".
	for _, duplicate := range []string{"vip", "Vip", "  VIP  "} {
		if _, err = env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: duplicate}); err == nil {
			t.Fatalf("duplicate %q was accepted", duplicate)
		}
	}
	spaced, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Gold   Tier"})
	if err != nil {
		t.Fatalf("create spaced name: %v", err)
	}
	if spaced.NormalizedName != "gold tier" {
		t.Fatalf("normalised = %q; want internal whitespace collapsed to one space", spaced.NormalizedName)
	}
	if _, err = env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "gold tier"}); err == nil {
		t.Fatal("a name differing only by internal spacing was accepted as a second tag")
	}
	_, err = env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "vip"})
	if codeOf(t, err) != CodeTagAlreadyExists {
		t.Fatalf("duplicate code = %s; want %s", codeOf(t, err), CodeTagAlreadyExists)
	}

	for _, reserved := range []string{"sys:churn", "system:vip", "karrygo:core"} {
		if _, err = env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: reserved}); err == nil {
			t.Fatalf("reserved namespace %q was accepted", reserved)
		}
	}
	// Empty, too short, markup, an embedded NUL and a zero-width-only name are
	// all refused: the last would otherwise render as an invisible tag no
	// operator could select, rename or delete.
	// A NUL and a zero-width-only name are built from explicit runes so the
	// test source itself stays plain ASCII.
	zeroWidthOnly := string([]rune{0x200b, 0x200b})
	embeddedNUL := "vip" + string([]rune{0})
	for _, bad := range []string{"", " ", "a", "<script>x</script>", embeddedNUL, zeroWidthOnly} {
		if _, err = env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: bad}); err == nil {
			t.Fatalf("invalid name %q was accepted", bad)
		}
	}
	if _, err = env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: strings.Repeat("x", 81)}); err == nil {
		t.Fatal("an over-long tag name was accepted")
	}
}

func TestTags_BranchScopeIsEnforced(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	// A branch manager cannot mint a business-wide taxonomy.
	_, err := env.service.CreateTag(ctx, actor, branchScope(branchA1), CreateTagInput{Name: "Business Wide", ScopeType: ScopeBusiness})
	if codeOf(t, err) != CodeTagScopeDenied {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeTagScopeDenied)
	}
	// Nor one belonging to a branch they do not hold.
	_, err = env.service.CreateTag(ctx, actor, branchScope(branchA1), CreateTagInput{
		Name: "Other Branch", ScopeType: ScopeBranch, BranchRef: branchA2})
	if codeOf(t, err) != CodeTagScopeDenied {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeTagScopeDenied)
	}

	own, err := env.service.CreateTag(ctx, actor, branchScope(branchA1), CreateTagInput{
		Name: "Lunch Regular", ScopeType: ScopeBranch, BranchRef: branchA1})
	if err != nil {
		t.Fatalf("branch tag: %v", err)
	}
	if own.BranchID == nil || *own.BranchID != branchA1 {
		t.Fatalf("branch tag = %+v; want branch %s", own, branchA1)
	}

	// A branch tag naming a branch of another business is refused by the tenancy
	// check before the database's composite foreign key ever sees it.
	_, err = env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{
		Name: "Foreign Branch", ScopeType: ScopeBranch, BranchRef: branchB1})
	if codeOf(t, err) != CodeCrossTenant {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeCrossTenant)
	}
}

// TestTags_BranchTagIsInvisibleToAnotherBranch proves a branch's private
// taxonomy is not discoverable: the neighbouring branch gets not-found, which is
// the same answer they would get for an id that never existed.
func TestTags_BranchTagIsInvisibleToAnotherBranch(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	tag, err := env.service.CreateTag(ctx, actor, branchScope(branchA1), CreateTagInput{
		Name: "Branch One Only", ScopeType: ScopeBranch, BranchRef: branchA1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.GetTag(ctx, actor, branchScope(branchA2), tag.ID); codeOf(t, err) != CodeTagNotFound {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeTagNotFound)
	}
	tags, _, _, err := env.service.ListTags(ctx, actor, branchScope(branchA2), TagListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range tags {
		if listed.ID == tag.ID {
			t.Fatal("another branch's tag appeared in the list")
		}
	}
}

func TestTags_AssignmentLifecycle(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "VIP"})
	if err != nil {
		t.Fatal(err)
	}
	first := env.seedTaggableProfile(t, businessA, "vs_a", "cus_1", branchA1)
	second := env.seedTaggableProfile(t, businessA, "vs_a", "cus_2", branchA1)

	result, err := env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{first.ID, second.ID})
	if err != nil {
		t.Fatalf("bulk assign: %v", err)
	}
	if result.Applied != 2 {
		t.Fatalf("applied = %d; want 2", result.Applied)
	}

	// Re-assigning is idempotent rather than an error: a retried bulk operation
	// must converge, not fail.
	repeat, err := env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{first.ID, second.ID})
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Applied != 0 || repeat.Skipped != 2 {
		t.Fatalf("repeat = %+v; want 0 applied, 2 skipped", repeat)
	}

	removed, err := env.service.BulkRemoveTag(ctx, actor, globalScope(), tag.ID, []string{first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if removed.Applied != 1 {
		t.Fatalf("removed = %d; want 1", removed.Applied)
	}

	tags, err := env.service.TagsForProfile(ctx, actor, globalScope(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].ID != tag.ID {
		t.Fatalf("tags for profile = %+v; want just %s", tags, tag.ID)
	}
}

func TestTags_BulkLimitsAndInvalidReferences(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Bulk"})
	if err != nil {
		t.Fatal(err)
	}

	oversized := make([]string, MaxBulkBatch+1)
	for index := range oversized {
		oversized[index] = "vcp_" + itoa(index)
	}
	_, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, oversized)
	if codeOf(t, err) != CodeBulkTooLarge {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeBulkTooLarge)
	}
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, nil); err == nil {
		t.Fatal("an empty bulk request was accepted")
	}
	// A reference that is not even shaped like an id is refused before it can
	// reach a query.
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID,
		[]string{"vcp_1'; DROP TABLE crm_tags; --"}); err == nil {
		t.Fatal("a malformed reference was accepted")
	}
}

// TestTags_BulkIsAllOrNothing proves an unauthorised or unknown reference
// anywhere in the batch prevents the whole operation, so a caller can never end
// up with a partially applied bulk tag they believe succeeded.
func TestTags_BulkIsAllOrNothing(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Partial"})
	if err != nil {
		t.Fatal(err)
	}
	valid := env.seedTaggableProfile(t, businessA, "vs_a", "cus_ok", branchA1)
	foreign := env.seedTaggableProfile(t, businessB, "vs_b", "cus_foreign", branchB1)

	_, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{valid.ID, foreign.ID})
	if err == nil {
		t.Fatal("a batch containing another business's customer was applied")
	}
	if status := statusOf(t, err); status != 404 {
		t.Fatalf("status = %d; want 404 so a cross-tenant probe learns nothing", status)
	}
	var count int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_tag_memberships WHERE tag_id=$1`, tag.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("memberships = %d; a rejected bulk operation must write nothing", count)
	}
}

func TestTags_ArchiveBlocksAssignmentAndClearsMembership(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Seasonal"})
	if err != nil {
		t.Fatal(err)
	}
	profile := env.seedTaggableProfile(t, businessA, "vs_a", "cus_seasonal", branchA1)
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{profile.ID}); err != nil {
		t.Fatal(err)
	}

	archived, err := env.service.ArchiveTag(ctx, actor, globalScope(), tag.ID, tag.Version)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.Status != TagArchived {
		t.Fatalf("status = %s; want %s", archived.Status, TagArchived)
	}
	var count int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_tag_memberships WHERE tag_id=$1`, tag.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("memberships after archive = %d; want 0", count)
	}
	_, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{profile.ID})
	if codeOf(t, err) != CodeTagArchived {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeTagArchived)
	}
	// The archived row itself survives, so historical audit still resolves.
	if _, err = env.service.GetTag(ctx, actor, globalScope(), tag.ID); err != nil {
		t.Fatalf("archived tag must remain readable: %v", err)
	}
}

func TestTags_VersionConflict(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Rename Me"})
	if err != nil {
		t.Fatal(err)
	}
	newName := "Renamed"
	if _, err = env.service.UpdateTag(ctx, actor, globalScope(), tag.ID, UpdateTagInput{
		Name: &newName, ExpectedVersion: tag.Version}); err != nil {
		t.Fatal(err)
	}
	// The second writer holds a stale version and must lose.
	stale := "Renamed Again"
	_, err = env.service.UpdateTag(ctx, actor, globalScope(), tag.ID, UpdateTagInput{
		Name: &stale, ExpectedVersion: tag.Version})
	if codeOf(t, err) != CodeVersionConflict {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeVersionConflict)
	}
}

// TestTags_CrossTenantAssignmentIsImpossible is the core tenancy assertion:
// business A cannot reach business B's tag or customer by id.
func TestTags_CrossTenantAssignmentIsImpossible(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actorA := actorFor(businessA, "vs_a")
	actorB := actorFor(businessB, "vs_b")

	tagB, err := env.service.CreateTag(ctx, actorB, globalScope(), CreateTagInput{Name: "B Only"})
	if err != nil {
		t.Fatal(err)
	}
	profileB := env.seedTaggableProfile(t, businessB, "vs_b", "cus_b", branchB1)

	if _, err = env.service.GetTag(ctx, actorA, globalScope(), tagB.ID); codeOf(t, err) != CodeTagNotFound {
		t.Fatalf("cross-tenant tag read code = %s; want %s", codeOf(t, err), CodeTagNotFound)
	}
	if _, err = env.service.BulkAssignTag(ctx, actorA, globalScope(), tagB.ID, []string{profileB.ID}); err == nil {
		t.Fatal("business A assigned business B's tag")
	}

	tagA, err := env.service.CreateTag(ctx, actorA, globalScope(), CreateTagInput{Name: "A Only"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.BulkAssignTag(ctx, actorA, globalScope(), tagA.ID, []string{profileB.ID}); err == nil {
		t.Fatal("business A tagged business B's customer")
	}
}

func TestTags_RedactedAndMergedProfilesCannotBeTagged(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Targetable"})
	if err != nil {
		t.Fatal(err)
	}
	profile := env.seedTaggableProfile(t, businessA, "vs_a", "cus_redact", branchA1)

	if _, err = env.pool.Exec(ctx, `UPDATE vendor_customer_profiles
		SET privacy_state='REDACTED', redacted_at=now() WHERE id=$1`, profile.ID); err != nil {
		t.Fatal(err)
	}
	_, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{profile.ID})
	if codeOf(t, err) != CodeProfileRedacted {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeProfileRedacted)
	}

	merged := env.seedTaggableProfile(t, businessA, "vs_a", "cus_merged", branchA1)
	survivor := env.seedTaggableProfile(t, businessA, "vs_a", "cus_survivor", branchA1)
	if _, err = env.pool.Exec(ctx, `UPDATE vendor_customer_profiles
		SET merge_state='MERGED_SOURCE', privacy_state='MERGED', merged_into_profile_id=$1 WHERE id=$2`,
		survivor.ID, merged.ID); err != nil {
		t.Fatal(err)
	}
	_, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{merged.ID})
	if codeOf(t, err) != CodeProfileMerged {
		t.Fatalf("code = %s; want %s", codeOf(t, err), CodeProfileMerged)
	}
}

// TestTags_BranchTagRequiresBranchRelationship stops a branch tag being applied
// to a customer who has never traded at that branch, which would otherwise let
// a branch label the whole business's customer base.
func TestTags_BranchTagRequiresBranchRelationship(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{
		Name: "Branch Two Regular", ScopeType: ScopeBranch, BranchRef: branchA2})
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := env.seedTaggableProfile(t, businessA, "vs_a", "cus_branch1", branchA1)
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{elsewhere.ID}); err == nil {
		t.Fatal("a branch tag was applied to a customer with no relationship at that branch")
	}
}

func TestTags_MemberCountIsBranchScoped(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	tag, err := env.service.CreateTag(ctx, actor, globalScope(), CreateTagInput{Name: "Everywhere"})
	if err != nil {
		t.Fatal(err)
	}
	atOne := env.seedTaggableProfile(t, businessA, "vs_a", "cus_b1", branchA1)
	atTwo := env.seedTaggableProfile(t, businessA, "vs_a", "cus_b2", branchA2)
	if _, err = env.service.BulkAssignTag(ctx, actor, globalScope(), tag.ID, []string{atOne.ID, atTwo.ID}); err != nil {
		t.Fatal(err)
	}

	_, globalCounts, _, err := env.service.ListTags(ctx, actor, globalScope(), TagListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if globalCounts[tag.ID] != 2 {
		t.Fatalf("business-wide count = %d; want 2", globalCounts[tag.ID])
	}
	_, branchCounts, _, err := env.service.ListTags(ctx, actor, branchScope(branchA1), TagListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if branchCounts[tag.ID] != 1 {
		t.Fatalf("branch count = %d; want 1 — a branch must not infer the business total", branchCounts[tag.ID])
	}
}
