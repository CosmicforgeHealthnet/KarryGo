package auth

import "testing"

func TestGenerateNumericOTP(t *testing.T) {
	code, err := GenerateNumericOTP(6)
	if err != nil {
		t.Fatalf("GenerateNumericOTP() error = %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("expected 6 digits, got %q", code)
	}
	for _, char := range code {
		if char < '0' || char > '9' {
			t.Fatalf("expected numeric otp, got %q", code)
		}
	}
}

func TestHashAndVerifyOTP(t *testing.T) {
	secret := []byte("secret")
	hash := HashOTP(secret, "challenge", "+2348012345678", "123456")
	if hash == "" {
		t.Fatal("expected otp hash")
	}
	if !VerifyOTP(secret, "challenge", "+2348012345678", "123456", hash) {
		t.Fatal("expected otp to verify")
	}
	if VerifyOTP(secret, "challenge", "+2348012345678", "000000", hash) {
		t.Fatal("expected wrong otp to fail")
	}
}
