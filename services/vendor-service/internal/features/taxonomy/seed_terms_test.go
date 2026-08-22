package taxonomy

import "testing"

func TestSection13B4NigerianVocabularyContract(t *testing.T) {
	food := loadFoodSeedForTest(t)
	shop := loadShopSeedForTest(t)

	if len(food.Nodes) != 60 {
		t.Fatalf("Food nodes=%d want=60", len(food.Nodes))
	}

	if len(shop.Nodes) != 32 {
		t.Fatalf("Shop nodes=%d want=32", len(shop.Nodes))
	}

	if len(food.Terms) != 7 {
		t.Fatalf("Food terms=%d want=7", len(food.Terms))
	}

	if len(shop.Terms) != 5 {
		t.Fatalf("Shop terms=%d want=5", len(shop.Terms))
	}

	required := map[string]TermRelationshipType{
		"Moin Moin":    TermRelationshipAlias,
		"Moimoi":       TermRelationshipAlias,
		"Moi-Moi":      TermRelationshipAlias,
		"Puff-Puff":    TermRelationshipAlias,
		"Pap":          TermRelationshipSynonym,
		"Ogi":          TermRelationshipSynonym,
		"Grilled Meat": TermRelationshipRelatedTerm,
		"Gari":         TermRelationshipAlias,
		"Okro":         TermRelationshipAlias,
		"Kpomo":        TermRelationshipAlias,
		"Kanda":        TermRelationshipAlias,
		"Peanut":       TermRelationshipSynonym,
	}

	for _, bundle := range []SeedBundle{food, shop} {
		for _, term := range bundle.Terms {
			want, exists := required[term.Term]
			if !exists {
				t.Fatalf(
					"unexpected seed vocabulary %q",
					term.Term,
				)
			}

			if term.RelationshipType != want {
				t.Fatalf(
					"%s relationship=%s want=%s",
					term.Term,
					term.RelationshipType,
					want,
				)
			}

			delete(required, term.Term)
		}
	}

	if len(required) != 0 {
		t.Fatalf("missing vocabulary terms: %v", required)
	}
}

func TestSection13B4GarriAndEbaRemainSeparate(t *testing.T) {
	food := loadFoodSeedForTest(t)
	shop := loadShopSeedForTest(t)

	var shopGarri bool
	var foodEba bool

	for _, node := range shop.Nodes {
		if node.CanonicalCode ==
			"SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA_GARRI" {
			shopGarri = true
		}

		if node.DisplayName == "Eba" {
			t.Fatal("Eba leaked into SHOP taxonomy")
		}
	}

	for _, node := range food.Nodes {
		if node.CanonicalCode ==
			"FOOD_DISH_TYPE_SOUPS_SWALLOW_EBA" {
			foodEba = true
		}

		if node.DisplayName == "Garri" {
			t.Fatal("Garri leaked into FOOD taxonomy")
		}
	}

	if !shopGarri || !foodEba {
		t.Fatal("required Garri/Eba canonical concepts missing")
	}

	for _, term := range food.Terms {
		normalized := NormalizeVocabularyTerm(term.Term)

		if normalized == "garri" || normalized == "gari" {
			t.Fatal(
				"Garri/Gari must not classify Eba in FOOD",
			)
		}
	}
}

func TestSection13B4RelatedTermIsNotExactEquivalence(
	t *testing.T,
) {
	food := loadFoodSeedForTest(t)

	for _, term := range food.Terms {
		if term.Term != "Grilled Meat" {
			continue
		}

		if term.RelationshipType !=
			TermRelationshipRelatedTerm {
			t.Fatal(
				"Grilled Meat must remain RELATED_TERM",
			)
		}

		if term.Metadata["exact_equivalence"] != false {
			t.Fatal(
				"RELATED_TERM must not claim exact equivalence",
			)
		}

		return
	}

	t.Fatal("RELATED_TERM fixture missing")
}

func TestSection13B4TermIDsAreVersionScoped(t *testing.T) {
	seed := SeedNodeTerm{
		NamespaceCode:    NamespaceShop,
		NodeCode:         "SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA_GARRI",
		Term:             "Gari",
		RelationshipType: TermRelationshipAlias,
		Locale:           "en-NG",
		VersionKey:       "v1",
	}

	first := DeterministicTermID(seed)

	seed.VersionKey = "v2"

	second := DeterministicTermID(seed)

	if first == second {
		t.Fatal(
			"different taxonomy versions must not reuse term IDs",
		)
	}
}
