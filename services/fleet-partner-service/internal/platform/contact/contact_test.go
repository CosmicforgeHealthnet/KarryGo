package contact

import "testing"

func TestNormalizeEmail(t *testing.T) {
	normalized, display, err := NormalizeEmail("  Owner@Example.COM ")
	if err != nil {
		t.Fatalf("valid email rejected: %v", err)
	}
	if normalized != "owner@example.com" {
		t.Fatalf("normalized = %q", normalized)
	}
	if display != "Owner@Example.COM" {
		t.Fatalf("display should preserve original casing, got %q", display)
	}
	for _, bad := range []string{"", "no-at-sign", "a@b", "a@@b.com", "spaces in@example.com"} {
		if _, _, err := NormalizeEmail(bad); err == nil {
			t.Fatalf("email %q should be rejected", bad)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"+2348012345678":    "+2348012345678",
		"08012345678":       "+2348012345678",
		"002348012345678":   "+2348012345678",
		"+234 801 234 5678": "+2348012345678",
		"+234-801-234-5678": "+2348012345678",
	}
	for input, want := range cases {
		got, _, err := NormalizePhone(input, "234")
		if err != nil {
			t.Fatalf("NormalizePhone(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizePhone(%q) = %q, want %q", input, got, want)
		}
	}
	for _, bad := range []string{"", "123", "abc", "+1234567890123456789"} {
		if _, _, err := NormalizePhone(bad, "234"); err == nil {
			t.Fatalf("phone %q should be rejected", bad)
		}
	}
	// A national number without a default country cannot be normalized.
	if _, _, err := NormalizePhone("08012345678", ""); err == nil {
		t.Fatal("national number without default country should be rejected")
	}
}

func TestNormalizePairRequiresBoth(t *testing.T) {
	if _, err := NormalizePair("owner@example.com", "", "234"); err == nil {
		t.Fatal("phone is mandatory")
	}
	if _, err := NormalizePair("", "+2348012345678", "234"); err == nil {
		t.Fatal("email is mandatory")
	}
	pair, err := NormalizePair("Owner@Example.com", "08012345678", "234")
	if err != nil {
		t.Fatalf("valid pair rejected: %v", err)
	}
	if pair.EmailNormalized != "owner@example.com" || pair.PhoneE164 != "+2348012345678" {
		t.Fatalf("pair = %+v", pair)
	}
}

func TestHashIsKeyedAndStable(t *testing.T) {
	first := Hash([]byte("secret-a"), "owner@example.com")
	again := Hash([]byte("secret-a"), "owner@example.com")
	other := Hash([]byte("secret-b"), "owner@example.com")
	if first != again {
		t.Fatal("hash must be stable for the same key and value")
	}
	if first == other {
		t.Fatal("hash must depend on the key")
	}
	if first == "owner@example.com" {
		t.Fatal("hash must not be the plain value")
	}
}

func TestMaskEmail(t *testing.T) {
	if got := MaskEmail("owner@example.com"); got != "o***@example.com" {
		t.Fatalf("MaskEmail = %q", got)
	}
	if got := MaskEmail("a@example.com"); got != "a***@example.com" {
		t.Fatalf("MaskEmail single char = %q", got)
	}
}
