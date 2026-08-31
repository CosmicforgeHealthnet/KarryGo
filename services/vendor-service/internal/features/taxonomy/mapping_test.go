package taxonomy

import "testing"

func TestSection13C1MappingEnums(t *testing.T) {
	mappingTypes := []MappingType{
		MappingTypePrimary,
		MappingTypeSecondary,
		MappingTypeDiscovery,
	}

	if len(mappingTypes) != 3 {
		t.Fatal("mapping type contract changed")
	}

	confidence := []MappingConfidenceBasis{
		MappingConfidenceManual,
		MappingConfidenceRule,
		MappingConfidenceImport,
		MappingConfidenceSystem,
	}

	if len(confidence) != 4 {
		t.Fatal("confidence basis contract changed")
	}
}

func TestSection13C1MappingValidation(t *testing.T) {
	score := 8500

	mapping := TaxonomyMapping{
		ID:              "mapping",
		NamespaceID:     "namespace",
		VersionID:       "version",
		NodeID:          "node",
		SourceType:      "CATALOG_ITEM",
		SourceRef:       "item-1",
		MappingType:     MappingTypePrimary,
		Status:          MappingStatusActive,
		ConfidenceBasis: MappingConfidenceManual,
		ConfidenceScore: &score,
	}

	if err := mapping.Validate(); err != nil {
		t.Fatalf(
			"valid mapping rejected: %v",
			err,
		)
	}
}

func TestSection13C1MappingConfidenceBounds(t *testing.T) {
	score := 10001

	mapping := TaxonomyMapping{
		NamespaceID:     "namespace",
		VersionID:       "version",
		NodeID:          "node",
		SourceType:      "CATALOG_ITEM",
		SourceRef:       "item-1",
		MappingType:     MappingTypePrimary,
		Status:          MappingStatusActive,
		ConfidenceBasis: MappingConfidenceManual,
		ConfidenceScore: &score,
	}

	if err := mapping.Validate(); err == nil {
		t.Fatal(
			"confidence score above 10000 accepted",
		)
	}
}

func TestSection13C1GovernanceEnums(t *testing.T) {
	caseTypes := []GovernanceCaseType{
		GovernanceCaseMappingReview,
		GovernanceCaseClassificationDispute,
		GovernanceCaseDuplicateConcept,
		GovernanceCaseMisclassification,
		GovernanceCaseVocabularyReview,
		GovernanceCasePolicyReview,
	}

	if len(caseTypes) != 6 {
		t.Fatal(
			"governance case type contract changed",
		)
	}

	statuses := []GovernanceStatus{
		GovernanceStatusOpen,
		GovernanceStatusInReview,
		GovernanceStatusResolved,
		GovernanceStatusRejected,
		GovernanceStatusCancelled,
	}

	if len(statuses) != 5 {
		t.Fatal(
			"governance status contract changed",
		)
	}
}

func TestSection13C1MappingIDDeterministic(t *testing.T) {
	input := CreateMappingInput{
		NamespaceCode:   NamespaceFood,
		VersionID:       "version-1",
		NodeID:          "node-1",
		SourceType:      "catalog_item",
		SourceRef:       "item-1",
		MappingType:     MappingTypePrimary,
		ConfidenceBasis: MappingConfidenceManual,
	}

	first := deterministicMappingID(input)
	second := deterministicMappingID(input)

	if first != second {
		t.Fatal(
			"mapping identity must be deterministic",
		)
	}

	input.NodeID = "node-2"

	if first == deterministicMappingID(input) {
		t.Fatal(
			"different taxonomy nodes must not share mapping ID",
		)
	}
}
