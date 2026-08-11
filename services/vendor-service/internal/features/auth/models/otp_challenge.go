// Package vendorauthmodels implements Vendor Service's AUTHENTICATION domain
// rules: OTP challenge state machine, refresh sessions, step-up grants.
//
// This is a completely separate security domain from the platform's
// logistics pickup/delivery custody handover (see
// docs/architecture/07_EXISTING_OTP_QR_INTEGRATION.md). Nothing here shares a
// table, a challenge ID, a Redis namespace, or a resend policy with that
// system, and nothing here can confirm a delivery — an OTPChallenge's Purpose
// is always "login" or "step_up", never a handover purpose.
package vendorauthmodels

import (
	"strconv"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

const (
	PurposeLogin  = "login"
	PurposeStepUp = "step_up"

	IdentifierPhone = "phone"
	IdentifierEmail = "email"

	ChallengeStatusActive   = "active"
	ChallengeStatusConsumed = "consumed"
	ChallengeStatusExpired  = "expired"
	ChallengeStatusLocked   = "locked"
)

// OTPChallenge is the AUTHENTICATION OTP challenge — never persists the raw
// code, only OTPHash (an HMAC over challenge id + identifier key + code, via
// shared/go/auth.HashOTP).
type OTPChallenge struct {
	ID              string
	Purpose         string
	IdentifierType  string
	IdentifierValue string
	OTPHash         string
	Status          string
	Attempts        int
	ResendCount     int
	Generation      int

	// Step-up-only binding (nil for purpose=login).
	SessionID     *string
	StaffID       *string
	VendorID      *string
	StepUpPurpose *string

	ExpiresAt  time.Time
	ConsumedAt *time.Time
	LockedAt   *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// IdentifierKey is the rate-limit/lookup key: "phone:+234..." or
// "email:someone@example.com".
func (c OTPChallenge) IdentifierKey() string {
	return c.IdentifierType + ":" + c.IdentifierValue
}

// Verify checks a submitted code against the challenge's current generation
// without mutating anything — the caller applies the resulting state change
// (consume or record-failed-attempt) atomically in the repository layer.
// Every failure path returns the same generic message deliberately: an
// unauthorized caller must not be able to distinguish "wrong code" from
// "expired" from "locked" from "wrong challenge id" through response content.
func (c OTPChallenge) Verify(secret []byte, submittedChallengeID string, otp string, maxAttempts int, now time.Time) error {
	generic := apperrors.Unauthorized("Invalid or expired verification code.", nil)

	if c.Status != ChallengeStatusActive {
		return generic
	}
	if submittedChallengeID != "" && submittedChallengeID != c.ID {
		return generic
	}
	if !c.ExpiresAt.After(now) {
		return generic
	}
	if c.Attempts >= maxAttempts {
		return generic
	}
	if !sharedauth.VerifyOTP(secret, c.verificationScope(), c.IdentifierKey(), otp, c.OTPHash) {
		return generic
	}
	return nil
}

// verificationScope binds the hash to both the challenge id and its current
// generation, so a resend (which bumps Generation) invalidates the previous
// code's hash even though the row id is unchanged.
func (c OTPChallenge) verificationScope() string {
	return c.ID + ":" + strconv.Itoa(c.Generation)
}

// HashFor computes OTPHash for a freshly generated code at the challenge's
// current (or next, for resend) generation.
func (c OTPChallenge) HashFor(secret []byte, otp string, generation int) string {
	scope := c.ID + ":" + strconv.Itoa(generation)
	return sharedauth.HashOTP(secret, scope, c.IdentifierKey(), otp)
}
