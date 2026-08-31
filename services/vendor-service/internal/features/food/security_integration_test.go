package food

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// Tenant isolation
// ---------------------------------------------------------------------------

// TestSecurity_CrossTenantAccessIsRefused proves that every read and write path
// is scoped by the session's business, not by anything the caller supplies.
func TestSecurity_CrossTenantAccessIsRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), fixedScopes{staffA: {Global: true}, staffB: {Global: true}}, nil)
	ctx := context.Background()
	actorA := actorFor(businessA, staffA)
	actorB := actorFor(businessB, staffB)

	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	item := env.seedItem(t, businessA, staffA, menu, section, "Jollof Rice", 250000)
	group := env.seedGroup(t, businessA, staffA, "Add extras", 0, 5)

	if _, err := env.service.GetMenu(ctx, actorB, globalScope(), menu.ID); errorCode(t, err) != CodeMenuNotFound {
		t.Errorf("business B must not read business A's menu: %v", err)
	}
	if _, _, _, err := env.service.GetItem(ctx, actorB, globalScope(), item.ID); errorCode(t, err) != CodeItemNotFound {
		t.Errorf("business B must not read business A's item: %v", err)
	}
	if _, err := env.service.GetModifierGroup(ctx, actorB, group.ID); errorCode(t, err) != CodeModifierGroupNotFound {
		t.Errorf("business B must not read business A's modifier group: %v", err)
	}
	if _, err := env.service.UpdateMenu(ctx, actorB, globalScope(), menu.ID,
		UpdateMenuInput{Name: stringPtr("Hijacked"), ExpectedVersion: menu.Version}); errorCode(t, err) != CodeMenuNotFound {
		t.Errorf("business B must not edit business A's menu: %v", err)
	}
	if _, err := env.service.ArchiveItem(ctx, actorB, globalScope(), item.ID, item.Version); errorCode(t, err) != CodeItemNotFound {
		t.Errorf("business B must not archive business A's item: %v", err)
	}

	// A menu for another business's branch is refused even when the caller's
	// own scope is global, because the branch is not in their business.
	if _, err := env.service.CreateMenu(ctx, actorB, globalScope(), CreateMenuInput{
		BranchID: branchA1, CapabilityType: "RESTAURANT", Name: "Cross tenant"}); err == nil {
		t.Error("business B must not create a menu on business A's branch")
	}

	// Business A's item may not borrow business B's modifier group.
	groupB := env.seedGroup(t, businessB, staffB, "B extras", 0, 3)
	if _, err := env.service.AttachModifierGroup(ctx, actorA, globalScope(), item.ID, groupB.ID); errorCode(t, err) != CodeModifierGroupNotFound {
		t.Errorf("cross-business attachment must be refused: %v", err)
	}

	// Neither business's list sees the other's rows.
	menus, _, err := env.service.ListMenus(ctx, actorB, globalScope(), MenuListOptions{})
	if err != nil || len(menus) != 0 {
		t.Errorf("business B's menu list must be empty: %v %d", err, len(menus))
	}
	groups, _, err := env.service.ListModifierGroups(ctx, actorB, false, 0, 0)
	if err != nil || len(groups) != 1 || groups[0].ID != groupB.ID {
		t.Errorf("business B must see only its own groups: %v %+v", err, groups)
	}
}

// ---------------------------------------------------------------------------
// Branch isolation
// ---------------------------------------------------------------------------

// TestSecurity_BranchManagerScopeIsEnforced proves branch scope survives even
// though the actor holds the same role family as a business-wide manager.
func TestSecurity_BranchManagerScopeIsEnforced(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	owner := actorFor(businessA, staffA)

	menuOne := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	menuTwo := env.seedMenu(t, businessA, staffA, branchA2, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menuTwo, "Mains")
	itemAtTwo := env.seedItem(t, businessA, staffA, menuTwo, section, "Jollof Rice", 250000)

	branchScope := Scope{BranchIDs: []string{branchA1}}

	if _, err := env.service.GetMenu(ctx, owner, branchScope, menuOne.ID); err != nil {
		t.Fatalf("the authorised branch must remain reachable: %v", err)
	}
	if _, err := env.service.GetMenu(ctx, owner, branchScope, menuTwo.ID); errorCode(t, err) != CodeMenuNotFound {
		t.Errorf("an unauthorised branch's menu must be absent, not forbidden-with-detail: %v", err)
	}
	if _, err := env.service.UpdateMenu(ctx, owner, branchScope, menuTwo.ID,
		UpdateMenuInput{Name: stringPtr("Hijacked"), ExpectedVersion: menuTwo.Version}); errorCode(t, err) != CodeMenuNotFound {
		t.Errorf("a branch-scoped actor must not edit another branch's menu: %v", err)
	}
	if _, _, _, err := env.service.GetItem(ctx, owner, branchScope, itemAtTwo.ID); errorCode(t, err) != CodeItemNotFound {
		t.Errorf("a branch-scoped actor must not read another branch's item: %v", err)
	}
	if _, err := env.service.CreateMenu(ctx, owner, branchScope, CreateMenuInput{
		BranchID: branchA2, CapabilityType: "FAST_FOOD", Name: "Not mine"}); err == nil {
		t.Error("a branch-scoped actor must not create a menu at another branch")
	}

	// The list is filtered in SQL, so the other branch never appears at all.
	menus, _, err := env.service.ListMenus(ctx, owner, branchScope, MenuListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(menus) != 1 || menus[0].ID != menuOne.ID {
		t.Fatalf("expected only the authorised branch's menu, got %+v", menus)
	}
	// Asking explicitly for the other branch is refused rather than answered.
	if _, _, err = env.service.ListMenus(ctx, owner, branchScope, MenuListOptions{BranchID: branchA2}); status(t, err) != 403 {
		t.Errorf("expected an explicit unauthorised branch filter to be refused, got %v", err)
	}
}

// TestSecurity_BranchScopedActorCannotEditSharedModifierGroups records the
// documented architectural decision: groups are business-level definitions, so
// editing one is a business-wide act.
func TestSecurity_BranchScopedActorCannotEditSharedModifierGroups(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	group := env.seedGroup(t, businessA, staffA, "Add extras", 0, 5)
	branchScope := Scope{BranchIDs: []string{branchA1}}

	if _, err := env.service.CreateModifierGroup(ctx, actor, branchScope,
		CreateModifierGroupInput{Name: "Branch only", MaxSelect: 1}); status(t, err) != 403 {
		t.Errorf("expected group creation to need business-wide access, got %v", err)
	}
	if _, err := env.service.UpdateModifierGroup(ctx, actor, branchScope, group.ID,
		UpdateModifierGroupInput{Name: stringPtr("Renamed"), ExpectedVersion: group.Version}); status(t, err) != 403 {
		t.Errorf("expected group editing to need business-wide access, got %v", err)
	}
	if _, err := env.service.CreateModifier(ctx, actor, branchScope, group.ID,
		CreateModifierInput{Name: "Extra cheese", PriceDeltaMinor: 5000}); status(t, err) != 403 {
		t.Errorf("expected modifier creation to need business-wide access, got %v", err)
	}
	if _, err := env.service.ArchiveModifierGroup(ctx, actor, branchScope, group.ID, group.Version); status(t, err) != 403 {
		t.Errorf("expected group archival to need business-wide access, got %v", err)
	}

	// Reading and attaching remain available: attaching is a decision about one
	// branch's item, not an edit of the shared definition.
	if _, _, err := env.service.ListModifierGroups(ctx, actor, false, 0, 0); err != nil {
		t.Errorf("a branch-scoped actor must still be able to see which groups exist: %v", err)
	}
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	item := env.seedItem(t, businessA, staffA, menu, section, "Burger", 400000)
	if _, err := env.service.AttachModifierGroup(ctx, actor, branchScope, item.ID, group.ID); err != nil {
		t.Errorf("a branch-scoped actor must be able to attach a group to their own item: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Authorization
// ---------------------------------------------------------------------------

func TestSecurity_MissingAuthenticationAndEmptyScopeAreDenied(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), fixedScopes{}, nil)
	ctx := context.Background()

	if _, err := env.service.Authorize(ctx, Actor{}, PermCatalogueRead); status(t, err) != 401 {
		t.Errorf("an unauthenticated actor must be refused with 401, got %v", err)
	}
	if _, err := env.service.Authorize(ctx, Actor{StaffID: "vs_x"}, PermCatalogueRead); status(t, err) != 401 {
		t.Errorf("an actor with no business must be refused with 401, got %v", err)
	}
	// Deny by default: an unmapped role resolves to an empty scope.
	if _, err := env.service.Authorize(ctx, actorFor(businessA, staffA), PermCatalogueManage); status(t, err) != 403 {
		t.Errorf("an unmapped permission must be refused with 403, got %v", err)
	}
}

// TestSecurity_ReadOnlyActorCannotWrite is the Analyst/Auditor shape: read
// resolves, every write permission resolves to nothing.
func TestSecurity_ReadOnlyActorCannotWrite(t *testing.T) {
	analyst := scopesByPermission{
		staffA: {PermCatalogueRead: {Global: true}},
		staffB: {PermCatalogueRead: {Global: true}, PermCatalogueManage: {Global: true}, PermCataloguePublish: {Global: true}},
	}
	env := newTestEnv(t, allFlagsOn(), analyst, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)

	if _, err := env.service.Authorize(ctx, actor, PermCatalogueRead); err != nil {
		t.Fatalf("the analyst must be able to read: %v", err)
	}
	for _, permission := range []string{PermCatalogueManage, PermCataloguePublish} {
		if _, err := env.service.Authorize(ctx, actor, permission); status(t, err) != 403 {
			t.Errorf("the analyst must not hold %s, got %v", permission, err)
		}
	}
}

// TestSecurity_PermissionSeedMatchesTheDocumentedRoleMatrix asserts the real
// migration seed rather than a restatement of it, so the IAM claim in the docs
// cannot drift away from what the database actually grants.
func TestSecurity_PermissionSeedMatchesTheDocumentedRoleMatrix(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()

	expected := map[string][]string{
		"role_owner":           {PermCatalogueRead, PermCatalogueManage, PermCataloguePublish, PermPricingManage, PermOperationsManage},
		"role_general_manager": {PermCatalogueRead, PermCatalogueManage, PermCataloguePublish, PermPricingManage, PermOperationsManage},
		"role_branch_manager":  {PermCatalogueRead, PermCatalogueManage, PermCataloguePublish, PermPricingManage, PermOperationsManage},
		"role_kitchen_pastry":  {PermCatalogueRead, PermCatalogueManage, PermOperationsManage},
		"role_analyst_auditor": {PermCatalogueRead},
		"role_marketing":       {},
		"role_crm_support":     {},
		"role_finance":         {},
		"role_procurement":     {},
		"role_shop_inventory":  {},
		"role_vendor_rider":    {},
	}
	for role, permissions := range expected {
		rows, err := env.pool.Query(ctx, `
			SELECT p.code FROM vendor_role_permissions rp
			JOIN vendor_permissions p ON p.id = rp.permission_id
			WHERE rp.role_id=$1 AND p.code LIKE 'vendor.food.%' ORDER BY p.code`, role)
		if err != nil {
			t.Fatal(err)
		}
		var granted []string
		for rows.Next() {
			var code string
			if err = rows.Scan(&code); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			granted = append(granted, code)
		}
		rows.Close()
		if len(granted) != len(permissions) {
			t.Errorf("%s: expected %v, got %v", role, permissions, granted)
			continue
		}
		for _, permission := range permissions {
			if !contains(granted, permission) {
				t.Errorf("%s: missing %s (has %v)", role, permission, granted)
			}
		}
	}

	// None of the Section 9A permissions is high-risk: catalogue editing is not
	// a bank change, and marking it so would dilute step-up where it matters.
	var highRisk int
	if err := env.pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_permissions WHERE code LIKE 'vendor.food.%' AND is_high_risk`).Scan(&highRisk); err != nil {
		t.Fatal(err)
	}
	if highRisk != 0 {
		t.Errorf("expected no high-risk Section 9A permission, got %d", highRisk)
	}
}

// ---------------------------------------------------------------------------
// Feature flags
// ---------------------------------------------------------------------------

func TestSecurity_DisabledWritesAndKillSwitchWithholdMutations(t *testing.T) {
	readOnly := newTestEnv(t, Flags{CatalogueEnabled: true, WritesEnabled: false}, nil, nil)
	if _, err := readOnly.service.CreateMenu(context.Background(), actorFor(businessA, staffA), globalScope(),
		CreateMenuInput{BranchID: branchA1, CapabilityType: "RESTAURANT", Name: "Main"}); status(t, err) != 503 {
		t.Fatalf("expected disabled writes to withhold the mutation, got %v", err)
	}
	// Reads stay available, so a rollout can withhold edits without blinding
	// operators.
	if _, _, err := readOnly.service.ListMenus(context.Background(), actorFor(businessA, staffA), globalScope(),
		MenuListOptions{}); err != nil {
		t.Fatalf("reads must survive a writes-disabled rollout: %v", err)
	}

	killed := newTestEnv(t, Flags{CatalogueEnabled: true, WritesEnabled: true, CommerceKillSwitch: true}, nil, nil)
	if _, err := killed.service.CreateMenu(context.Background(), actorFor(businessA, staffA), globalScope(),
		CreateMenuInput{BranchID: branchA1, CapabilityType: "RESTAURANT", Name: "Main"}); status(t, err) != 503 {
		t.Fatalf("the kill switch must override the writes flag, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Malformed input
// ---------------------------------------------------------------------------

func TestSecurity_MalformedIdentifiersAreRefusedWithoutLeakingExistence(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)

	for _, id := range []string{"", "   ", "'; DROP TABLE food_menus; --", "fmn_" + string(rune(0x1F600))} {
		if _, err := env.service.GetMenu(ctx, actor, globalScope(), id); errorCode(t, err) != CodeMenuNotFound {
			t.Errorf("malformed menu id %q must be reported as absent, got %v", id, err)
		}
		if _, _, _, err := env.service.GetItem(ctx, actor, globalScope(), id); errorCode(t, err) != CodeItemNotFound {
			t.Errorf("malformed item id %q must be reported as absent, got %v", id, err)
		}
	}
	// The injection attempt changed nothing: the table is still there.
	var count int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM food_menus`).Scan(&count); err != nil {
		t.Fatalf("food_menus must still exist: %v", err)
	}
}

// TestSecurity_OversizedPayloadsAreRefused covers the bounded-input rules that
// keep a single request from writing an unbounded row.
func TestSecurity_OversizedPayloadsAreRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")

	long := make([]byte, 5000)
	for index := range long {
		long[index] = 'a'
	}
	if _, err := env.service.CreateItem(ctx, actor, globalScope(), menu.ID, section.ID,
		CreateItemInput{Name: "Oversized", Description: string(long), BasePriceMinor: 1000}); status(t, err) != 422 {
		t.Errorf("expected an oversized description to be refused, got %v", err)
	}
	if _, err := env.service.CreateItem(ctx, actor, globalScope(), menu.ID, section.ID,
		CreateItemInput{Name: string(long), BasePriceMinor: 1000}); status(t, err) != 422 {
		t.Errorf("expected an oversized name to be refused, got %v", err)
	}
	tooMany := make([]string, MaxReorderEntries+1)
	for index := range tooMany {
		tooMany[index] = "fms_probe"
	}
	if _, err := env.service.ReorderSections(ctx, actor, globalScope(), menu.ID, tooMany); errorCode(t, err) != CodeInvalidReorder {
		t.Errorf("expected an oversized reorder to be refused, got %v", err)
	}
	if _, err := env.service.ReorderSections(ctx, actor, globalScope(), menu.ID, nil); errorCode(t, err) != CodeInvalidReorder {
		t.Errorf("expected an empty reorder to be refused, got %v", err)
	}
}
