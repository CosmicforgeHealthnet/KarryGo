package pagination

import (
	"net/http"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
)

func TestParseDefaultsAndBounds(t *testing.T) {
	page, err := Parse("", "")
	if err != nil || page.Limit != DefaultLimit || page.Offset != 0 {
		t.Fatalf("defaults = %+v %v", page, err)
	}
	page, err = Parse("0", "0")
	if err != nil || page.Limit != DefaultLimit || page.Offset != 0 {
		t.Fatalf("zero = %+v %v", page, err)
	}
	page, err = Parse(" 200 ", "10000")
	if err != nil || page.Limit != MaxLimit || page.Offset != MaxOffset {
		t.Fatalf("upper bounds = %+v %v", page, err)
	}
}

func TestParseRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct{ limit, offset, field string }{
		{"-1", "", "limit"},
		{"201", "", "limit"},
		{"100000", "", "limit"},
		{"ten", "", "limit"},
		{"", "-1", "offset"},
		{"", "10001", "offset"},
		{"", "99999999999999999999", "offset"},
		{"", "1.5", "offset"},
	} {
		_, err := Parse(tc.limit, tc.offset)
		if err == nil {
			t.Fatalf("Parse(%q,%q) accepted", tc.limit, tc.offset)
		}
		appErr := apperrors.From(err)
		if appErr.Status != http.StatusUnprocessableEntity || len(appErr.Fields) != 1 || appErr.Fields[0].Field != tc.field {
			t.Fatalf("Parse(%q,%q) = %d %+v", tc.limit, tc.offset, appErr.Status, appErr.Fields)
		}
	}
}

func TestClamp(t *testing.T) {
	if ClampLimit(-5) != DefaultLimit || ClampLimit(0) != DefaultLimit || ClampLimit(7) != 7 || ClampLimit(5000) != MaxLimit {
		t.Fatal("ClampLimit")
	}
	if ClampOffset(-1) != 0 || ClampOffset(3) != 3 || ClampOffset(1<<40) != MaxOffset {
		t.Fatal("ClampOffset")
	}
}
