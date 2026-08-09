package models

import "testing"

func TestCanTransition_Matrix(t *testing.T) {
	allowed := map[[2]string]bool{
		{StatusDraft, StatusActive}:      true,
		{StatusDraft, StatusArchived}:    true,
		{StatusActive, StatusInactive}:   true,
		{StatusActive, StatusArchived}:   true,
		{StatusInactive, StatusActive}:   true,
		{StatusInactive, StatusArchived}: true,
	}
	statuses := []string{StatusDraft, StatusActive, StatusInactive, StatusArchived}
	for _, from := range statuses {
		for _, to := range statuses {
			want := allowed[[2]string{from, to}]
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestTerminal(t *testing.T) {
	if !Terminal(StatusArchived) {
		t.Error("ARCHIVED must be terminal")
	}
	for _, status := range []string{StatusDraft, StatusActive, StatusInactive} {
		if Terminal(status) {
			t.Errorf("%s must not be terminal", status)
		}
	}
}

func TestIsTaxiCategory(t *testing.T) {
	for _, category := range []string{CategoryTaxi, CategoryPassengerTransport} {
		if !IsTaxiCategory(category) {
			t.Errorf("%s must be a Taxi category", category)
		}
	}
	for _, category := range []string{CategoryPackageDelivery, CategoryHauling, CategoryOther} {
		if IsTaxiCategory(category) {
			t.Errorf("%s must not be a Taxi category", category)
		}
	}
}

func TestValidCategory(t *testing.T) {
	if !ValidCategory(CategoryHauling) {
		t.Error("HAULING must be a valid category")
	}
	if ValidCategory("NOT_A_REAL_CATEGORY") {
		t.Error("an unrecognized category must not validate")
	}
}

func TestNormalizeCode(t *testing.T) {
	cases := map[string]string{
		"Same Day":    "SAME_DAY",
		"same-day ":   "SAMEDAY",
		" TRUCK_10T ": "TRUCK_10T",
		"a  b   c":    "A_B_C",
		"":            "",
	}
	for input, want := range cases {
		if got := NormalizeCode(input); got != want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeCode_TwoSpellingsCollide(t *testing.T) {
	if NormalizeCode("Same Day") != NormalizeCode("same_day") {
		t.Error("two spellings of the same code must normalize identically")
	}
}

func TestValidateRequiredFieldsSchema_Valid(t *testing.T) {
	schema := JSONMap{
		"packageWeightKg": map[string]any{"type": "number", "required": true},
		"fragile":         map[string]any{"type": "boolean", "required": false},
		"size":            map[string]any{"type": "enum", "enum": []any{"SMALL", "MEDIUM", "LARGE"}},
	}
	if err := ValidateRequiredFieldsSchema(schema); err != nil {
		t.Errorf("expected valid schema, got error: %v", err)
	}
}

func TestValidateRequiredFieldsSchema_RejectsUnsupportedKeyword(t *testing.T) {
	schema := JSONMap{
		"weight": map[string]any{"type": "number", "$eval": "1+1"},
	}
	if err := ValidateRequiredFieldsSchema(schema); err == nil {
		t.Error("expected an error for an unsupported keyword")
	}
}

func TestValidateRequiredFieldsSchema_RejectsMissingType(t *testing.T) {
	schema := JSONMap{"weight": map[string]any{"required": true}}
	if err := ValidateRequiredFieldsSchema(schema); err == nil {
		t.Error("expected an error for a descriptor with no type")
	}
}

func TestValidateRequiredFieldsSchema_RejectsUnsupportedType(t *testing.T) {
	schema := JSONMap{"weight": map[string]any{"type": "object"}}
	if err := ValidateRequiredFieldsSchema(schema); err == nil {
		t.Error("expected an error for an unsupported field type")
	}
}

func TestValidateRequiredFieldsSchema_RejectsNonObjectDescriptor(t *testing.T) {
	schema := JSONMap{"weight": "not-an-object"}
	if err := ValidateRequiredFieldsSchema(schema); err == nil {
		t.Error("expected an error for a non-object descriptor")
	}
}

func TestValidateRequiredFieldsSchema_BoundsFieldCount(t *testing.T) {
	schema := make(JSONMap, maxSchemaFields+1)
	for i := 0; i < maxSchemaFields+1; i++ {
		schema[string(rune('a'+i%26))+string(rune(i))] = map[string]any{"type": "string"}
	}
	if err := ValidateRequiredFieldsSchema(schema); err == nil {
		t.Error("expected an error when the schema exceeds the field-count bound")
	}
}

func TestValidateRequiredFieldsSchema_BoundsEnumSize(t *testing.T) {
	values := make([]any, maxEnumValues+1)
	for i := range values {
		values[i] = "V"
	}
	schema := JSONMap{"size": map[string]any{"type": "enum", "enum": values}}
	if err := ValidateRequiredFieldsSchema(schema); err == nil {
		t.Error("expected an error when an enum exceeds the value-count bound")
	}
}

func TestSnapshot_OmitsPolicyReferences(t *testing.T) {
	item := CatalogItem{
		ID: "svc_1", Name: "Same Day", Code: "SAME_DAY", Category: CategoryPackageDelivery,
		PricingPolicyReference: "secret-pricing-policy",
	}
	snapshot := item.Snapshot()
	if _, present := snapshot["pricingPolicyReference"]; present {
		t.Error("snapshot must not carry the pricing policy reference")
	}
	if snapshot["code"] != "SAME_DAY" {
		t.Error("snapshot must carry the code")
	}
}
