package taxonomy

import (
	"encoding/json"
	"testing"
)

func TestSection13C4GovernanceCaseTypes(t *testing.T) {
	values := []GovernanceCaseType{
		GovernanceCaseMappingReview,
		GovernanceCaseClassificationDispute,
		GovernanceCaseDuplicateConcept,
		GovernanceCaseMisclassification,
		GovernanceCaseVocabularyReview,
		GovernanceCasePolicyReview,
	}

	for _, value := range values {
		if !validGovernanceCaseType(value) {
			t.Fatalf(
				"valid governance case rejected: %s",
				value,
			)
		}
	}
}

func TestSection13C4GovernanceTransitions(t *testing.T) {
	values := []GovernanceTransition{
		GovernanceTransitionStartReview,
		GovernanceTransitionAssign,
		GovernanceTransitionResolve,
		GovernanceTransitionReject,
		GovernanceTransitionCancel,
	}

	for _, value := range values {
		if !validGovernanceTransition(value) {
			t.Fatalf(
				"valid governance transition rejected: %s",
				value,
			)
		}
	}
}

func TestSection13C4DeterministicCaseID(t *testing.T) {
	first := deterministicGovernanceCaseID(
		NamespaceFood,
		"version-1",
		"catalog_item",
		"item-1",
		GovernanceCaseMappingReview,
	)

	second := deterministicGovernanceCaseID(
		NamespaceFood,
		"version-1",
		"CATALOG_ITEM",
		"item-1",
		GovernanceCaseMappingReview,
	)

	if first != second {
		t.Fatal(
			"subject_type normalization changed deterministic case ID",
		)
	}

	other := deterministicGovernanceCaseID(
		NamespaceShop,
		"version-1",
		"CATALOG_ITEM",
		"item-1",
		GovernanceCaseMappingReview,
	)

	if first == other {
		t.Fatal(
			"FOOD/SHOP governance cases collided",
		)
	}
}

func TestSection13C4GovernanceHashStable(
	t *testing.T,
) {
	meta1 := json.RawMessage(
		`{"b":2,"a":1}`,
	)

	meta2 := json.RawMessage(
		`{"a":1,"b":2}`,
	)

	normalized1, err :=
		normalizeGovernanceMetadata(meta1)
	if err != nil {
		t.Fatal(err)
	}

	normalized2, err :=
		normalizeGovernanceMetadata(meta2)
	if err != nil {
		t.Fatal(err)
	}

	first := governanceCommandEnvelope{
		Operation: "OPEN_CASE",

		NamespaceCode: NamespaceFood,

		SubjectType: "CATALOG_ITEM",

		SubjectRef: "item-1",

		CaseType: GovernanceCaseMappingReview,

		Priority: GovernancePriorityNormal,

		Metadata: normalized1,
	}

	second := first
	second.Metadata = normalized2

	firstHash, err :=
		governanceRequestHash(first)
	if err != nil {
		t.Fatal(err)
	}

	secondHash, err :=
		governanceRequestHash(second)
	if err != nil {
		t.Fatal(err)
	}

	if firstHash != secondHash {
		t.Fatal(
			"equivalent metadata produced different governance hash",
		)
	}
}

func TestSection13C4MetadataMustBeObject(
	t *testing.T,
) {
	_, err :=
		normalizeGovernanceMetadata(
			json.RawMessage(
				`["invalid"]`,
			),
		)

	if err == nil {
		t.Fatal(
			"non-object governance metadata accepted",
		)
	}
}

func TestSection13C4GovernancePriorities(
	t *testing.T,
) {
	values := []GovernancePriority{
		GovernancePriorityLow,
		GovernancePriorityNormal,
		GovernancePriorityHigh,
		GovernancePriorityCritical,
	}

	for _, value := range values {
		if !validGovernancePriority(value) {
			t.Fatalf(
				"priority rejected: %s",
				value,
			)
		}
	}
}
