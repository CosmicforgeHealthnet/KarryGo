package handover

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBuildChallengeID(t *testing.T) {
	a := BuildChallengeID("hauling", StepPickup, "booking-1")
	b := BuildChallengeID("hauling", StepDelivery, "booking-1")
	c := BuildChallengeID("dispatch", StepPickup, "booking-1")

	if a == b || a == c || b == c {
		t.Fatalf("expected distinct challenge IDs, got %q %q %q", a, b, c)
	}

	if got, want := a, "handover:hauling:pickup:booking-1"; got != want {
		t.Fatalf("BuildChallengeID() = %q, want %q", got, want)
	}
}

func TestSecureCodeRecordUsesIndependentContextBoundCredentials(t *testing.T) {
	secret := []byte("secure-handover-test-secret")
	now := time.Now().UTC().Truncate(time.Microsecond)
	rec, err := NewSecureCodeRecord(secret, "hauling", StepPickup, "challenge-1", 1, "booking-1", "provider-1", "customer-1", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Code) != 6 || len(rec.QRToken) < 40 || strings.Contains(rec.QRToken, rec.Code) || rec.QRToken == rec.Code {
		t.Fatalf("credentials are not independent opaque values: otp_len=%d qr_len=%d", len(rec.Code), len(rec.QRToken))
	}
	if rec.ExpiresAt.Sub(now) != DefaultTTL {
		t.Fatalf("TTL = %v, want %v", rec.ExpiresAt.Sub(now), DefaultTTL)
	}
	if !VerifySecureCredential(secret, "OTP", rec, rec.Code, rec.Hash) || !VerifySecureCredential(secret, "QR", rec, rec.QRToken, rec.QRHash) {
		t.Fatal("issued credentials did not verify")
	}
	restored := rec
	restored.Code, restored.QRToken = "", ""
	if recovered, ok := RecoverSecureCredentials(secret, restored); !ok || recovered.Code != rec.Code || recovered.QRToken != rec.QRToken {
		t.Fatal("credential reconstruction failed")
	}
	if VerifySecureCredential(secret, "QR", rec, rec.Code, rec.QRHash) || VerifySecureCredential(secret, "OTP", rec, rec.QRToken, rec.Hash) {
		t.Fatal("OTP and QR purposes must not be interchangeable")
	}

	contexts := []CodeRecord{rec, rec, rec, rec}
	contexts[0].BookingID = "booking-2"
	contexts[1].ProviderID = "provider-2"
	contexts[2].Stage = StepDelivery
	contexts[3].Version = 2
	for i, changed := range contexts {
		if VerifySecureCredential(secret, "OTP", changed, rec.Code, rec.Hash) || VerifySecureCredential(secret, "QR", changed, rec.QRToken, rec.QRHash) {
			t.Fatalf("credential verified across changed authoritative context %d", i)
		}
	}
}

func TestSecureCodeRecordGeneratesUnpredictableQRTokens(t *testing.T) {
	secret := []byte("secure-handover-test-secret")
	seen := make(map[string]struct{}, 128)
	for i := 0; i < 128; i++ {
		rec, err := NewSecureCodeRecord(secret, "hauling", StepDelivery, fmt.Sprintf("challenge-%d", i+1), 1, "booking-1", "provider-1", "customer-1", 0, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if _, duplicate := seen[rec.QRToken]; duplicate {
			t.Fatal("duplicate opaque QR token generated")
		}
		seen[rec.QRToken] = struct{}{}
	}
}

func TestNewCodeRecordAndVerifyRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	challengeID := BuildChallengeID("hauling", StepPickup, "booking-1")
	now := time.Now()

	rec, err := NewCodeRecord(secret, challengeID, now)
	if err != nil {
		t.Fatalf("NewCodeRecord() error = %v", err)
	}

	if len(rec.Code) == 0 {
		t.Fatalf("expected a non-empty generated code")
	}

	if !VerifyCode(secret, challengeID, rec.Code, rec.Hash) {
		t.Fatalf("expected correct code to verify")
	}

	if VerifyCode(secret, challengeID, "000000", rec.Hash) && rec.Code != "000000" {
		t.Fatalf("expected wrong code to fail verification")
	}

	// Verifying under a different challengeID (different step/entity) must fail
	// even with the same code, proving namespacing prevents cross-step replay.
	otherChallengeID := BuildChallengeID("hauling", StepDelivery, "booking-1")
	if VerifyCode(secret, otherChallengeID, rec.Code, rec.Hash) {
		t.Fatalf("expected verification to fail across different challenge IDs")
	}
}

func TestCodeRecordIsExpired(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name string
		rec  CodeRecord
		want bool
	}{
		{"zero expiry treated as expired", CodeRecord{}, true},
		{"future expiry not expired", CodeRecord{ExpiresAt: now.Add(time.Minute)}, false},
		{"past expiry is expired", CodeRecord{ExpiresAt: now.Add(-time.Minute)}, true},
		{"exact boundary not expired", CodeRecord{ExpiresAt: now}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rec.IsExpired(now); got != tc.want {
				t.Fatalf("IsExpired() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCodeRecordAttemptsExceeded(t *testing.T) {
	cases := []struct {
		attempts int
		want     bool
	}{
		{0, false},
		{MaxVerifyAttempts - 1, false},
		{MaxVerifyAttempts, true},
		{MaxVerifyAttempts + 1, true},
	}

	for _, tc := range cases {
		rec := CodeRecord{Attempts: tc.attempts}
		if got := rec.AttemptsExceeded(); got != tc.want {
			t.Fatalf("attempts=%d: AttemptsExceeded() = %v, want %v", tc.attempts, got, tc.want)
		}
	}
}

func TestCodeRecordCanResend(t *testing.T) {
	now := time.Now()

	t.Run("never sent can resend immediately", func(t *testing.T) {
		rec := CodeRecord{}
		ok, wait := rec.CanResend(now)
		if !ok || wait != 0 {
			t.Fatalf("CanResend() = (%v, %v), want (true, 0)", ok, wait)
		}
	})

	t.Run("within cooldown is blocked", func(t *testing.T) {
		last := now.Add(-10 * time.Second)
		rec := CodeRecord{LastSentAt: &last}
		ok, wait := rec.CanResend(now)
		if ok {
			t.Fatalf("expected resend to be blocked within cooldown")
		}
		if wait <= 0 || wait > ResendCooldown {
			t.Fatalf("expected a positive wait under the cooldown window, got %v", wait)
		}
	})

	t.Run("exact cooldown boundary allows resend", func(t *testing.T) {
		last := now.Add(-ResendCooldown)
		rec := CodeRecord{LastSentAt: &last}
		ok, wait := rec.CanResend(now)
		if !ok || wait != 0 {
			t.Fatalf("CanResend() = (%v, %v), want (true, 0)", ok, wait)
		}
	})

	t.Run("past cooldown allows resend", func(t *testing.T) {
		last := now.Add(-ResendCooldown - time.Second)
		rec := CodeRecord{LastSentAt: &last}
		ok, wait := rec.CanResend(now)
		if !ok || wait != 0 {
			t.Fatalf("CanResend() = (%v, %v), want (true, 0)", ok, wait)
		}
	})

	t.Run("max resends reached blocks regardless of cooldown", func(t *testing.T) {
		rec := CodeRecord{ResendCount: MaxResends}
		ok, _ := rec.CanResend(now)
		if ok {
			t.Fatalf("expected resend to be blocked once MaxResends is reached")
		}
	})
}
