package crm

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Corporate profile lifecycle
// ---------------------------------------------------------------------------

func TestCorporate_CreateAllocatesAnImmutableTenantLocalCode(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	first := env.createCorporate(t, businessA, "vs_a", CorporateProfileInput{OrganizationName: "Acme Industries"})
	second := env.createCorporate(t, businessA, "vs_a", CorporateProfileInput{OrganizationName: "Beta Schools"})

	if first.Code != "CORP-000001" || second.Code != "CORP-000002" {
		t.Fatalf("expected sequential codes, got %s and %s", first.Code, second.Code)
	}
	if first.Status != CorporateProspect {
		t.Fatalf("a new corporate customer should open in PROSPECT, got %s", first.Status)
	}

	// The counter is per business. A global sequence would make CORP-000001
	// disclose how many corporate accounts every other tenant has created.
	otherTenant := env.createCorporate(t, businessB, "vs_b", CorporateProfileInput{OrganizationName: "Gamma Hotels"})
	if otherTenant.Code != "CORP-000001" {
		t.Fatalf("another tenant's numbering leaked: %s", otherTenant.Code)
	}

	// The code is server-allocated and never client-settable: there is no input
	// field for it, and an update cannot move it.
	updated, err := env.service.UpdateCorporateCustomer(ctx, actorFor(businessA, "vs_a"), globalScope(), first.ID,
		UpdateCorporateCustomerInput{Profile: CorporateProfileInput{OrganizationName: "Acme Industries Ltd"},
			ExpectedVersion: first.Version})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Code != first.Code {
		t.Fatalf("the corporate code changed on update: %s -> %s", first.Code, updated.Code)
	}
}

func TestCorporate_LifecycleTransitions(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.createCorporate(t, businessA, "vs_a", CorporateProfileInput{OrganizationName: "Acme Industries"})

	// A relationship cannot be paused before it ever started.
	if _, err := env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		CorporateOnHold, profile.Version); err == nil {
		t.Fatal("PROSPECT -> ON_HOLD should be refused")
	} else if !isCRMCode(err, CodeCorporateInvalidState) {
		t.Fatalf("expected an invalid-state rejection, got %v", err)
	}

	active, err := env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		CorporateActive, profile.Version)
	if err != nil {
		t.Fatalf("PROSPECT -> ACTIVE: %v", err)
	}
	held, err := env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		CorporateOnHold, active.Version)
	if err != nil {
		t.Fatalf("ACTIVE -> ON_HOLD: %v", err)
	}
	inactive, err := env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		CorporateInactive, held.Version)
	if err != nil {
		t.Fatalf("ON_HOLD -> INACTIVE: %v", err)
	}
	// INACTIVE is recoverable; ARCHIVED is not.
	revived, err := env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		CorporateActive, inactive.Version)
	if err != nil {
		t.Fatalf("INACTIVE -> ACTIVE: %v", err)
	}
	archived, err := env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		CorporateArchived, revived.Version)
	if err != nil {
		t.Fatalf("ACTIVE -> ARCHIVED: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("an archived account should carry an archived_at timestamp")
	}
	if _, err = env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		CorporateActive, archived.Version); err == nil {
		t.Fatal("ARCHIVED must be terminal")
	}
	// Repeating a settled transition converges rather than failing on a version
	// it already advanced.
	again, err := env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID, CorporateArchived, 0)
	if err != nil || again.Status != CorporateArchived {
		t.Fatalf("re-archiving should be a no-op: %+v err=%v", again.Status, err)
	}
	// PROSPECT is an opening state, never a destination.
	other := env.createCorporate(t, businessA, "vs_a", CorporateProfileInput{OrganizationName: "Beta Schools"})
	if _, err = env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), other.ID,
		CorporateProspect, other.Version); err == nil {
		t.Fatal("an account cannot be moved back to PROSPECT")
	}
}

func TestCorporate_ArchiveBlocksOrdinaryWritesAndCancelsOpenTasks(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	location := env.addCorporateLocation(t, businessA, "vs_a", profile.ID, "HQ", "Head office")
	interaction := env.addCorporateInteraction(t, businessA, "vs_a", profile.ID, CreateCorporateInteractionInput{})
	open := env.addCorporateTask(t, businessA, "vs_a", profile.ID, CreateCorporateTaskInput{Title: "Call procurement"})
	done := env.addCorporateTask(t, businessA, "vs_a", profile.ID, CreateCorporateTaskInput{Title: "Send catalogue"})
	if _, err := env.service.TransitionTask(ctx, actor, globalScope(), done.ID, TaskCompleted, done.Version); err != nil {
		t.Fatalf("complete a task before archiving: %v", err)
	}

	current := env.reloadCorporate(t, businessA, profile.ID)
	archived, err := env.service.TransitionCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		CorporateArchived, current.Version)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}

	// History survives in full.
	if _, err = env.service.GetCorporateLocation(ctx, actor, globalScope(), profile.ID, location.ID); err != nil {
		t.Fatalf("an archived account lost its locations: %v", err)
	}
	if _, err = env.service.GetCorporateInteraction(ctx, actor, globalScope(), profile.ID, interaction.ID); err != nil {
		t.Fatalf("an archived account lost its interactions: %v", err)
	}

	// Unfinished work is cancelled transactionally rather than deleted or left
	// open on an account nobody works any more.
	cancelled := env.reloadCorporateTask(t, businessA, open.ID)
	if cancelled.Status != TaskCancelled || cancelled.CancelledAt == nil {
		t.Fatalf("an open task survived the archive as %s", cancelled.Status)
	}
	settled := env.reloadCorporateTask(t, businessA, done.ID)
	if settled.Status != TaskCompleted {
		t.Fatalf("a completed task was disturbed by the archive: %s", settled.Status)
	}
	// The cancellation is recorded in the same immutable history every other
	// transition uses.
	history, err := env.service.ListTaskHistory(ctx, actor, globalScope(), open.ID)
	if err != nil {
		t.Fatalf("task history: %v", err)
	}
	var sawCancel bool
	for _, entry := range history {
		if entry.Action == "CANCELLED" {
			sawCancel = true
		}
	}
	if !sawCancel {
		t.Fatal("the archive cancellation was not recorded in task history")
	}

	// Ordinary corporate writes are refused.
	for name, err := range map[string]error{
		"location": func() error {
			_, e := env.service.CreateCorporateLocation(ctx, actor, globalScope(), profile.ID,
				CorporateLocationInput{Code: "WH1", Name: "Warehouse"})
			return e
		}(),
		"cost centre": func() error {
			_, e := env.service.CreateCorporateCostCenter(ctx, actor, globalScope(), profile.ID,
				CorporateCostCenterInput{Code: "CC1", Name: "Ops"})
			return e
		}(),
		"interaction": func() error {
			_, e := env.service.CreateCorporateInteraction(ctx, actor, globalScope(), profile.ID,
				CreateCorporateInteractionInput{Type: InteractionNote, Direction: DirectionInternal,
					Subject: "Note", OccurredAt: env.now})
			return e
		}(),
		"task": func() error {
			_, e := env.service.CreateCorporateTask(ctx, actor, globalScope(), profile.ID,
				CreateCorporateTaskInput{Title: "Follow up"})
			return e
		}(),
		"update": func() error {
			_, e := env.service.UpdateCorporateCustomer(ctx, actor, globalScope(), profile.ID,
				UpdateCorporateCustomerInput{Profile: CorporateProfileInput{OrganizationName: "Renamed"},
					ExpectedVersion: archived.Version})
			return e
		}(),
	} {
		if err == nil {
			t.Errorf("an archived account accepted a new %s", name)
		}
	}
}

func TestCorporate_OptimisticConcurrency(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	first, err := env.service.UpdateCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		UpdateCorporateCustomerInput{Profile: CorporateProfileInput{OrganizationName: "Acme One"},
			ExpectedVersion: profile.Version})
	if err != nil {
		t.Fatalf("first writer should succeed: %v", err)
	}
	if _, err = env.service.UpdateCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		UpdateCorporateCustomerInput{Profile: CorporateProfileInput{OrganizationName: "Acme Two"},
			ExpectedVersion: profile.Version}); err == nil {
		t.Fatal("a stale expected_version should conflict, not silently win")
	} else if !isCRMCode(err, CodeCorporateVersionConflict) {
		t.Fatalf("expected a version conflict, got %v", err)
	}
	if env.reloadCorporate(t, businessA, profile.ID).OrganizationName != "Acme One" {
		t.Fatal("the losing write was applied")
	}
	if first.Version == profile.Version {
		t.Fatal("a successful update must advance the version")
	}
	// A missing expected_version is refused outright, so the guard is not
	// opt-in.
	if _, err = env.service.UpdateCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		UpdateCorporateCustomerInput{Profile: CorporateProfileInput{OrganizationName: "Acme Three"}}); err == nil {
		t.Fatal("an update without expected_version should be refused")
	}
}

func TestCorporate_ValidationAndBounds(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	cases := map[string]CorporateProfileInput{
		"unsupported organization type": {OrganizationName: "Acme", OrganizationType: "SPACESHIP"},
		"name too short":                {OrganizationName: "A"},
		"name with markup":              {OrganizationName: "<script>Acme</script>"},
		"branch set with no branches":   {OrganizationName: "Acme", ScopeMode: CorporateScopeBranchSet},
		"business scope naming a branch": {OrganizationName: "Acme", ScopeMode: CorporateScopeBusiness,
			BranchRefs: []string{branchA1}},
		"unsupported scope": {OrganizationName: "Acme", ScopeMode: "REGION"},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := env.service.CreateCorporateCustomer(ctx, actor, globalScope(), input); err == nil {
				t.Fatalf("expected %s to be rejected", name)
			}
		})
	}

	// An organization name longer than the bound is refused rather than
	// silently truncated.
	long := make([]byte, MaxOrganizationNameLength+1)
	for index := range long {
		long[index] = 'a'
	}
	if _, err := env.service.CreateCorporateCustomer(ctx, actor, globalScope(),
		CorporateProfileInput{OrganizationName: string(long)}); err == nil {
		t.Fatal("an over-long organization name should be rejected")
	}

	// A relationship cannot have started in the future.
	future := env.now.Add(72 * time.Hour)
	if _, err := env.service.CreateCorporateCustomer(ctx, actor, globalScope(),
		CorporateProfileInput{OrganizationName: "Acme", RelationshipStartedAt: &future}); err == nil {
		t.Fatal("a future relationship start should be rejected")
	}
}

func TestCorporate_ExternalOrganizationReferenceIsUniquePerBusiness(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	env.createCorporate(t, businessA, "vs_a", CorporateProfileInput{
		OrganizationName: "Acme Industries", ExternalOrganizationID: "ORG-9001"})
	if _, err := env.service.CreateCorporateCustomer(ctx, actorFor(businessA, "vs_a"), globalScope(),
		CorporateProfileInput{OrganizationName: "Acme Trading", ExternalOrganizationID: "ORG-9001"}); err == nil {
		t.Fatal("a duplicate trusted external reference should be refused within one business")
	} else if !isCRMCode(err, CodeCorporateDuplicate) {
		t.Fatalf("expected a duplicate rejection, got %v", err)
	}
	// Another tenant may legitimately use the same external reference.
	if _, err := env.service.CreateCorporateCustomer(ctx, actorFor(businessB, "vs_b"), globalScope(),
		CorporateProfileInput{OrganizationName: "Acme Industries", ExternalOrganizationID: "ORG-9001"}); err != nil {
		t.Fatalf("another tenant should be able to use the same external reference: %v", err)
	}
	// Two accounts may legitimately share an organization NAME: refusing that
	// would both block real relationships and turn a duplicate-name error into
	// an oracle for accounts the caller cannot see.
	if _, err := env.service.CreateCorporateCustomer(ctx, actorFor(businessA, "vs_a"), globalScope(),
		CorporateProfileInput{OrganizationName: "Acme Industries"}); err != nil {
		t.Fatalf("a duplicate organization name should be allowed: %v", err)
	}
}

func TestCorporate_AccountManagerMustBeActiveVendorStaff(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	// Another tenant's staff member is not assignable.
	if _, err := env.service.CreateCorporateCustomer(ctx, actor, globalScope(), CorporateProfileInput{
		OrganizationName: "Acme", AccountManagerStaffRef: "vs_b"}); err == nil {
		t.Fatal("another tenant's staff member was accepted as account manager")
	} else if !isCRMCode(err, CodeCorporateManagerInvalid) {
		t.Fatalf("expected an account-manager rejection, got %v", err)
	}
	if _, err := env.service.CreateCorporateCustomer(ctx, actor, globalScope(), CorporateProfileInput{
		OrganizationName: "Acme", AccountManagerStaffRef: "vs_nobody"}); err == nil {
		t.Fatal("an unknown staff member was accepted as account manager")
	}

	profile := env.createCorporate(t, businessA, "vs_a", CorporateProfileInput{
		OrganizationName: "Acme", AccountManagerStaffRef: "vs_a"})
	if profile.AccountManagerStaffID == nil || *profile.AccountManagerStaffID != "vs_a" {
		t.Fatalf("expected the account manager to be recorded, got %v", profile.AccountManagerStaffID)
	}
	// An inactive staff member cannot be assigned.
	suspended := env.seedBranchStaff(t, businessA, "vs_suspended", branchA1)
	if _, err := env.pool.Exec(ctx, `UPDATE vendor_staff SET status='suspended' WHERE id=$1`, suspended); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.UpdateCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		UpdateCorporateCustomerInput{Profile: CorporateProfileInput{OrganizationName: "Acme",
			AccountManagerStaffRef: suspended}, ExpectedVersion: profile.Version}); err == nil {
		t.Fatal("a suspended staff member was accepted as account manager")
	}
	// And it can be cleared explicitly.
	cleared, err := env.service.UpdateCorporateCustomer(ctx, actor, globalScope(), profile.ID,
		UpdateCorporateCustomerInput{Profile: CorporateProfileInput{OrganizationName: "Acme",
			ClearAccountManager: true}, ExpectedVersion: profile.Version})
	if err != nil {
		t.Fatalf("clear account manager: %v", err)
	}
	if cleared.AccountManagerStaffID != nil {
		t.Fatal("the account manager was not cleared")
	}
}

// ---------------------------------------------------------------------------
// Branch scope
// ---------------------------------------------------------------------------

func TestCorporate_BranchScopeReplacement(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.branchCorporate(t, businessA, "vs_a", "Acme Industries", branchA1)
	branches, err := env.service.BranchScopesFor(ctx, env.pool, businessA, profile.ID)
	if err != nil || len(branches) != 1 || branches[0] != branchA1 {
		t.Fatalf("expected one branch scope, got %v err=%v", branches, err)
	}

	widened, replaced, err := env.service.ReplaceBranchScopes(ctx, actor, globalScope(), profile.ID,
		CorporateScopeBranchSet, []string{branchA1, branchA2}, profile.Version)
	if err != nil {
		t.Fatalf("replace branch scopes: %v", err)
	}
	if len(replaced) != 2 {
		t.Fatalf("expected two branches, got %v", replaced)
	}

	// Moving to BUSINESS drops the rows entirely, so "which branches" and
	// "everybody" stay structurally different states.
	business, none, err := env.service.ReplaceBranchScopes(ctx, actor, globalScope(), profile.ID,
		CorporateScopeBusiness, nil, widened.Version)
	if err != nil {
		t.Fatalf("widen to business scope: %v", err)
	}
	if business.ScopeMode != CorporateScopeBusiness || len(none) != 0 {
		t.Fatalf("expected business scope with no branch rows, got %s / %v", business.ScopeMode, none)
	}
	stored, err := env.service.BranchScopesFor(ctx, env.pool, businessA, profile.ID)
	if err != nil || len(stored) != 0 {
		t.Fatalf("branch scope rows survived the move to business scope: %v err=%v", stored, err)
	}

	// A branch that does not belong to this business is refused.
	if _, _, err = env.service.ReplaceBranchScopes(ctx, actor, globalScope(), profile.ID,
		CorporateScopeBranchSet, []string{branchB1}, business.Version); err == nil {
		t.Fatal("another business's branch was accepted as a corporate scope")
	}
	// BRANCH_SET with no branches is refused rather than silently meaning
	// "nobody".
	if _, _, err = env.service.ReplaceBranchScopes(ctx, actor, globalScope(), profile.ID,
		CorporateScopeBranchSet, nil, business.Version); err == nil {
		t.Fatal("a branch-set scope with no branches should be refused")
	}
}

func TestCorporate_BranchActorCannotCreateOrWidenToBusinessScope(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	if _, err := env.service.CreateCorporateCustomer(ctx, actor, branchScope(branchA1),
		CorporateProfileInput{OrganizationName: "Business wide"}); err == nil {
		t.Fatal("a branch actor created a business-scope corporate customer")
	}
	// Nor may they name a branch they do not hold.
	if _, err := env.service.CreateCorporateCustomer(ctx, actor, branchScope(branchA1),
		CorporateProfileInput{OrganizationName: "Other branch", ScopeMode: CorporateScopeBranchSet,
			BranchRefs: []string{branchA2}}); err == nil {
		t.Fatal("a branch actor scoped an account to a branch they do not hold")
	}
	// Their own branch is fine.
	profile, err := env.service.CreateCorporateCustomer(ctx, actor, branchScope(branchA1),
		CorporateProfileInput{OrganizationName: "Own branch", ScopeMode: CorporateScopeBranchSet,
			BranchRefs: []string{branchA1}})
	if err != nil {
		t.Fatalf("a branch actor should be able to open an account for their own branch: %v", err)
	}
	// And they cannot then widen it to the whole business.
	if _, _, err = env.service.ReplaceBranchScopes(ctx, actor, branchScope(branchA1), profile.ID,
		CorporateScopeBusiness, nil, profile.Version); err == nil {
		t.Fatal("a branch actor widened an account to business scope")
	}
}

// ---------------------------------------------------------------------------
// Flags
// ---------------------------------------------------------------------------

func TestCorporate_RespectsFeatureFlags(t *testing.T) {
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	off := allFlagsOn()
	off.CorporateEnabled = false
	disabled := newTestEnv(t, off)
	if _, err := disabled.service.CreateCorporateCustomer(ctx, actor, globalScope(),
		CorporateProfileInput{OrganizationName: "Acme"}); err == nil {
		t.Fatal("corporate CRM should be unavailable when the flag is off")
	} else if !isCRMCode(err, CodeCorporateDisabled) {
		t.Fatalf("expected a corporate-disabled error, got %v", err)
	}
	if _, _, err := disabled.service.ListCorporateCustomers(ctx, actor, globalScope(),
		CorporateListOptions{}); err == nil {
		t.Fatal("corporate reads should be unavailable when the flag is off")
	}

	readOnly := allFlagsOn()
	readOnly.WritesEnabled = false
	frozen := newTestEnv(t, readOnly)
	if _, err := frozen.service.CreateCorporateCustomer(ctx, actor, globalScope(),
		CorporateProfileInput{OrganizationName: "Acme"}); err == nil {
		t.Fatal("corporate writes should be refused during a write freeze")
	}
	// Reads survive a write freeze so a rollout does not blind operators.
	if _, _, err := frozen.service.ListCorporateCustomers(ctx, actor, globalScope(),
		CorporateListOptions{}); err != nil {
		t.Fatalf("reads should survive a write freeze: %v", err)
	}
}

func TestCorporate_ListFiltersSortsAndPaginates(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	school := env.createCorporate(t, businessA, "vs_a", CorporateProfileInput{
		OrganizationName: "Zenith School", OrganizationType: OrgSchool})
	env.activeCorporate(t, businessA, "vs_a", "Acme Industries")

	byType, _, err := env.service.ListCorporateCustomers(ctx, actor, globalScope(),
		CorporateListOptions{OrganizationType: OrgSchool})
	if err != nil || len(byType) != 1 || byType[0].ID != school.ID {
		t.Fatalf("organization type filter: %d results err=%v", len(byType), err)
	}
	byStatus, _, err := env.service.ListCorporateCustomers(ctx, actor, globalScope(),
		CorporateListOptions{Status: CorporateActive})
	if err != nil || len(byStatus) != 1 {
		t.Fatalf("status filter: %d results err=%v", len(byStatus), err)
	}
	// Search covers the organization name and the vendor-local code, and
	// nothing else — there is deliberately no contact search.
	byName, _, err := env.service.ListCorporateCustomers(ctx, actor, globalScope(),
		CorporateListOptions{Search: "zenith"})
	if err != nil || len(byName) != 1 || byName[0].ID != school.ID {
		t.Fatalf("name search: %d results err=%v", len(byName), err)
	}
	byCode, _, err := env.service.ListCorporateCustomers(ctx, actor, globalScope(),
		CorporateListOptions{Search: school.Code})
	if err != nil || len(byCode) != 1 {
		t.Fatalf("code search: %d results err=%v", len(byCode), err)
	}

	// Sorting is a closed registry; an unknown key falls back to the default
	// rather than reaching SQL.
	sorted, _, err := env.service.ListCorporateCustomers(ctx, actor, globalScope(),
		CorporateListOptions{Sort: "organization_name", Direction: "asc"})
	if err != nil || len(sorted) != 2 || sorted[0].OrganizationName != "Acme Industries" {
		t.Fatalf("sorted list: %v err=%v", sorted, err)
	}
	if _, _, err = env.service.ListCorporateCustomers(ctx, actor, globalScope(),
		CorporateListOptions{Sort: "organization_name; DROP TABLE crm_tasks"}); err != nil {
		t.Fatalf("an unknown sort key should fall back safely, got %v", err)
	}

	// Unsupported filter values are validation errors, not silent matches.
	if _, _, err = env.service.ListCorporateCustomers(ctx, actor, globalScope(),
		CorporateListOptions{Status: "DELETED"}); err == nil {
		t.Fatal("an unsupported status filter should be rejected")
	}
	page, hasMore, err := env.service.ListCorporateCustomers(ctx, actor, globalScope(),
		CorporateListOptions{Limit: 1})
	if err != nil || len(page) != 1 || !hasMore {
		t.Fatalf("pagination: %d results hasMore=%t err=%v", len(page), hasMore, err)
	}
}
