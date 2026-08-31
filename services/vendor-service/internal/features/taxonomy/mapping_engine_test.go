package taxonomy

import (
	"encoding/json"
	"testing"
)

func TestSection13C3MappingRequestNormalization(
	t *testing.T,
) {
	score := 9300

	normalized, hash, err :=
		prepareReplaceFacetMappingsInput(
			ReplaceFacetMappingsInput{
				NamespaceCode: NamespaceFood,

				FacetCode: "dish_type",

				SourceType: "catalog_item",

				SourceRef: " item-123 ",

				Mappings: []MappingCandidate{
					{
						NodeCode: "food_dish_type_snacks_puff_puff",

						MappingType: MappingTypeSecondary,

						ConfidenceBasis: MappingConfidenceRule,

						ConfidenceScore: &score,

						Metadata: json.RawMessage(
							`{"source":"rule"}`,
						),
					},
					{
						NodeCode: "food_dish_type_snacks",

						MappingType: MappingTypePrimary,
					},
				},
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if normalized.FacetCode !=
		"DISH_TYPE" {
		t.Fatalf(
			"facet=%s",
			normalized.FacetCode,
		)
	}

	if normalized.SourceType !=
		"CATALOG_ITEM" {
		t.Fatalf(
			"source type=%s",
			normalized.SourceType,
		)
	}

	if normalized.SourceRef !=
		"item-123" {
		t.Fatalf(
			"source ref=%s",
			normalized.SourceRef,
		)
	}

	if hash == "" {
		t.Fatal("request hash missing")
	}

	if len(normalized.Mappings) != 2 {
		t.Fatalf(
			"mappings=%d want=2",
			len(normalized.Mappings),
		)
	}

	if normalized.Mappings[0].
		ConfidenceBasis == "" {
		t.Fatal(
			"default confidence basis missing",
		)
	}
}

func TestSection13C3HashStableAcrossMappingOrder(
	t *testing.T,
) {
	first := ReplaceFacetMappingsInput{
		NamespaceCode: NamespaceFood,

		FacetCode: "DISH_TYPE",

		SourceType: "CATALOG_ITEM",

		SourceRef: "item-1",

		Mappings: []MappingCandidate{
			{
				NodeCode: "FOOD_DISH_TYPE_SNACKS",

				MappingType: MappingTypePrimary,
			},
			{
				NodeCode: "FOOD_DISH_TYPE_SNACKS_PUFF_PUFF",

				MappingType: MappingTypeSecondary,
			},
		},
	}

	second := first

	second.Mappings = []MappingCandidate{
		first.Mappings[1],
		first.Mappings[0],
	}

	_, firstHash, err :=
		prepareReplaceFacetMappingsInput(first)
	if err != nil {
		t.Fatal(err)
	}

	_, secondHash, err :=
		prepareReplaceFacetMappingsInput(second)
	if err != nil {
		t.Fatal(err)
	}

	if firstHash != secondHash {
		t.Fatal(
			"mapping order changed request hash",
		)
	}
}

func TestSection13C3RejectsMultiplePrimaryMappings(
	t *testing.T,
) {
	_, _, err :=
		prepareReplaceFacetMappingsInput(
			ReplaceFacetMappingsInput{
				NamespaceCode: NamespaceFood,

				FacetCode: "DISH_TYPE",

				SourceType: "CATALOG_ITEM",

				SourceRef: "item-1",

				Mappings: []MappingCandidate{
					{
						NodeCode: "FOOD_DISH_TYPE_SNACKS",

						MappingType: MappingTypePrimary,
					},
					{
						NodeCode: "FOOD_DISH_TYPE_DESSERTS",

						MappingType: MappingTypePrimary,
					},
				},
			},
		)

	if err == nil {
		t.Fatal(
			"multiple PRIMARY mappings accepted",
		)
	}
}

func TestSection13C3RejectsDuplicateNode(
	t *testing.T,
) {
	_, _, err :=
		prepareReplaceFacetMappingsInput(
			ReplaceFacetMappingsInput{
				NamespaceCode: NamespaceShop,

				FacetCode: "CATEGORY",

				SourceType: "CATALOG_ITEM",

				SourceRef: "item-2",

				Mappings: []MappingCandidate{
					{
						NodeCode: "SHOP_CATEGORY_FOODSTUFF",

						MappingType: MappingTypePrimary,
					},
					{
						NodeCode: "SHOP_CATEGORY_FOODSTUFF",

						MappingType: MappingTypeSecondary,
					},
				},
			},
		)

	if err == nil {
		t.Fatal(
			"duplicate node mapping accepted",
		)
	}
}

func TestSection13C3AllowsEmptyReplacementSet(
	t *testing.T,
) {
	normalized, _, err :=
		prepareReplaceFacetMappingsInput(
			ReplaceFacetMappingsInput{
				NamespaceCode: NamespaceShop,

				FacetCode: "CATEGORY",

				SourceType: "CATALOG_ITEM",

				SourceRef: "item-clear",

				Mappings: []MappingCandidate{},
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if len(normalized.Mappings) != 0 {
		t.Fatal(
			"empty mapping replacement changed",
		)
	}
}

func TestSection13C3RejectsInvalidConfidence(
	t *testing.T,
) {
	score := 10001

	_, _, err :=
		prepareReplaceFacetMappingsInput(
			ReplaceFacetMappingsInput{
				NamespaceCode: NamespaceFood,

				FacetCode: "DISH_TYPE",

				SourceType: "CATALOG_ITEM",

				SourceRef: "item-score",

				Mappings: []MappingCandidate{
					{
						NodeCode: "FOOD_DISH_TYPE_SNACKS",

						MappingType: MappingTypePrimary,

						ConfidenceBasis: MappingConfidenceSystem,

						ConfidenceScore: &score,
					},
				},
			},
		)

	if err == nil {
		t.Fatal(
			"confidence above 10000 accepted",
		)
	}
}

func TestSection13C3MetadataMustBeObject(
	t *testing.T,
) {
	_, _, err :=
		prepareReplaceFacetMappingsInput(
			ReplaceFacetMappingsInput{
				NamespaceCode: NamespaceFood,

				FacetCode: "DISH_TYPE",

				SourceType: "CATALOG_ITEM",

				SourceRef: "item-metadata",

				Mappings: []MappingCandidate{
					{
						NodeCode: "FOOD_DISH_TYPE_SNACKS",

						MappingType: MappingTypePrimary,

						Metadata: json.RawMessage(
							`["invalid"]`,
						),
					},
				},
			},
		)

	if err == nil {
		t.Fatal(
			"non-object metadata accepted",
		)
	}
}
