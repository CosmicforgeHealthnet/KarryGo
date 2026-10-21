package auth

import (
	"strings"
	"testing"
)

func identity(s string) (string, error) { return s, nil }

func TestParseTestLogin(t *testing.T) {
	off, err := ParseTestLogin("staging", "  ", "123456", identity)
	if err != nil || off != nil {
		t.Fatalf("no numbers must mean off, got %v %v", off, err)
	}
	if _, err := ParseTestLogin("production", "+2348000001001", "123456", identity); err == nil {
		t.Fatal("must refuse to run in production")
	}
	if _, err := ParseTestLogin("staging", "+2348000001001", "12345", identity); err == nil {
		t.Fatal("must refuse a code that is not six digits")
	}
	if _, err := ParseTestLogin("staging", "+2348031234567", "123456", identity); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("must refuse a personal mobile number, got %v", err)
	}

	tl, err := ParseTestLogin("staging", "+2348000001001, +2348000002001", "123456", identity)
	if err != nil || tl.Count() != 2 {
		t.Fatalf("want 2 numbers, got %v %v", tl, err)
	}
	if code, ok := tl.CodeFor("phone", "+2348000001001"); !ok || code != "123456" {
		t.Fatal("a listed number gets the fixed code")
	}
	if _, ok := tl.CodeFor("phone", "+2348000009999"); ok {
		t.Fatal("an unlisted number must not")
	}
	if _, ok := tl.CodeFor("email", "+2348000001001"); ok {
		t.Fatal("only phone sign-in")
	}
	var none *TestLogin
	if _, ok := none.CodeFor("phone", "+2348000001001"); ok {
		t.Fatal("nil switch is off")
	}
}
