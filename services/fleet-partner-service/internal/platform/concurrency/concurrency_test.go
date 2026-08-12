package concurrency

import (
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
)

func TestParseIfMatch(t *testing.T) {
	for header, want := range map[string]int64{`"3"`: 3, "3": 3, ` "12" `: 12} {
		got, err := ParseIfMatch(header)
		if err != nil || got != want {
			t.Fatalf("ParseIfMatch(%q) = %d, %v", header, got, err)
		}
	}
	for _, bad := range []string{"", `"abc"`, "0", "-1", `"1.5"`} {
		if _, err := ParseIfMatch(bad); err == nil {
			t.Fatalf("ParseIfMatch(%q) should fail", bad)
		}
	}
}

func TestExpectedVersionSources(t *testing.T) {
	if _, err := ExpectedVersion("", nil); err == nil {
		t.Fatal("missing version should fail")
	}
	v := int64(4)
	got, err := ExpectedVersion("", &v)
	if err != nil || got != 4 {
		t.Fatalf("body version = %d, %v", got, err)
	}
	got, err = ExpectedVersion(`"7"`, &v)
	if err != nil || got != 7 {
		t.Fatalf("If-Match should win, got %d, %v", got, err)
	}
}

func TestAsConflict(t *testing.T) {
	err := AsConflict(ErrVersionMismatch)
	appErr := apperrors.From(err)
	if appErr.Code != apperrors.CodeConflict {
		t.Fatalf("code = %s", appErr.Code)
	}
}
