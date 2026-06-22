package providerauthmodels

import (
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

type OTPChallenge struct {
	ID              string    `json:"id"`
	IdentifierType  string    `json:"identifier_type"`
	IdentifierValue string    `json:"identifier_value"`
	OTPHash         string    `json:"otp_hash"`
	Attempts        int       `json:"attempts"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func VerifyOTPChallenge(secret []byte, challenge OTPChallenge, challengeID string, otp string, maxAttempts int, now time.Time) error {
	if challenge.ExpiresAt.Before(now) {
		return apperrors.Unauthorized("Invalid or expired verification code.", sharedauth.ErrExpiredToken)
	}
	if challengeID != "" && challengeID != challenge.ID {
		return apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}
	if challenge.Attempts >= maxAttempts {
		return apperrors.Unauthorized("Too many incorrect attempts. Please request a new code.", nil)
	}
	if !sharedauth.VerifyOTP(secret, challenge.ID, challenge.IdentifierKey(), otp, challenge.OTPHash) {
		return apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}
	return nil
}

func (c OTPChallenge) IdentifierKey() string {
	return c.IdentifierType + ":" + c.IdentifierValue
}
