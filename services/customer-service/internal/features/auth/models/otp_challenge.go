package authmodels

import (
	"time"

	"karrygo/shared/go/apperrors"
	sharedauth "karrygo/shared/go/auth"
)

type OTPChallenge struct {
	ID        string    `json:"id"`
	Phone     string    `json:"phone"`
	OTPHash   string    `json:"otp_hash"`
	Attempts  int       `json:"attempts"`
	ExpiresAt time.Time `json:"expires_at"`
}

func VerifyOTPChallenge(secret []byte, challenge OTPChallenge, challengeID string, otp string, maxAttempts int, now time.Time) error {
	if challenge.ExpiresAt.Before(now) {
		return apperrors.Unauthorized("Invalid or expired verification code.", sharedauth.ErrExpiredToken)
	}
	if challengeID != "" && challengeID != challenge.ID {
		return apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}
	if challenge.Attempts >= maxAttempts {
		return apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}
	if !sharedauth.VerifyOTP(secret, challenge.ID, challenge.Phone, otp, challenge.OTPHash) {
		return apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}

	return nil
}
