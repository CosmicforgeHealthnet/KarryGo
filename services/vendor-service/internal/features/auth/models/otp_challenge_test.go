package vendorauthmodels

import (
	"testing"
	"time"

	sharedauth "cosmicforge/logistics/shared/go/auth"
)

var testSecret = []byte("test-otp-secret")

func freshChallenge(now time.Time) OTPChallenge {
	c := OTPChallenge{
		ID:              "chal_1",
		Purpose:         PurposeLogin,
		IdentifierType:  IdentifierPhone,
		IdentifierValue: "+2348011111111",
		Status:          ChallengeStatusActive,
		Generation:      1,
		ExpiresAt:       now.Add(10 * time.Minute),
		CreatedAt:       now,
	}
	c.OTPHash = c.HashFor(testSecret, "123456", 1)
	return c
}

func TestOTPChallenge_VerifyCorrectCodeSucceeds(t *testing.T) {
	now := time.Now()
	c := freshChallenge(now)
	if err := c.Verify(testSecret, c.ID, "123456", 5, now); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestOTPChallenge_WrongCodeFails(t *testing.T) {
	now := time.Now()
	c := freshChallenge(now)
	if err := c.Verify(testSecret, c.ID, "000000", 5, now); err == nil {
		t.Fatal("expected wrong code to fail")
	}
}

func TestOTPChallenge_ExpiredFails(t *testing.T) {
	now := time.Now()
	c := freshChallenge(now)
	c.ExpiresAt = now.Add(-time.Second)
	if err := c.Verify(testSecret, c.ID, "123456", 5, now); err == nil {
		t.Fatal("expected expired challenge to fail")
	}
}

func TestOTPChallenge_ConsumedFails(t *testing.T) {
	now := time.Now()
	c := freshChallenge(now)
	c.Status = ChallengeStatusConsumed
	if err := c.Verify(testSecret, c.ID, "123456", 5, now); err == nil {
		t.Fatal("expected consumed challenge to fail")
	}
}

func TestOTPChallenge_LockedFails(t *testing.T) {
	now := time.Now()
	c := freshChallenge(now)
	c.Status = ChallengeStatusLocked
	if err := c.Verify(testSecret, c.ID, "123456", 5, now); err == nil {
		t.Fatal("expected locked challenge to fail")
	}
}

func TestOTPChallenge_AttemptsExhaustedFails(t *testing.T) {
	now := time.Now()
	c := freshChallenge(now)
	c.Attempts = 5
	if err := c.Verify(testSecret, c.ID, "123456", 5, now); err == nil {
		t.Fatal("expected exhausted attempts to fail even with the correct code")
	}
}

func TestOTPChallenge_WrongChallengeIDFails(t *testing.T) {
	now := time.Now()
	c := freshChallenge(now)
	if err := c.Verify(testSecret, "some-other-challenge-id", "123456", 5, now); err == nil {
		t.Fatal("expected mismatched challenge id to fail")
	}
}

func TestOTPChallenge_ResendInvalidatesPreviousGeneration(t *testing.T) {
	now := time.Now()
	c := freshChallenge(now) // generation 1, code "123456"

	// Simulate a resend: bump generation, issue a new code, overwrite hash.
	c.Generation = 2
	c.OTPHash = c.HashFor(testSecret, "654321", 2)

	if err := c.Verify(testSecret, c.ID, "123456", 5, now); err == nil {
		t.Fatal("expected the old (generation 1) code to be rejected after resend")
	}
	if err := c.Verify(testSecret, c.ID, "654321", 5, now); err != nil {
		t.Fatalf("expected the new (generation 2) code to succeed, got %v", err)
	}
}

func TestOTPChallenge_RawOTPNeverAppearsInHash(t *testing.T) {
	now := time.Now()
	c := freshChallenge(now)
	if c.OTPHash == "123456" {
		t.Fatal("OTPHash must never equal the raw code")
	}
	// The hash must be a genuine HMAC digest, not a trivial transform: verify
	// it round-trips only through shared/go/auth.VerifyOTP, not by string
	// comparison against the raw code.
	if sharedauth.VerifyOTP(testSecret, c.verificationScope(), c.IdentifierKey(), "123456", c.OTPHash) == false {
		t.Fatal("expected the stored hash to verify against the known raw code via the shared HMAC helper")
	}
}

func TestSession_IsActive(t *testing.T) {
	now := time.Now()
	active := Session{ExpiresAt: now.Add(time.Hour)}
	if !active.IsActive(now) {
		t.Fatal("expected an unexpired, unrevoked session to be active")
	}

	expired := Session{ExpiresAt: now.Add(-time.Hour)}
	if expired.IsActive(now) {
		t.Fatal("expected an expired session to be inactive")
	}

	revokedAt := now.Add(-time.Minute)
	revoked := Session{ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt}
	if revoked.IsActive(now) {
		t.Fatal("expected a revoked session to be inactive even if unexpired")
	}
}

func TestStepUpGrant_UsableRequiresExactMatchOnEveryDimension(t *testing.T) {
	now := time.Now()
	grant := StepUpGrant{
		SessionID: "sess_1", StaffID: "staff_1", VendorID: "vendor_1", Purpose: "finance.bank_change",
		Status: StepUpStatusActive, ExpiresAt: now.Add(5 * time.Minute),
	}

	if !grant.Usable("sess_1", "staff_1", "vendor_1", "finance.bank_change", now) {
		t.Fatal("expected an exact match to be usable")
	}
	if grant.Usable("sess_2", "staff_1", "vendor_1", "finance.bank_change", now) {
		t.Fatal("expected a wrong session to be refused")
	}
	if grant.Usable("sess_1", "staff_2", "vendor_1", "finance.bank_change", now) {
		t.Fatal("expected a wrong staff to be refused")
	}
	if grant.Usable("sess_1", "staff_1", "vendor_2", "finance.bank_change", now) {
		t.Fatal("expected a wrong vendor to be refused")
	}
	if grant.Usable("sess_1", "staff_1", "vendor_1", "owner.transfer", now) {
		t.Fatal("expected a wrong purpose to be refused")
	}
	if grant.Usable("sess_1", "staff_1", "vendor_1", "finance.bank_change", now.Add(10*time.Minute)) {
		t.Fatal("expected an expired grant to be refused")
	}

	consumed := grant
	consumed.Status = StepUpStatusConsumed
	if consumed.Usable("sess_1", "staff_1", "vendor_1", "finance.bank_change", now) {
		t.Fatal("expected a consumed grant to be refused")
	}
}
