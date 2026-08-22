package taxonomy

import (
	"os"
	"strings"
	"testing"
)

func loadFoodSeedForTest(t *testing.T) SeedBundle {
	t.Helper()

	file, err := os.Open("seeds/food.v1.json")
	if err != nil {
		t.Fatalf("open Food seed: %v", err)
	}
	defer file.Close()

	bundle, err := DecodeSeedBundle(file)
	if err != nil {
		t.Fatalf("decode Food seed: %v", err)
	}
	return bundle
}

func TestFoodSeedV1Contract(t *testing.T) {
	bundle := loadFoodSeedForTest(t)

	if bundle.NamespaceCode != NamespaceFood {
		t.Fatalf("namespace=%s want FOOD", bundle.NamespaceCode)
	}
	if bundle.SeedKey != "KARRYGO_FOOD_V1" {
		t.Fatalf("seed key=%s", bundle.SeedKey)
	}
	if len(bundle.Facets) != 5 {
		t.Fatalf("Food facets=%d want=5", len(bundle.Facets))
	}
	if len(bundle.Nodes) != 60 {
		t.Fatalf("Food nodes=%d want=60", len(bundle.Nodes))
	}

	requiredFacets := map[string]FacetStructureType{
		"PROVIDER_TYPE": FacetStructureFlat,
		"CUISINE":       FacetStructureHierarchical,
		"MEAL_PERIOD":   FacetStructureFlat,
		"DISH_TYPE":     FacetStructureHierarchical,
		"OCCASION":      FacetStructureFlat,
	}

	for _, facet := range bundle.Facets {
		want, exists := requiredFacets[facet.Code]
		if !exists {
			t.Fatalf("unexpected Food facet %s", facet.Code)
		}
		if facet.StructureType != want {
			t.Fatalf("facet %s structure=%s want=%s", facet.Code, facet.StructureType, want)
		}
		delete(requiredFacets, facet.Code)
	}
	if len(requiredFacets) != 0 {
		t.Fatalf("missing Food facets: %v", requiredFacets)
	}
}

func TestFoodSeedContainsRequiredDiscoveryVocabulary(t *testing.T) {
	bundle := loadFoodSeedForTest(t)

	required := []string{
		"FOOD_PROVIDER_TYPE_RESTAURANT",
		"FOOD_PROVIDER_TYPE_FAST_FOOD",
		"FOOD_PROVIDER_TYPE_CLOUD_KITCHEN",
		"FOOD_PROVIDER_TYPE_PASTRY_BAKERY",
		"FOOD_CUISINE_AFRICAN_FOOD",
		"FOOD_CUISINE_NIGERIAN_LOCAL_DISHES",
		"FOOD_CUISINE_CONTINENTAL_INTERNATIONAL",
		"FOOD_MEAL_PERIOD_BREAKFAST_BRUNCH",
		"FOOD_MEAL_PERIOD_LUNCH",
		"FOOD_MEAL_PERIOD_DINNER",
		"FOOD_MEAL_PERIOD_LATE_NIGHT",
		"FOOD_DISH_TYPE_RICE_MEALS",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW",
		"FOOD_DISH_TYPE_GRILLS_BBQ",
		"FOOD_DISH_TYPE_PIZZA",
		"FOOD_DISH_TYPE_BURGERS",
		"FOOD_DISH_TYPE_SHAWARMA",
		"FOOD_DISH_TYPE_CAKES_CELEBRATIONS",
		"FOOD_DISH_TYPE_PASTRIES",
		"FOOD_DISH_TYPE_SMALL_CHOPS",
		"FOOD_DISH_TYPE_SNACKS",
		"FOOD_DISH_TYPE_DESSERTS",
		"FOOD_DISH_TYPE_ICE_CREAM",
		"FOOD_DISH_TYPE_DRINKS",
		"FOOD_DISH_TYPE_JUICES",
		"FOOD_DISH_TYPE_SMOOTHIES",
		"FOOD_DISH_TYPE_HEALTHY_MEALS",
		"FOOD_OCCASION_FAMILY_MEALS",
		"FOOD_OCCASION_GROUP_ORDERS",
		"FOOD_OCCASION_BULK_FOOD",
		"FOOD_OCCASION_CORPORATE_MEALS",
		"FOOD_OCCASION_SCHEDULED_FOOD",
		"FOOD_OCCASION_CELEBRATION_FOOD",
		"FOOD_OCCASION_PARTY_PACKS",
		"FOOD_DISH_TYPE_RICE_MEALS_JOLLOF_RICE",
		"FOOD_DISH_TYPE_RICE_MEALS_FRIED_RICE",
		"FOOD_DISH_TYPE_RICE_MEALS_OFADA_RICE",
		"FOOD_DISH_TYPE_RICE_MEALS_NATIVE_RICE",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_EGUSI_SOUP",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_OGBONO_SOUP",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_AFANG",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_EDIKANG_IKONG",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_BANGA",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_OHA",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_NSALA",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_FISHERMAN_SOUP",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_EBA",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_AMALA",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_POUNDED_YAM",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_SEMO",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_FUFU",
		"FOOD_DISH_TYPE_SOUPS_SWALLOW_TUWO",
		"FOOD_DISH_TYPE_GRILLS_BBQ_SUYA",
		"FOOD_DISH_TYPE_GRILLS_BBQ_KILISHI",
		"FOOD_DISH_TYPE_SNACKS_AKARA",
		"FOOD_DISH_TYPE_SNACKS_MOI_MOI",
		"FOOD_DISH_TYPE_SNACKS_PUFF_PUFF",
		"FOOD_DISH_TYPE_SNACKS_CHIN_CHIN",
	}

	found := make(map[string]bool, len(bundle.Nodes))
	for _, node := range bundle.Nodes {
		found[node.CanonicalCode] = true
	}

	for _, code := range required {
		if !found[code] {
			t.Fatalf("required Food taxonomy code missing: %s", code)
		}
	}
}

func TestFoodMealPeriodsAreDiscoveryOnly(t *testing.T) {
	bundle := loadFoodSeedForTest(t)

	count := 0
	for _, node := range bundle.Nodes {
		if node.FacetCode != "MEAL_PERIOD" {
			continue
		}
		count++

		discoveryOnly, ok := node.Metadata["discovery_only"].(bool)
		if !ok || !discoveryOnly {
			t.Fatalf("%s must be discovery-only", node.CanonicalCode)
		}

		openAuthority, ok := node.Metadata["branch_opening_authority"].(bool)
		if !ok || openAuthority {
			t.Fatalf("%s must not be branch-opening authority", node.CanonicalCode)
		}
	}

	if count != 4 {
		t.Fatalf("meal periods=%d want=4", count)
	}
}

func TestFoodSeedIsDataDrivenAndExpandable(t *testing.T) {
	bundle := loadFoodSeedForTest(t)

	bundle.Nodes = append(bundle.Nodes, SeedNodeDefinition{
		CanonicalCode: "FOOD_CUISINE_ITALIAN",
		FacetCode:     "CUISINE",
		DisplayName:   "Italian",
		ParentCode:    "FOOD_CUISINE_CONTINENTAL_INTERNATIONAL",
		Position:      100,
		Metadata:      map[string]any{"kind": "CUISINE"},
	})

	bundle.Nodes = append(bundle.Nodes, SeedNodeDefinition{
		CanonicalCode: "FOOD_DISH_TYPE_NOODLES_PASTA",
		FacetCode:     "DISH_TYPE",
		DisplayName:   "Noodles & Pasta",
		Position:      200,
		Metadata:      map[string]any{"kind": "GROUP"},
	})

	if err := bundle.Validate(); err != nil {
		t.Fatalf("generic taxonomy engine rejected data-only expansion: %v", err)
	}
}

func TestFoodSeedRejectsCrossNamespaceAndCycles(t *testing.T) {
	bundle := loadFoodSeedForTest(t)

	badNamespace := bundle
	badNamespace.Nodes = append(badNamespace.Nodes, SeedNodeDefinition{
		CanonicalCode: "SHOP_CATEGORY_BAD",
		FacetCode:     "DISH_TYPE",
		DisplayName:   "Bad",
		Position:      999,
	})
	if err := badNamespace.Validate(); err == nil || !strings.Contains(err.Error(), "outside namespace") {
		t.Fatalf("cross-namespace seed node must fail, got %v", err)
	}

	cyclic := bundle
	for i := range cyclic.Nodes {
		switch cyclic.Nodes[i].CanonicalCode {
		case "FOOD_DISH_TYPE_RICE_MEALS":
			cyclic.Nodes[i].ParentCode = "FOOD_DISH_TYPE_RICE_MEALS_JOLLOF_RICE"
		}
	}
	if err := cyclic.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cyclic Food seed must fail, got %v", err)
	}
}
