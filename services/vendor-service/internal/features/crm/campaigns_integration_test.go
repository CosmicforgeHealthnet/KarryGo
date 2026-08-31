package crm

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// Definition lifecycle
// ---------------------------------------------------------------------------

func TestCampaign_CreateAndVersionLifecycle(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	audience := env.staticAudience(t, businessA, "vs_a", "Loyal customers", profile.ID)

	campaign, version, err := env.service.CreateCampaign(ctx, actor, globalScope(),
		campaignInput("Summer winback", audience.ID, "EMAIL", "PUSH"))
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	if campaign.Status != CampaignDraft {
		t.Fatalf("a new campaign should be DRAFT, got %s", campaign.Status)
	}
	if campaign.CurrentDefinitionVersion != 1 || version.DefinitionVersion != 1 {
		t.Fatalf("first definition should be version 1: campaign=%d version=%d",
			campaign.CurrentDefinitionVersion, version.DefinitionVersion)
	}
	if len(version.Channels) != 2 {
		t.Fatalf("expected two channels, got %v", version.Channels)
	}

	// A name-only edit that does not change what the campaign targets still
	// changes the campaign, but it is a real definition change and mints a new
	// immutable version.
	edit := campaignInput("Summer winback 2026", audience.ID, "EMAIL", "PUSH")
	updated, newVersion, err := env.service.UpdateCampaign(ctx, actor, globalScope(), campaign.ID,
		UpdateCampaignInput{Definition: edit, ChangeReason: "renamed", ExpectedVersion: campaign.Version})
	if err != nil {
		t.Fatalf("update campaign: %v", err)
	}
	if newVersion.DefinitionVersion != 2 || updated.CurrentDefinitionVersion != 2 {
		t.Fatalf("expected definition version 2, got %d/%d", newVersion.DefinitionVersion, updated.CurrentDefinitionVersion)
	}

	// Version 1 must be exactly what it was. An immutable history that could be
	// rewritten would make every prepared audience unexplainable.
	original, err := env.service.CampaignVersionAt(ctx, env.pool, businessA, campaign.ID, 1)
	if err != nil {
		t.Fatalf("reload version 1: %v", err)
	}
	if original.Name != "Summer winback" || original.DefinitionHash != version.DefinitionHash {
		t.Fatalf("version 1 was rewritten: name=%s hash=%s", original.Name, original.DefinitionHash)
	}

	// Re-submitting the identical definition is not a change and must not mint
	// a third version.
	same, sameVersion, err := env.service.UpdateCampaign(ctx, actor, globalScope(), campaign.ID,
		UpdateCampaignInput{Definition: edit, ExpectedVersion: updated.Version})
	if err != nil {
		t.Fatalf("no-op update: %v", err)
	}
	if same.CurrentDefinitionVersion != 2 || sameVersion.DefinitionVersion != 2 {
		t.Fatalf("an unchanged definition minted a new version: %d", same.CurrentDefinitionVersion)
	}
}

func TestCampaign_OptimisticConcurrencyOnUpdate(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone")
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Race", audience.ID))

	first, _, err := env.service.UpdateCampaign(ctx, actor, globalScope(), campaign.ID, UpdateCampaignInput{
		Definition: campaignInput("Race A", audience.ID), ExpectedVersion: campaign.Version})
	if err != nil {
		t.Fatalf("first writer should succeed: %v", err)
	}
	// The second writer still holds the pre-update version and must lose.
	if _, _, err = env.service.UpdateCampaign(ctx, actor, globalScope(), campaign.ID, UpdateCampaignInput{
		Definition: campaignInput("Race B", audience.ID), ExpectedVersion: campaign.Version}); err == nil {
		t.Fatal("a stale expected_version should be a conflict, not a silent last-write-wins")
	} else if !isCRMCode(err, CodeCampaignVersionConflict) {
		t.Fatalf("expected a version conflict, got %v", err)
	}
	if reloaded := env.reloadCampaign(t, businessA, campaign.ID); reloaded.Name != "Race A" {
		t.Fatalf("the losing write was applied: %s", reloaded.Name)
	}
	if first.Version == campaign.Version {
		t.Fatal("a successful update must advance the campaign version")
	}

	// An update with no expected_version at all is refused: silently accepting
	// one would make the whole guard opt-in.
	if _, _, err = env.service.UpdateCampaign(ctx, actor, globalScope(), campaign.ID, UpdateCampaignInput{
		Definition: campaignInput("Race C", audience.ID)}); err == nil {
		t.Fatal("an update without expected_version should be refused")
	}
}

func TestCampaign_ArchiveIsTerminal(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone")
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Retire me", audience.ID))

	archived, err := env.service.ArchiveCampaign(ctx, actor, globalScope(), campaign.ID, campaign.Version)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.Status != CampaignArchived || archived.ArchivedAt == nil {
		t.Fatalf("expected an archived campaign with a timestamp: %+v", archived)
	}
	// Archiving again is a no-op rather than an error, so a retried command
	// settles instead of failing.
	again, err := env.service.ArchiveCampaign(ctx, actor, globalScope(), campaign.ID, 0)
	if err != nil || again.Status != CampaignArchived {
		t.Fatalf("re-archiving should be a no-op: %+v err=%v", again, err)
	}
	// An archived campaign is frozen: it can neither be edited nor prepared.
	if _, _, err = env.service.UpdateCampaign(ctx, actor, globalScope(), campaign.ID, UpdateCampaignInput{
		Definition: campaignInput("Back from the dead", audience.ID), ExpectedVersion: archived.Version}); err == nil {
		t.Fatal("an archived campaign should not be editable")
	}
	if _, err = env.service.PrepareCampaign(ctx, actor, globalScope(), campaign.ID); err == nil {
		t.Fatal("an archived campaign should not be preparable")
	}
}

func TestCampaign_ScopeIsImmutableAfterCreation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	branchAudience := env.branchAudience(t, businessA, "vs_a", "Branch regulars", branchA1)
	definition := campaignInput("Branch offer", branchAudience.ID)
	definition.ScopeType = ScopeBranch
	definition.BranchRef = branchA1
	campaign := env.createCampaign(t, businessA, "vs_a", definition)

	// Moving a branch campaign to business scope would silently reinterpret
	// every audience already prepared from it.
	widened := definition
	widened.ScopeType = ScopeBusiness
	widened.BranchRef = ""
	if _, _, err := env.service.UpdateCampaign(ctx, actor, globalScope(), campaign.ID, UpdateCampaignInput{
		Definition: widened, ExpectedVersion: campaign.Version}); err == nil {
		t.Fatal("widening a branch campaign to business scope should be refused")
	}
}

// TestCampaign_BranchCampaignCannotUseABusinessAudience is the disclosure rule.
// A branch campaign that could name a business-wide segment would hand a branch
// manager a lawful route to business-wide customer counts.
func TestCampaign_BranchCampaignCannotUseABusinessAudience(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	businessWide := env.staticAudience(t, businessA, "vs_a", "Everyone in the business")
	otherBranch := env.branchAudience(t, businessA, "vs_a", "Other branch regulars", branchA2)

	definition := campaignInput("Branch offer", businessWide.ID)
	definition.ScopeType = ScopeBranch
	definition.BranchRef = branchA1
	if _, _, err := env.service.CreateCampaign(ctx, actor, globalScope(), definition); err == nil {
		t.Fatal("a branch campaign using a business-wide audience should be refused")
	} else if !isCRMCode(err, CodeCampaignAudienceScope) {
		t.Fatalf("expected an audience scope rejection, got %v", err)
	}

	definition.SourceSegmentRef = otherBranch.ID
	if _, _, err := env.service.CreateCampaign(ctx, actor, globalScope(), definition); err == nil {
		t.Fatal("a branch campaign using another branch's audience should be refused")
	}
}

func TestCampaign_BranchActorCannotCreateABusinessCampaign(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone")
	if _, _, err := env.service.CreateCampaign(ctx, actor, branchScope(branchA1),
		campaignInput("Business-wide blast", audience.ID)); err == nil {
		t.Fatal("a branch-scoped actor should not be able to create a business campaign")
	}
}

func TestCampaign_MissingOrArchivedAudienceIsRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	if _, _, err := env.service.CreateCampaign(ctx, actor, globalScope(),
		campaignInput("Ghost audience", "crmseg_does_not_exist")); err == nil {
		t.Fatal("an unknown source segment should be refused")
	} else if !isCRMCode(err, CodeCampaignAudienceMissing) {
		t.Fatalf("expected an audience-not-found rejection, got %v", err)
	}

	audience := env.staticAudience(t, businessA, "vs_a", "Retired list")
	if _, err := env.service.SetSegmentStatus(ctx, actor, globalScope(), audience.ID, SegmentArchived, 0); err != nil {
		t.Fatalf("archive audience: %v", err)
	}
	if _, _, err := env.service.CreateCampaign(ctx, actor, globalScope(),
		campaignInput("Retired audience", audience.ID)); err == nil {
		t.Fatal("an archived source segment should be refused")
	}
}

func TestCampaign_DuplicateNameIsRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone")
	env.createCampaign(t, businessA, "vs_a", campaignInput("Spring sale", audience.ID))
	if _, _, err := env.service.CreateCampaign(ctx, actor, globalScope(),
		campaignInput("  spring   SALE ", audience.ID)); err == nil {
		t.Fatal("a duplicate campaign name should be refused")
	} else if !isCRMCode(err, CodeCampaignNameExists) {
		t.Fatalf("expected a duplicate-name rejection, got %v", err)
	}
}

func TestCampaign_ListFiltersAndPagination(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	audience := env.staticAudience(t, businessA, "vs_a", "Everyone")
	retention := campaignInput("Retention push", audience.ID, "PUSH")
	retention.Objective = ObjectiveRetention
	env.createCampaign(t, businessA, "vs_a", retention)
	env.createCampaign(t, businessA, "vs_a", campaignInput("General email", audience.ID, "EMAIL"))

	byObjective, _, err := env.service.ListCampaigns(ctx, actor, globalScope(),
		CampaignListOptions{Objective: ObjectiveRetention})
	if err != nil || len(byObjective) != 1 || byObjective[0].Name != "Retention push" {
		t.Fatalf("objective filter: %d results err=%v", len(byObjective), err)
	}
	byChannel, _, err := env.service.ListCampaigns(ctx, actor, globalScope(), CampaignListOptions{Channel: "PUSH"})
	if err != nil || len(byChannel) != 1 || byChannel[0].Name != "Retention push" {
		t.Fatalf("channel filter: %d results err=%v", len(byChannel), err)
	}
	// Sorting is a closed set and filters are validated, so a caller cannot
	// smuggle an expression into either.
	if _, _, err = env.service.ListCampaigns(ctx, actor, globalScope(), CampaignListOptions{Status: "SENT"}); err == nil {
		t.Fatal("an unsupported status filter should be rejected")
	}
	if _, _, err = env.service.ListCampaigns(ctx, actor, globalScope(), CampaignListOptions{Channel: "TELEPATHY"}); err == nil {
		t.Fatal("an unsupported channel filter should be rejected")
	}

	page, hasMore, err := env.service.ListCampaigns(ctx, actor, globalScope(), CampaignListOptions{Limit: 1})
	if err != nil || len(page) != 1 || !hasMore {
		t.Fatalf("pagination: %d results hasMore=%t err=%v", len(page), hasMore, err)
	}
}

func TestCampaign_WritesRespectFlags(t *testing.T) {
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	flags := allFlagsOn()
	flags.CampaignsEnabled = false
	env := newTestEnv(t, flags)
	if _, _, err := env.service.CreateCampaign(ctx, actor, globalScope(),
		campaignInput("Disabled", "crmseg_1")); err == nil {
		t.Fatal("campaigns should be unavailable when the flag is off")
	} else if !isCRMCode(err, CodeCampaignsDisabled) {
		t.Fatalf("expected a campaigns-disabled error, got %v", err)
	}

	readOnly := allFlagsOn()
	readOnly.WritesEnabled = false
	readOnlyEnv := newTestEnv(t, readOnly)
	if _, _, err := readOnlyEnv.service.CreateCampaign(ctx, actor, globalScope(),
		campaignInput("Frozen", "crmseg_1")); err == nil {
		t.Fatal("campaign writes should be refused when CRM writes are frozen")
	}
	// Reads survive a write freeze so a rollout does not blind operators.
	if _, _, err := readOnlyEnv.service.ListCampaigns(ctx, actor, globalScope(), CampaignListOptions{}); err != nil {
		t.Fatalf("reads should survive a write freeze: %v", err)
	}
}
