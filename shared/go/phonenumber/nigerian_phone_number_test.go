package phonenumber

import "testing"

func TestNormalizeNigerianPhoneNumber(t *testing.T) {
	tests := map[string]string{
		"08012345678":       "+2348012345678",
		"2348012345678":     "+2348012345678",
		"+234 801 234 5678": "+2348012345678",
	}

	for input, expected := range tests {
		actual, err := NormalizeNigerianPhoneNumber(input)
		if err != nil {
			t.Fatalf("NormalizeNigerianPhoneNumber(%q) error = %v", input, err)
		}
		if actual != expected {
			t.Fatalf("NormalizeNigerianPhoneNumber(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestNormalizeNigerianPhoneNumberRejectsInvalidPhone(t *testing.T) {
	if _, err := NormalizeNigerianPhoneNumber("123"); err == nil {
		t.Fatal("expected invalid phone error")
	}
}
