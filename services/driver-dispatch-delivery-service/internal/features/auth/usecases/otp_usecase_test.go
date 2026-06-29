package authusecases

import (
	"testing"
	"time"
	"unicode"

	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
)

func TestGenerateCodeCreatesSixDigits(t *testing.T) {
	otp := NewOTPUsecase(OTPOptions{
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 3,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})

	code, err := otp.GenerateCode()
	if err != nil {
		t.Fatalf("GenerateCode() error = %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("len(code) = %d, want 6", len(code))
	}
	for _, char := range code {
		if !unicode.IsDigit(char) {
			t.Fatalf("code contains non-digit rune %q", char)
		}
	}
}

func TestHashAndCompareCode(t *testing.T) {
	otpUsecase := NewOTPUsecase(OTPOptions{
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 3,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})

	hash := otpUsecase.HashCode("otp-id", "+15551234567", "123456")
	if hash == "123456" {
		t.Fatal("HashCode() returned plain OTP")
	}

	otp := fakeOTP("otp-id", "+15551234567", hash)
	if !otpUsecase.CompareCode(otp, "123456") {
		t.Fatal("CompareCode() did not match original OTP")
	}
	if otpUsecase.CompareCode(otp, "000000") {
		t.Fatal("CompareCode() matched incorrect OTP")
	}
}

func TestNormalizePhoneNumber(t *testing.T) {
	phone, err := NormalizePhoneNumber("+1 (555) 123-4567")
	if err != nil {
		t.Fatalf("NormalizePhoneNumber() error = %v", err)
	}
	if phone != "+15551234567" {
		t.Fatalf("phone = %q, want +15551234567", phone)
	}
}

func TestOTPRateLimitPrefix(t *testing.T) {
	if OTPRateLimitPrefix != "dispatch_rider_auth:otp_rate:" {
		t.Fatalf("OTPRateLimitPrefix = %q", OTPRateLimitPrefix)
	}
}

func fakeOTP(id string, phoneNumber string, hash string) authmodels.OTP {
	return authmodels.OTP{ID: id, PhoneNumber: phoneNumber, OTPCodeHash: hash}
}

// ── E.164 validation tests ────────────────────────────────────────────────────

func TestValidatePhoneNumber_ValidE164(t *testing.T) {
	cases := []string{
		"+2348012345678",
		"+15551234567",
		"+447911123456",
	}
	for _, phone := range cases {
		if err := ValidatePhoneNumber(phone); err != nil {
			t.Errorf("ValidatePhoneNumber(%q) error = %v, want nil", phone, err)
		}
	}
}

func TestValidatePhoneNumber_MissingPlus(t *testing.T) {
	// E.164 requires leading '+'; bare local or national formats must fail.
	cases := []string{"08012345678", "2348012345678", "15551234567"}
	for _, phone := range cases {
		if err := ValidatePhoneNumber(phone); err == nil {
			t.Errorf("ValidatePhoneNumber(%q) = nil, want validation error", phone)
		}
	}
}

func TestValidatePhoneNumber_Empty(t *testing.T) {
	if err := ValidatePhoneNumber(""); err == nil {
		t.Error("ValidatePhoneNumber('') = nil, want error")
	}
}

func TestNormalizePhoneNumber_RejectsNoPlus(t *testing.T) {
	_, err := NormalizePhoneNumber("08012345678")
	if err == nil {
		t.Error("NormalizePhoneNumber('08012345678') = nil, want E.164 error")
	}
}

func TestNormalizePhoneNumber_StripsFormatting(t *testing.T) {
	phone, err := NormalizePhoneNumber("+234 801 234 5678")
	if err != nil {
		t.Fatalf("NormalizePhoneNumber error = %v", err)
	}
	if phone != "+2348012345678" {
		t.Errorf("phone = %q, want +2348012345678", phone)
	}
}
