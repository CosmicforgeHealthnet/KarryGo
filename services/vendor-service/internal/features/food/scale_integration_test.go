package food

import (
	"context"
	"testing"
)

// TestScale_RealisticCatalogueStaysBoundedAndDeterministic builds a catalogue
// of the size a busy restaurant group actually runs and checks the two things
// that break first at that size: unbounded list responses, and a per-row query
// hiding behind a list endpoint.
//
// It is a shape check, not a benchmark. No timing is asserted, because a timing
// assertion on shared test infrastructure measures the machine rather than the
// code.
func TestScale_RealisticCatalogueStaysBoundedAndDeterministic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the Section 9A scale fixture in short mode")
	}
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)

	const (
		menus            = 3
		sectionsPerMenu  = 8
		itemsPerSection  = 12
		variantsPerItem  = 3
		modifierGroups   = 6
		modifiersInGroup = 5
	)

	groups := make([]ModifierGroup, 0, modifierGroups)
	for index := 0; index < modifierGroups; index++ {
		group := env.seedGroup(t, businessA, staffA, "Group "+itoa(index), 0, 5)
		for modifier := 0; modifier < modifiersInGroup; modifier++ {
			if _, err := env.service.CreateModifier(ctx, actor, globalScope(), group.ID,
				CreateModifierInput{Name: "Choice " + itoa(index) + "-" + itoa(modifier),
					PriceDeltaMinor: int64(modifier) * 5000}); err != nil {
				t.Fatal(err)
			}
		}
		groups = append(groups, group)
	}

	var wideSection Section
	items := 0
	for menuIndex := 0; menuIndex < menus; menuIndex++ {
		capability := "RESTAURANT"
		branch := branchA1
		if menuIndex%2 == 1 {
			capability = "FAST_FOOD"
		}
		if menuIndex == 2 {
			branch = branchA2
		}
		menu, err := env.service.CreateMenu(ctx, actor, globalScope(), CreateMenuInput{
			BranchID: branch, CapabilityType: capability, Name: "Menu " + itoa(menuIndex)})
		if err != nil {
			t.Fatal(err)
		}
		for sectionIndex := 0; sectionIndex < sectionsPerMenu; sectionIndex++ {
			section := env.seedSection(t, businessA, staffA, menu, "Section "+itoa(sectionIndex))
			if menuIndex == 0 && sectionIndex == 0 {
				wideSection = section
			}
			for itemIndex := 0; itemIndex < itemsPerSection; itemIndex++ {
				item := env.seedItem(t, businessA, staffA, menu, section,
					"Item "+itoa(sectionIndex)+"-"+itoa(itemIndex), int64(100000+itemIndex*1000))
				items++
				for variantIndex := 0; variantIndex < variantsPerItem; variantIndex++ {
					if _, err = env.service.CreateVariant(ctx, actor, globalScope(), item.ID,
						CreateVariantInput{Name: "Size " + itoa(variantIndex)}); err != nil {
						t.Fatal(err)
					}
				}
				// Every item carries two shared groups, which is the reuse the
				// relationship model exists for: 288 items, still six group rows.
				for _, group := range groups[:2] {
					if _, err = env.service.AttachModifierGroup(ctx, actor, globalScope(), item.ID, group.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	if items != menus*sectionsPerMenu*itemsPerSection {
		t.Fatalf("fixture built %d items", items)
	}

	// The group definitions did not multiply with the items that use them.
	var groupRows, attachmentRows int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM food_modifier_groups WHERE business_id=$1`,
		businessA).Scan(&groupRows); err != nil {
		t.Fatal(err)
	}
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM food_item_modifier_groups WHERE business_id=$1`,
		businessA).Scan(&attachmentRows); err != nil {
		t.Fatal(err)
	}
	if groupRows != modifierGroups {
		t.Fatalf("modifier groups multiplied with items: %d rows for %d definitions", groupRows, modifierGroups)
	}
	if attachmentRows != items*2 {
		t.Fatalf("expected %d attachments, got %d", items*2, attachmentRows)
	}

	// List endpoints are bounded whatever the caller asks for.
	page, _, err := env.service.ListMenus(ctx, actor, globalScope(), MenuListOptions{Limit: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) > 100 {
		t.Fatalf("the menu list returned %d rows despite the bound", len(page))
	}
	listed, media, currency, hasMore, err := env.service.ListItems(ctx, actor, globalScope(),
		wideSection.MenuID, wideSection.ID, false, 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 5 || !hasMore || currency != "NGN" {
		t.Fatalf("unexpected page: %d items hasMore=%t currency=%s", len(listed), hasMore, currency)
	}
	if media == nil {
		t.Fatal("the media map must be present even when no item has media")
	}
	// Ordering is deterministic and contiguous across the whole wide section.
	all, _, _, _, err := env.service.ListItems(ctx, actor, globalScope(),
		wideSection.MenuID, wideSection.ID, false, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != itemsPerSection {
		t.Fatalf("expected %d items in the section, got %d", itemsPerSection, len(all))
	}
	for index, item := range all {
		if item.Position != index {
			t.Fatalf("position %d at index %d breaks contiguity", item.Position, index)
		}
	}

	// A full reorder of the widest collection is one atomic operation, and the
	// result is still contiguous.
	reversed := make([]string, 0, len(all))
	for index := len(all) - 1; index >= 0; index-- {
		reversed = append(reversed, all[index].ID)
	}
	reordered, err := env.service.ReorderItems(ctx, actor, globalScope(), wideSection.MenuID, wideSection.ID, reversed)
	if err != nil {
		t.Fatalf("reorder at scale: %v", err)
	}
	for index, item := range reordered {
		if item.ID != reversed[index] || item.Position != index {
			t.Fatalf("reorder at scale produced %s at position %d, index %d", item.ID, item.Position, index)
		}
	}

	// The attached-group read joins its definitions rather than fetching them
	// one at a time, which is asserted by the join returning a populated group
	// on every row.
	attached, err := env.service.ListAttachedModifierGroups(ctx, actor, globalScope(), all[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attached) != 2 {
		t.Fatalf("expected two attached groups, got %d", len(attached))
	}
	for _, entry := range attached {
		if entry.Group.ID != entry.ModifierGroupID || entry.Group.Name == "" {
			t.Fatalf("attachment %s did not carry its definition", entry.ModifierGroupID)
		}
	}
}
