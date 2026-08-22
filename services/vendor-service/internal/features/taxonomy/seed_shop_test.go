package taxonomy

import (
	"os"
	"testing"
)

func loadShopSeedForTest(t *testing.T) SeedBundle {
	t.Helper()

	file, err := os.Open("seeds/shop.v1.json")
	if err != nil {
		t.Fatalf("open Shop seed: %v", err)
	}
	defer file.Close()

	bundle, err := DecodeSeedBundle(file)
	if err != nil {
		t.Fatalf("decode Shop seed: %v", err)
	}
	return bundle
}

func TestShopSeedV1Contract(t *testing.T) {
	bundle := loadShopSeedForTest(t)

	if bundle.NamespaceCode != NamespaceShop {
		t.Fatalf("namespace=%s want SHOP", bundle.NamespaceCode)
	}
	if bundle.SeedKey != "KARRYGO_SHOP_V1" {
		t.Fatalf("seed key=%s", bundle.SeedKey)
	}
	if len(bundle.Facets) != 1 {
		t.Fatalf("Shop facets=%d want=1", len(bundle.Facets))
	}
	if bundle.Facets[0].Code != "CATEGORY" ||
		bundle.Facets[0].StructureType != FacetStructureHierarchical {
		t.Fatalf("Shop CATEGORY facet invalid: %+v", bundle.Facets[0])
	}
	if len(bundle.Nodes) != 32 {
		t.Fatalf("Shop nodes=%d want=32", len(bundle.Nodes))
	}
	if len(bundle.AuthorityVocabulary) != 17 {
		t.Fatalf("measurement vocabulary=%d want=17", len(bundle.AuthorityVocabulary))
	}
}

func TestShopSeedContainsRequiredCanonicalHierarchy(t *testing.T) {
	bundle := loadShopSeedForTest(t)

	required := []string{
		"SHOP_CATEGORY_FOODSTUFF",
		"SHOP_CATEGORY_FRESH_PRODUCE",
		"SHOP_CATEGORY_PROTEIN",
		"SHOP_CATEGORY_COOKING_ESSENTIALS",
		"SHOP_CATEGORY_FROZEN_CHILLED",
		"SHOP_CATEGORY_BEVERAGES",
		"SHOP_CATEGORY_DAIRY_EGGS",
		"SHOP_CATEGORY_PACKAGED_GROCERY",
		"SHOP_CATEGORY_BULK_WHOLESALE",
		"SHOP_CATEGORY_FOODSTUFF_RICE_GRAINS",
		"SHOP_CATEGORY_FOODSTUFF_BEANS_PULSES",
		"SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA",
		"SHOP_CATEGORY_FOODSTUFF_YAM_TUBERS",
		"SHOP_CATEGORY_FOODSTUFF_PLANTAIN",
		"SHOP_CATEGORY_PROTEIN_MEAT_POULTRY",
		"SHOP_CATEGORY_PROTEIN_FISH_SEAFOOD",
		"SHOP_CATEGORY_FRESH_PRODUCE_VEGETABLES",
		"SHOP_CATEGORY_FRESH_PRODUCE_FRUITS",
		"SHOP_CATEGORY_COOKING_ESSENTIALS_COOKING_OILS",
		"SHOP_CATEGORY_COOKING_ESSENTIALS_SPICES_SEASONING",
		"SHOP_CATEGORY_COOKING_ESSENTIALS_FLOUR_BAKING",
		"SHOP_CATEGORY_FROZEN_CHILLED_FROZEN_FOODS",
		"SHOP_CATEGORY_BEVERAGES_DRINKS_WATER",
		"SHOP_CATEGORY_DAIRY_EGGS_DAIRY",
		"SHOP_CATEGORY_DAIRY_EGGS_EGGS",
		"SHOP_CATEGORY_PACKAGED_GROCERY_CANNED_FOODS",
		"SHOP_CATEGORY_PACKAGED_GROCERY_PACKAGED_GROCERIES",
		"SHOP_CATEGORY_BULK_WHOLESALE_FOODSTUFF",
	}

	found := make(map[string]bool, len(bundle.Nodes))
	for _, node := range bundle.Nodes {
		found[node.CanonicalCode] = true
	}

	for _, code := range required {
		if !found[code] {
			t.Fatalf("required Shop taxonomy code missing: %s", code)
		}
	}
}

func TestShopMeasurementVocabularyReferencesSection12Authority(t *testing.T) {
	bundle := loadShopSeedForTest(t)

	expected := map[string]string{
		"PAINT":        "smd_paint",
		"PAINT_RUBBER": "smd_paint_rubber",
		"DERICA":       "smd_derica",
		"MUDU":         "smd_mudu",
		"CONGO":        "smd_congo",
		"TIYA":         "smd_tiya",
		"KOBIOWU":      "smd_kobiowu",
		"BASKET":       "smd_basket",
		"BOWL":         "smd_bowl",
		"BASIN":        "smd_basin",
		"BUNCH":        "smd_bunch",
		"TUBER":        "smd_tuber",
		"BAG":          "smd_bag",
		"SACK":         "smd_sack",
		"CARTON":       "smd_carton",
		"CRATE":        "smd_crate",
		"JERRYCAN":     "smd_jerrycan",
	}

	if len(bundle.AuthorityVocabulary) != len(expected) {
		t.Fatalf("authority vocabulary=%d want=%d", len(bundle.AuthorityVocabulary), len(expected))
	}

	for _, item := range bundle.AuthorityVocabulary {
		if item.AuthorityType != "SECTION12_MEASUREMENT" {
			t.Fatalf("unexpected authority type %s", item.AuthorityType)
		}

		wantRef, exists := expected[item.AuthorityCode]
		if !exists {
			t.Fatalf("unexpected Section 12 measurement code %s", item.AuthorityCode)
		}
		if item.AuthorityRef != wantRef {
			t.Fatalf("%s authority ref=%s want=%s", item.AuthorityCode, item.AuthorityRef, wantRef)
		}

		if item.Metadata["authority"] != "SECTION_12" {
			t.Fatalf("%s does not declare Section 12 authority", item.AuthorityCode)
		}
		if item.Metadata["source_table"] != "shop_measure_definitions" {
			t.Fatalf("%s source table changed", item.AuthorityCode)
		}
		if item.Metadata["read_only_reference"] != true {
			t.Fatalf("%s must remain read-only reference", item.AuthorityCode)
		}
	}
}

func TestShopMeasurementVocabularyDoesNotInventHalfPaint(t *testing.T) {
	bundle := loadShopSeedForTest(t)

	for _, item := range bundle.AuthorityVocabulary {
		if item.AuthorityCode == "HALF_PAINT" || item.AuthorityRef == "smd_half_paint" {
			t.Fatal("Half Paint must be a fraction of PAINT, not a new measurement definition")
		}
	}

	var paintFound bool
	for _, item := range bundle.AuthorityVocabulary {
		if item.AuthorityCode != "PAINT" {
			continue
		}
		paintFound = true
		if item.Metadata["fractional_composition"] != true {
			t.Fatal("PAINT must advertise fractional query composition")
		}
	}
	if !paintFound {
		t.Fatal("PAINT vocabulary reference missing")
	}
}

func TestShopAuthorityVocabularyRejectsFoodScope(t *testing.T) {
	shop := loadShopSeedForTest(t)
	shop.NamespaceCode = NamespaceFood

	if err := shop.Validate(); err == nil {
		t.Fatal("Section 12 measurement vocabulary must not be accepted in FOOD scope")
	}
}

func TestShopAuthorityVocabularyHasNoDuplicateNormalizedTerms(t *testing.T) {
	bundle := loadShopSeedForTest(t)

	seen := map[string]bool{}
	for _, item := range bundle.AuthorityVocabulary {
		term := item.NormalizedTerm()
		if seen[term] {
			t.Fatalf("duplicate normalized measurement vocabulary term %q", term)
		}
		seen[term] = true
	}
}

func TestShopSeedIsDataDrivenAndExpandable(t *testing.T) {
	bundle := loadShopSeedForTest(t)

	bundle.Nodes = append(bundle.Nodes, SeedNodeDefinition{
		CanonicalCode: "SHOP_CATEGORY_FOODSTUFF_MAIZE_CORN",
		FacetCode:     "CATEGORY",
		DisplayName:   "Maize & Corn",
		ParentCode:    "SHOP_CATEGORY_FOODSTUFF",
		Position:      100,
		Metadata:      map[string]any{"kind": "CATEGORY"},
	})

	if err := bundle.Validate(); err != nil {
		t.Fatalf("data-only Shop category expansion rejected: %v", err)
	}
}
