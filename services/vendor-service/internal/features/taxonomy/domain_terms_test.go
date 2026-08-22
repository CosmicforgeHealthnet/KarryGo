package taxonomy

import "testing"

func TestNormalizeVocabularyTerm(t *testing.T) {
	tests := map[string]string{
		"  GARI  ":       "gari",
		"MOIN   MOIN":    "moin moin",
		"PUFF-PUFF":      "puff-puff",
		"PUFF PUFF":      "puff puff",
		"Paint   Rubber": "paint rubber",
	}

	for input, want := range tests {
		if got := NormalizeVocabularyTerm(input); got != want {
			t.Fatalf("NormalizeVocabularyTerm(%q)=%q want %q", input, got, want)
		}
	}
}

func TestTermRelationshipTypesRemainDistinct(t *testing.T) {
	values := []TermRelationshipType{
		TermRelationshipAlias,
		TermRelationshipSynonym,
		TermRelationshipRelatedTerm,
	}

	seen := map[TermRelationshipType]bool{}

	for _, value := range values {
		if !value.Valid() {
			t.Fatalf("%q should be valid", value)
		}
		if seen[value] {
			t.Fatalf("duplicate relationship type %q", value)
		}
		seen[value] = true
	}

	if TermRelationshipType("RELATED").Valid() {
		t.Fatal("RELATED must not be accepted as RELATED_TERM")
	}
}

func TestDeterministicTermID(t *testing.T) {
	seed := SeedNodeTerm{
		NamespaceCode:    NamespaceFood,
		NodeCode:         "MOI_MOI",
		Term:             "Moin   Moin",
		RelationshipType: TermRelationshipAlias,
		Locale:           "en-NG",
	}

	first := DeterministicTermID(seed)
	second := DeterministicTermID(seed)

	if first == "" || first != second {
		t.Fatalf("deterministic IDs differ: %q / %q", first, second)
	}

	shop := seed
	shop.NamespaceCode = NamespaceShop

	if DeterministicTermID(shop) == first {
		t.Fatal("FOOD and SHOP vocabulary must have different deterministic IDs")
	}
}

func TestSeedNodeTermValidation(t *testing.T) {
	valid := SeedNodeTerm{
		NamespaceCode:    NamespaceFood,
		NodeCode:         "GROUNDNUT",
		Term:             "Peanut",
		RelationshipType: TermRelationshipSynonym,
		Locale:           "en-NG",
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("valid seed rejected: %v", err)
	}

	invalid := valid
	invalid.RelationshipType = TermRelationshipType("ALIAS_OR_SYNONYM")

	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid semantic relationship must be rejected")
	}
}
