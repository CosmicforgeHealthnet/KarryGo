package models

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := Document{SubjectType: "ORDER", SubjectReference: "ord_1", Title: "Order ORD-1", RequiredPermission: "orders.read"}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected a valid document, got %v", err)
	}
	cases := []Document{
		{SubjectReference: "ord_1", Title: "T", RequiredPermission: "orders.read"},
		{SubjectType: "ORDER", Title: "T", RequiredPermission: "orders.read"},
		{SubjectType: "ORDER", SubjectReference: "ord_1", RequiredPermission: "orders.read"},
		{SubjectType: "ORDER", SubjectReference: "ord_1", Title: "T"},
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: expected an error for %+v", i, c)
		}
	}
}

func TestBounded(t *testing.T) {
	ok := Document{SubjectType: "ORDER", Title: "short", SearchableText: "short text"}
	if !ok.Bounded() {
		t.Error("expected a small document to be bounded")
	}
	tooLongTitle := Document{SubjectType: "ORDER", Title: strings.Repeat("a", maxTitleLen+1)}
	if tooLongTitle.Bounded() {
		t.Error("expected an oversized title to be rejected")
	}
	tooLongText := Document{SubjectType: "ORDER", Title: "ok", SearchableText: strings.Repeat("a", maxSearchableLen+1)}
	if tooLongText.Bounded() {
		t.Error("expected oversized searchable text to be rejected")
	}
	tooLongType := Document{SubjectType: strings.Repeat("a", maxSubjectTypeLen+1), Title: "ok"}
	if tooLongType.Bounded() {
		t.Error("expected an oversized subject type to be rejected")
	}
}

func TestClone(t *testing.T) {
	original := JSONMap{"a": 1}
	cloned := original.Clone()
	cloned["b"] = 2
	if _, present := original["b"]; present {
		t.Error("mutating the clone must not mutate the original")
	}
	var nilMap JSONMap
	if nilMap.Clone() != nil {
		t.Error("cloning a nil map must return nil")
	}
}
