package food

import (
	"context"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
)

func errorCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Details == nil {
		return ""
	}
	code, _ := appErr.Details["code"].(string)
	return code
}

func status(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	return apperrors.From(err).Status
}

// ---------------------------------------------------------------------------
// Menus
// ---------------------------------------------------------------------------

func TestMenu_CreateReadUpdateLifecycle(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)

	menu, err := env.service.CreateMenu(ctx, actor, globalScope(), CreateMenuInput{
		BranchID: branchA1, CapabilityType: "RESTAURANT", Name: "  Main   Menu  ", Description: "All day"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if menu.Status != StatusDraft || menu.Version != 1 {
		t.Fatalf("expected a DRAFT menu at version 1, got %s v%d", menu.Status, menu.Version)
	}
	if menu.Name != "Main Menu" {
		t.Fatalf("expected interior whitespace to be normalised, got %q", menu.Name)
	}

	fetched, err := env.service.GetMenu(ctx, actor, globalScope(), menu.ID)
	if err != nil || fetched.ID != menu.ID {
		t.Fatalf("get: %v", err)
	}

	menus, hasMore, err := env.service.ListMenus(ctx, actor, globalScope(), MenuListOptions{})
	if err != nil || len(menus) != 1 || hasMore {
		t.Fatalf("list: %v menus=%d hasMore=%t", err, len(menus), hasMore)
	}

	updated, err := env.service.UpdateMenu(ctx, actor, globalScope(), menu.ID, UpdateMenuInput{
		Name: stringPtr("Breakfast Menu"), ExpectedVersion: menu.Version})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Version != 2 || updated.Name != "Breakfast Menu" {
		t.Fatalf("unexpected update result: %+v", updated)
	}

	// The version fence is real: replaying the same expected version fails.
	if _, err = env.service.UpdateMenu(ctx, actor, globalScope(), menu.ID, UpdateMenuInput{
		Name: stringPtr("Stale"), ExpectedVersion: menu.Version}); errorCode(t, err) != CodeStaleVersion {
		t.Fatalf("expected a stale-version conflict, got %v", err)
	}

	activated, err := env.service.SetMenuStatus(ctx, actor, globalScope(), menu.ID, StatusActive, updated.Version)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	paused, err := env.service.SetMenuStatus(ctx, actor, globalScope(), menu.ID, StatusPaused, activated.Version)
	if err != nil {
		t.Fatalf("pause: %v", err)
	}
	archived, err := env.service.SetMenuStatus(ctx, actor, globalScope(), menu.ID, StatusArchived, paused.Version)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("expected archived_at to be set")
	}
	// ARCHIVED is terminal.
	if _, err = env.service.SetMenuStatus(ctx, actor, globalScope(), menu.ID, StatusActive, archived.Version); errorCode(t, err) != CodeArchived {
		t.Fatalf("expected an archived menu to refuse reactivation, got %v", err)
	}

	actions := env.auditActions(t, menu.ID)
	for _, expected := range []string{"food.menu.created", "food.menu.updated", "food.menu.activated",
		"food.menu.paused", "food.menu.archived"} {
		if !contains(actions, expected) {
			t.Errorf("missing audit action %s in %v", expected, actions)
		}
	}
	events := env.outboxTypes(t, menu.ID)
	if !contains(events, "food.menu.created") || !contains(events, "food.menu.archived") {
		t.Errorf("missing outbox events in %v", events)
	}
}

func TestMenu_RejectsNonFoodCapability(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	for _, capability := range []string{"CLOUD_KITCHEN", "PASTRY_BAKERY", "SHOP_GROCERY", "MARKET", "WHOLESALE", "TAXI"} {
		_, err := env.service.CreateMenu(context.Background(), actorFor(businessA, staffA), globalScope(),
			CreateMenuInput{BranchID: branchA1, CapabilityType: capability, Name: "Menu"})
		if errorCode(t, err) != CodeCapabilityNotSupported {
			t.Errorf("capability %s: expected FOOD_CAPABILITY_NOT_SUPPORTED, got %v", capability, err)
		}
	}
}

func TestMenu_ActivationRequiresOperationalBranchCapability(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")

	// A catalogue may be prepared while the capability is still being set up...
	if _, err := env.pool.Exec(ctx, `UPDATE vendor_branch_capabilities SET status='DRAFT'
		WHERE branch_id=$1 AND capability_type='RESTAURANT'`, branchA1); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.UpdateMenu(ctx, actor, globalScope(), menu.ID,
		UpdateMenuInput{Description: stringPtr("still preparing"), ExpectedVersion: menu.Version}); err != nil {
		t.Fatalf("draft editing must stay possible while onboarding: %v", err)
	}
	// ...but publishing one is refused until it is operational.
	if _, err := env.service.SetMenuStatus(ctx, actor, globalScope(), menu.ID, StatusActive, menu.Version+1); errorCode(t, err) != CodeNotPublishable {
		t.Fatalf("expected publication to be refused, got %v", err)
	}

	if _, err := env.pool.Exec(ctx, `UPDATE vendor_branch_capabilities SET status='ACTIVE'
		WHERE branch_id=$1 AND capability_type='RESTAURANT'`, branchA1); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `UPDATE vendor_branches SET status='PAUSED' WHERE id=$1`, branchA1); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.SetMenuStatus(ctx, actor, globalScope(), menu.ID, StatusActive, menu.Version+1); errorCode(t, err) != CodeNotPublishable {
		t.Fatalf("expected a non-active branch to refuse publication, got %v", err)
	}

	if _, err := env.pool.Exec(ctx, `UPDATE vendor_branches SET status='ACTIVE' WHERE id=$1`, branchA1); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.SetMenuStatus(ctx, actor, globalScope(), menu.ID, StatusActive, menu.Version+1); err != nil {
		t.Fatalf("expected publication to succeed once everything is operational: %v", err)
	}
}

func TestMenu_BranchWithoutCapabilityCannotHostAMenu(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	if _, err := env.pool.Exec(ctx, `DELETE FROM vendor_branch_capabilities
		WHERE branch_id=$1 AND capability_type='FAST_FOOD'`, branchA2); err != nil {
		t.Fatal(err)
	}
	_, err := env.service.CreateMenu(ctx, actorFor(businessA, staffA), globalScope(),
		CreateMenuInput{BranchID: branchA2, CapabilityType: "FAST_FOOD", Name: "Quick Bites"})
	if errorCode(t, err) != CodeCapabilityNotSupported {
		t.Fatalf("expected the missing branch capability to be refused, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Sections
// ---------------------------------------------------------------------------

func TestSection_CreateListReorderArchive(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")

	starters := env.seedSection(t, businessA, staffA, menu, "Starters")
	mains := env.seedSection(t, businessA, staffA, menu, "Main Meals")
	drinks := env.seedSection(t, businessA, staffA, menu, "Drinks")
	if starters.Position != 0 || mains.Position != 1 || drinks.Position != 2 {
		t.Fatalf("expected append ordering 0,1,2 got %d,%d,%d", starters.Position, mains.Position, drinks.Position)
	}

	reordered, err := env.service.ReorderSections(ctx, actor, globalScope(), menu.ID,
		[]string{drinks.ID, starters.ID, mains.ID})
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if len(reordered) != 3 || reordered[0].ID != drinks.ID || reordered[2].ID != mains.ID {
		t.Fatalf("unexpected order: %+v", reordered)
	}
	for index, section := range reordered {
		if section.Position != index {
			t.Fatalf("expected contiguous positions, got %d at index %d", section.Position, index)
		}
	}

	// Duplicate, foreign and incomplete lists are all refused.
	if err = reorderErr(env, actor, menu.ID, []string{drinks.ID, drinks.ID, mains.ID}); errorCode(t, err) != CodeInvalidReorder {
		t.Errorf("expected duplicate ids to be refused, got %v", err)
	}
	if err = reorderErr(env, actor, menu.ID, []string{drinks.ID, starters.ID, "fms_not_real"}); errorCode(t, err) != CodeInvalidReorder {
		t.Errorf("expected an unknown id to be refused, got %v", err)
	}
	if err = reorderErr(env, actor, menu.ID, []string{drinks.ID, starters.ID}); errorCode(t, err) != CodeInvalidReorder {
		t.Errorf("expected an incomplete list to be refused, got %v", err)
	}

	// Archiving a section takes its items with it rather than orphaning them.
	item := env.seedItem(t, businessA, staffA, menu, mains, "Jollof Rice", 250000)
	current, err := env.service.GetMenu(ctx, actor, globalScope(), menu.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = current
	sections, err := env.service.ListSections(ctx, actor, globalScope(), menu.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	var live Section
	for _, section := range sections {
		if section.ID == mains.ID {
			live = section
		}
	}
	if _, err = env.service.ArchiveSection(ctx, actor, globalScope(), menu.ID, mains.ID, live.Version); err != nil {
		t.Fatalf("archive section: %v", err)
	}
	archivedItem, _, _, err := env.service.GetItem(ctx, actor, globalScope(), item.ID)
	if err != nil {
		t.Fatalf("get item after section archive: %v", err)
	}
	if archivedItem.Status != StatusArchived {
		t.Fatalf("expected the item to be archived with its section, got %s", archivedItem.Status)
	}

	remaining, err := env.service.ListSections(ctx, actor, globalScope(), menu.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 live sections, got %d", len(remaining))
	}
	for index, section := range remaining {
		if section.Position != index {
			t.Fatalf("expected live sections to stay contiguous, got %d at %d", section.Position, index)
		}
	}
}

func reorderErr(env *testEnv, actor Actor, menuID string, ids []string) error {
	_, err := env.service.ReorderSections(context.Background(), actor, globalScope(), menuID, ids)
	return err
}

func TestSection_CrossMenuNestedIDORIsRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menuOne := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	menuTwo := env.seedMenu(t, businessA, staffA, branchA1, "FAST_FOOD")
	section := env.seedSection(t, businessA, staffA, menuOne, "Starters")

	if _, err := env.service.UpdateSection(ctx, actor, globalScope(), menuTwo.ID, section.ID,
		UpdateSectionInput{Name: stringPtr("Hijacked"), ExpectedVersion: section.Version}); errorCode(t, err) != CodeSectionNotFound {
		t.Fatalf("expected a section addressed through the wrong menu to be absent, got %v", err)
	}
	if _, err := env.service.ArchiveSection(ctx, actor, globalScope(), menuTwo.ID, section.ID, section.Version); errorCode(t, err) != CodeSectionNotFound {
		t.Fatalf("expected archive through the wrong menu to be absent, got %v", err)
	}
	// The other menu's reorder cannot adopt it either.
	if _, err := env.service.ReorderSections(ctx, actor, globalScope(), menuTwo.ID, []string{section.ID}); errorCode(t, err) != CodeInvalidReorder {
		t.Fatalf("expected a foreign section in reorder to be refused, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Items
// ---------------------------------------------------------------------------

func TestItem_PricingIsIntegerMinorUnitsAndBounded(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")

	free, err := env.service.CreateItem(ctx, actor, globalScope(), menu.ID, section.ID,
		CreateItemInput{Name: "Tap Water", BasePriceMinor: 0})
	if err != nil {
		t.Fatalf("a zero price is legitimate: %v", err)
	}
	if free.BasePriceMinor != 0 {
		t.Fatalf("expected 0, got %d", free.BasePriceMinor)
	}

	if _, err = env.service.CreateItem(ctx, actor, globalScope(), menu.ID, section.ID,
		CreateItemInput{Name: "Negative", BasePriceMinor: -1}); status(t, err) != 422 {
		t.Fatalf("expected a negative price to be refused, got %v", err)
	}
	if _, err = env.service.CreateItem(ctx, actor, globalScope(), menu.ID, section.ID,
		CreateItemInput{Name: "Overflow", BasePriceMinor: MaxPriceMinor + 1}); status(t, err) != 422 {
		t.Fatalf("expected an out-of-range price to be refused, got %v", err)
	}

	item, err := env.service.CreateItem(ctx, actor, globalScope(), menu.ID, section.ID,
		CreateItemInput{Name: "Jollof Rice", BasePriceMinor: 250000})
	if err != nil {
		t.Fatal(err)
	}
	_, _, currency, err := env.service.GetItem(ctx, actor, globalScope(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if currency != "NGN" {
		t.Fatalf("expected the business currency, got %q", currency)
	}
}

func TestItem_DietaryAndAllergenMetadataIsBoundedAndNormalised(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")

	item, err := env.service.CreateItem(ctx, actor, globalScope(), menu.ID, section.ID, CreateItemInput{
		Name: "Vegan Bowl", BasePriceMinor: 300000,
		DietaryCodes:  []string{"vegan", " Gluten_Free "},
		AllergenCodes: []string{"soy"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(item.DietaryCodes) != 2 || item.DietaryCodes[0] != "GLUTEN_FREE" || item.DietaryCodes[1] != "VEGAN" {
		t.Fatalf("expected normalised, sorted codes, got %v", item.DietaryCodes)
	}

	for name, codes := range map[string][]string{
		"duplicate":  {"VEGAN", "vegan"},
		"too short":  {"V"},
		"bad chars":  {"<script>"},
		"sql-ish":    {"1; DROP TABLE food_items"},
		"unbounded":  {strings.Repeat("A", 41)},
		"too many":   make([]string, MaxDietaryCodes+1),
		"whitespace": {"  "},
	} {
		if _, err = env.service.CreateItem(ctx, actor, globalScope(), menu.ID, section.ID, CreateItemInput{
			Name: "Probe " + name, BasePriceMinor: 100, DietaryCodes: codes}); status(t, err) != 422 {
			t.Errorf("%s codes should be refused, got %v", name, err)
		}
	}
}

func TestItem_ReorderAndCrossSectionIsolation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	mains := env.seedSection(t, businessA, staffA, menu, "Mains")
	drinks := env.seedSection(t, businessA, staffA, menu, "Drinks")

	rice := env.seedItem(t, businessA, staffA, menu, mains, "Jollof Rice", 250000)
	beans := env.seedItem(t, businessA, staffA, menu, mains, "Beans", 150000)
	cola := env.seedItem(t, businessA, staffA, menu, drinks, "Cola", 50000)

	items, err := env.service.ReorderItems(ctx, actor, globalScope(), menu.ID, mains.ID, []string{beans.ID, rice.ID})
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if items[0].ID != beans.ID || items[1].ID != rice.ID {
		t.Fatalf("unexpected order: %+v", items)
	}

	// An item from another section cannot be pulled into this one's ordering.
	if _, err = env.service.ReorderItems(ctx, actor, globalScope(), menu.ID, mains.ID,
		[]string{beans.ID, rice.ID, cola.ID}); errorCode(t, err) != CodeInvalidReorder {
		t.Fatalf("expected a cross-section id to be refused, got %v", err)
	}
	// The drinks ordering is untouched by any of it.
	drinkItems, _, _, _, err := env.service.ListItems(ctx, actor, globalScope(), menu.ID, drinks.ID, false, 0, 0)
	if err != nil || len(drinkItems) != 1 || drinkItems[0].Position != 0 {
		t.Fatalf("drinks ordering disturbed: %v %+v", err, drinkItems)
	}
}

func TestItem_LifecycleAndArchive(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	item := env.seedItem(t, businessA, staffA, menu, section, "Jollof Rice", 250000)

	published, err := env.service.UpdateItem(ctx, actor, globalScope(), item.ID, UpdateItemInput{
		Status: stringPtr(StatusActive), ExpectedVersion: item.Version})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err = env.service.UpdateItem(ctx, actor, globalScope(), item.ID, UpdateItemInput{
		Status: stringPtr(StatusDraft), ExpectedVersion: published.Version}); status(t, err) != 409 {
		t.Fatalf("expected a published item to refuse a return to draft, got %v", err)
	}
	if _, err = env.service.UpdateItem(ctx, actor, globalScope(), item.ID, UpdateItemInput{
		Status: stringPtr("SOLD_OUT"), ExpectedVersion: published.Version}); status(t, err) != 409 {
		t.Fatalf("SOLD_OUT belongs to Section 9B and must be refused here, got %v", err)
	}
	if _, err = env.service.UpdateItem(ctx, actor, globalScope(), item.ID, UpdateItemInput{
		Status: stringPtr(StatusArchived), ExpectedVersion: published.Version}); status(t, err) != 409 {
		t.Fatalf("expected archiving through update to be refused, got %v", err)
	}

	archived, err := env.service.ArchiveItem(ctx, actor, globalScope(), item.ID, published.Version)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("expected archived_at to be set")
	}
	if _, err = env.service.UpdateItem(ctx, actor, globalScope(), item.ID, UpdateItemInput{
		Name: stringPtr("Renamed"), ExpectedVersion: archived.Version}); errorCode(t, err) != CodeArchived {
		t.Fatalf("expected an archived item to refuse edits, got %v", err)
	}
	// Archive is not deletion: the row is still resolvable.
	var count int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM food_items WHERE id=$1`, item.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("expected the archived item to be retained, count=%d err=%v", count, err)
	}
}

// ---------------------------------------------------------------------------
// Variants
// ---------------------------------------------------------------------------

func TestVariant_InheritOverrideAndOrdering(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	item := env.seedItem(t, businessA, staffA, menu, section, "Burger", 400000)

	single, err := env.service.CreateVariant(ctx, actor, globalScope(), item.ID, CreateVariantInput{Name: "Single"})
	if err != nil {
		t.Fatal(err)
	}
	if single.PriceOverrideMinor != nil {
		t.Fatal("expected a variant with no override to inherit the base price")
	}
	double, err := env.service.CreateVariant(ctx, actor, globalScope(), item.ID,
		CreateVariantInput{Name: "Double", PriceOverrideMinor: int64Ptr(650000)})
	if err != nil {
		t.Fatal(err)
	}
	if double.PriceOverrideMinor == nil || *double.PriceOverrideMinor != 650000 {
		t.Fatalf("unexpected override: %+v", double.PriceOverrideMinor)
	}

	if _, err = env.service.CreateVariant(ctx, actor, globalScope(), item.ID,
		CreateVariantInput{Name: "Negative", PriceOverrideMinor: int64Ptr(-5)}); status(t, err) != 422 {
		t.Fatalf("expected a negative override to be refused, got %v", err)
	}

	cleared, err := env.service.UpdateVariant(ctx, actor, globalScope(), item.ID, double.ID,
		UpdateVariantInput{ClearPriceOverride: true, ExpectedVersion: double.Version})
	if err != nil {
		t.Fatalf("clear override: %v", err)
	}
	if cleared.PriceOverrideMinor != nil {
		t.Fatal("expected the override to be cleared back to inheritance")
	}

	reordered, err := env.service.ReorderVariants(ctx, actor, globalScope(), item.ID, []string{double.ID, single.ID})
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if reordered[0].ID != double.ID || reordered[0].Position != 0 || reordered[1].Position != 1 {
		t.Fatalf("unexpected variant order: %+v", reordered)
	}

	archived, err := env.service.ArchiveVariant(ctx, actor, globalScope(), item.ID, single.ID, reordered[1].Version)
	if err != nil {
		t.Fatalf("archive variant: %v", err)
	}
	if archived.Status != ChildArchived {
		t.Fatalf("unexpected status %s", archived.Status)
	}
	live, err := env.service.ListVariants(ctx, actor, globalScope(), item.ID, false)
	if err != nil || len(live) != 1 || live[0].Position != 0 {
		t.Fatalf("expected one live variant at position 0, got %v %+v", err, live)
	}
}

func TestVariant_CrossItemNestedIDORIsRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	burger := env.seedItem(t, businessA, staffA, menu, section, "Burger", 400000)
	pizza := env.seedItem(t, businessA, staffA, menu, section, "Pizza", 500000)

	variant, err := env.service.CreateVariant(ctx, actor, globalScope(), burger.ID, CreateVariantInput{Name: "Single"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.UpdateVariant(ctx, actor, globalScope(), pizza.ID, variant.ID,
		UpdateVariantInput{Name: stringPtr("Hijacked"), ExpectedVersion: variant.Version}); errorCode(t, err) != CodeVariantNotFound {
		t.Fatalf("expected a variant mutated through the wrong item to be absent, got %v", err)
	}
	if _, err = env.service.ArchiveVariant(ctx, actor, globalScope(), pizza.ID, variant.ID, variant.Version); errorCode(t, err) != CodeVariantNotFound {
		t.Fatalf("expected archive through the wrong item to be absent, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Modifier groups, modifiers and attachments
// ---------------------------------------------------------------------------

func TestModifierGroup_SelectionLimits(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)

	group, err := env.service.CreateModifierGroup(ctx, actor, globalScope(),
		CreateModifierGroupInput{Name: "Choose your sauce", MinSelect: 1, MaxSelect: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !ModifierGroupViewOf(group).Required {
		t.Fatal("min_select > 0 must present as required")
	}
	optional := env.seedGroup(t, businessA, staffA, "Add extras", 0, 5)
	if ModifierGroupViewOf(optional).Required {
		t.Fatal("min_select = 0 must present as optional")
	}

	for name, in := range map[string]CreateModifierGroupInput{
		"min above max":  {Name: "Bad A", MinSelect: 3, MaxSelect: 2},
		"negative min":   {Name: "Bad B", MinSelect: -1, MaxSelect: 2},
		"zero max":       {Name: "Bad C", MinSelect: 0, MaxSelect: 0},
		"unbounded max":  {Name: "Bad D", MinSelect: 0, MaxSelect: 1000},
		"unbounded min":  {Name: "Bad E", MinSelect: 500, MaxSelect: 500},
		"empty name":     {Name: "  ", MinSelect: 0, MaxSelect: 1},
		"negative both":  {Name: "Bad F", MinSelect: -2, MaxSelect: -1},
		"max below zero": {Name: "Bad G", MinSelect: 0, MaxSelect: -3},
	} {
		if _, err = env.service.CreateModifierGroup(ctx, actor, globalScope(), in); status(t, err) != 422 {
			t.Errorf("%s should be refused, got %v", name, err)
		}
	}
}

func TestModifier_PriceDeltaRules(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	group := env.seedGroup(t, businessA, staffA, "Add extras", 0, 5)

	zero, err := env.service.CreateModifier(ctx, actor, globalScope(), group.ID,
		CreateModifierInput{Name: "No sauce", PriceDeltaMinor: 0})
	if err != nil {
		t.Fatalf("a zero delta is legitimate: %v", err)
	}
	cheese, err := env.service.CreateModifier(ctx, actor, globalScope(), group.ID,
		CreateModifierInput{Name: "Extra cheese", PriceDeltaMinor: 50000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.CreateModifier(ctx, actor, globalScope(), group.ID,
		CreateModifierInput{Name: "Discount", PriceDeltaMinor: -50000}); status(t, err) != 422 {
		t.Fatalf("a negative delta is a promotion and must be refused here, got %v", err)
	}

	reordered, err := env.service.ReorderModifiers(ctx, actor, globalScope(), group.ID, []string{cheese.ID, zero.ID})
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if reordered[0].ID != cheese.ID {
		t.Fatalf("unexpected order: %+v", reordered)
	}

	// A modifier addressed through a different group does not exist.
	other := env.seedGroup(t, businessA, staffA, "Choose drink", 1, 1)
	if _, err = env.service.UpdateModifier(ctx, actor, globalScope(), other.ID, cheese.ID,
		UpdateModifierInput{Name: stringPtr("Hijacked"), ExpectedVersion: cheese.Version}); errorCode(t, err) != CodeModifierNotFound {
		t.Fatalf("expected a cross-group modifier to be absent, got %v", err)
	}
}

func TestItemModifierGroup_AttachDetachReorder(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	burger := env.seedItem(t, businessA, staffA, menu, section, "Burger", 400000)
	pizza := env.seedItem(t, businessA, staffA, menu, section, "Pizza", 500000)
	extras := env.seedGroup(t, businessA, staffA, "Add extras", 0, 5)
	sauces := env.seedGroup(t, businessA, staffA, "Choose your sauce", 1, 1)

	if _, err := env.service.AttachModifierGroup(ctx, actor, globalScope(), burger.ID, extras.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.AttachModifierGroup(ctx, actor, globalScope(), burger.ID, sauces.ID); err != nil {
		t.Fatal(err)
	}
	// The same group on a second item is reuse, not duplication.
	if _, err := env.service.AttachModifierGroup(ctx, actor, globalScope(), pizza.ID, extras.ID); err != nil {
		t.Fatalf("a group must be reusable across items: %v", err)
	}
	if _, err := env.service.AttachModifierGroup(ctx, actor, globalScope(), burger.ID, extras.ID); errorCode(t, err) != CodeDuplicateRelationship {
		t.Fatalf("expected a duplicate attachment to be refused")
	}

	attached, err := env.service.ReorderAttachedModifierGroups(ctx, actor, globalScope(), burger.ID,
		[]string{sauces.ID, extras.ID})
	if err != nil {
		t.Fatalf("reorder attachments: %v", err)
	}
	if len(attached) != 2 || attached[0].ModifierGroupID != sauces.ID {
		t.Fatalf("unexpected attachment order: %+v", attached)
	}

	if err = env.service.DetachModifierGroup(ctx, actor, globalScope(), burger.ID, extras.ID); err != nil {
		t.Fatalf("detach: %v", err)
	}
	// Detaching removes only the relationship.
	group, err := env.service.GetModifierGroup(ctx, actor, extras.ID)
	if err != nil || group.Status != ChildActive {
		t.Fatalf("expected the group itself to survive detachment: %v %+v", err, group)
	}
	pizzaGroups, err := env.service.ListAttachedModifierGroups(ctx, actor, globalScope(), pizza.ID)
	if err != nil || len(pizzaGroups) != 1 {
		t.Fatalf("expected the other item's attachment to survive: %v %+v", err, pizzaGroups)
	}
	if err = env.service.DetachModifierGroup(ctx, actor, globalScope(), burger.ID, extras.ID); errorCode(t, err) != CodeModifierGroupNotFound {
		t.Fatalf("expected a second detach to report the relationship as absent, got %v", err)
	}
}

func TestItemModifierGroup_ArchivedEndsAreRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	burger := env.seedItem(t, businessA, staffA, menu, section, "Burger", 400000)
	extras := env.seedGroup(t, businessA, staffA, "Add extras", 0, 5)

	if _, err := env.service.AttachModifierGroup(ctx, actor, globalScope(), burger.ID, extras.ID); err != nil {
		t.Fatal(err)
	}
	archivedGroup, err := env.service.ArchiveModifierGroup(ctx, actor, globalScope(), extras.ID, extras.Version)
	if err != nil {
		t.Fatalf("archive group: %v", err)
	}
	if archivedGroup.Status != ChildArchived {
		t.Fatalf("unexpected status %s", archivedGroup.Status)
	}
	// Archiving detaches it everywhere, and it cannot be attached again.
	attached, err := env.service.ListAttachedModifierGroups(ctx, actor, globalScope(), burger.ID)
	if err != nil || len(attached) != 0 {
		t.Fatalf("expected the archived group to be detached: %v %+v", err, attached)
	}
	if _, err = env.service.AttachModifierGroup(ctx, actor, globalScope(), burger.ID, extras.ID); errorCode(t, err) != CodeArchived {
		t.Fatalf("expected an archived group to refuse attachment, got %v", err)
	}

	// An archived item accepts no new attachment either.
	other := env.seedGroup(t, businessA, staffA, "Choose drink", 1, 1)
	current, _, _, err := env.service.GetItem(ctx, actor, globalScope(), burger.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.ArchiveItem(ctx, actor, globalScope(), burger.ID, current.Version); err != nil {
		t.Fatalf("archive item: %v", err)
	}
	if _, err = env.service.AttachModifierGroup(ctx, actor, globalScope(), burger.ID, other.ID); errorCode(t, err) != CodeArchived {
		t.Fatalf("expected an archived item to refuse attachment, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Media
// ---------------------------------------------------------------------------

func TestItemMedia_OpaqueReferencesOnly(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, activeMedia("med_one", "med_two"))
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	item := env.seedItem(t, businessA, staffA, menu, section, "Burger", 400000)

	updated, media, err := env.service.ReplaceItemMedia(ctx, actor, globalScope(), item.ID,
		[]MediaRefInput{{MediaRef: "med_one"}, {MediaRef: "med_two", IsPrimary: true}}, item.Version)
	if err != nil {
		t.Fatalf("replace media: %v", err)
	}
	if len(media) != 2 || media[0].Position != 0 || media[1].Position != 1 {
		t.Fatalf("expected ordered references, got %+v", media)
	}
	if media[0].IsPrimary || !media[1].IsPrimary {
		t.Fatalf("expected the declared cover to be honoured, got %+v", media)
	}

	if _, _, err = env.service.ReplaceItemMedia(ctx, actor, globalScope(), item.ID,
		[]MediaRefInput{{MediaRef: "med_one"}, {MediaRef: "med_one"}}, updated.Version); status(t, err) != 422 {
		t.Fatalf("expected a duplicate reference to be refused, got %v", err)
	}
	if _, _, err = env.service.ReplaceItemMedia(ctx, actor, globalScope(), item.ID,
		[]MediaRefInput{{MediaRef: "med_one", IsPrimary: true}, {MediaRef: "med_two", IsPrimary: true}}, updated.Version); status(t, err) != 422 {
		t.Fatalf("expected two covers to be refused, got %v", err)
	}
	// A reference Media does not know about cannot be proven owned, so it is
	// refused rather than stored — the same shape Section 7 uses for onboarding
	// documents.
	if _, _, err = env.service.ReplaceItemMedia(ctx, actor, globalScope(), item.ID,
		[]MediaRefInput{{MediaRef: "med_unknown"}}, updated.Version); errorCode(t, err) != CodeMediaUnavailable {
		t.Fatalf("expected an unowned reference to be refused, got %v", err)
	}
	if _, _, err = env.service.ReplaceItemMedia(ctx, actor, globalScope(), item.ID,
		[]MediaRefInput{{MediaRef: "!! not an id !!"}}, updated.Version); status(t, err) != 422 {
		t.Fatalf("expected a malformed reference to be refused, got %v", err)
	}
	for _, ref := range []string{"med_quarantined", "med_pending"} {
		if _, _, err = env.service.ReplaceItemMedia(ctx, actor, globalScope(), item.ID,
			[]MediaRefInput{{MediaRef: ref}}, updated.Version); status(t, err) != 422 {
			t.Fatalf("expected %s to be refused, got %v", ref, err)
		}
	}
	overflow := make([]MediaRefInput, MaxMediaRefs+1)
	for index := range overflow {
		overflow[index] = MediaRefInput{MediaRef: "med_one"}
	}
	if _, _, err = env.service.ReplaceItemMedia(ctx, actor, globalScope(), item.ID, overflow, updated.Version); status(t, err) != 422 {
		t.Fatalf("expected too many references to be refused, got %v", err)
	}

	// An undeclared cover is chosen deterministically rather than left absent.
	_, media, err = env.service.ReplaceItemMedia(ctx, actor, globalScope(), item.ID,
		[]MediaRefInput{{MediaRef: "med_two"}, {MediaRef: "med_one"}}, updated.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !media[0].IsPrimary || media[0].MediaRef != "med_two" {
		t.Fatalf("expected the first reference to become the cover, got %+v", media)
	}
}

func TestItemMedia_WithoutMediaAuthorityTheWriteIsRefused(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Mains")
	item := env.seedItem(t, businessA, staffA, menu, section, "Burger", 400000)

	// Fail closed: an unverified reference is worse than no reference.
	if _, _, err := env.service.ReplaceItemMedia(ctx, actor, globalScope(), item.ID,
		[]MediaRefInput{{MediaRef: "med_one"}}, item.Version); errorCode(t, err) != CodeMediaUnavailable {
		t.Fatalf("expected the write to be refused without media validation, got %v", err)
	}
	// Clearing the list needs no authority, because it asserts nothing.
	if _, _, err := env.service.ReplaceItemMedia(ctx, actor, globalScope(), item.ID, nil, item.Version); err != nil {
		t.Fatalf("clearing media must remain possible: %v", err)
	}
}
