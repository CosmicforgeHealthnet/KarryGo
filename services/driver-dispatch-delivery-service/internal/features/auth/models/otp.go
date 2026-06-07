package authmodels

import (
	"time"

	"karrygo/shared/go/apperrors"
)

type OTP struct {
	ID          string
	PhoneNumber string
	OTPCodeHash string
	Attempts    int
	MaxAttempts int
	ExpiresAt   time.Time
	Verified    bool
	LockedUntil *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (o OTP) IsExpired(now time.Time) bool {
	return !o.ExpiresAt.After(now)
}

func (o OTP) IsVerified() bool {
	return o.Verified
}

func (o OTP) IsLocked(now time.Time) bool {
	return o.LockedUntil != nil && o.LockedUntil.After(now)
}

func ValidateOTPCode(code string) error {
	if len(code) != 6 {
		return apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "otp_code", Message: "Verification code must be 6 digits."},
		})
	}

	for _, char := range code {
		if char < '0' || char > '9' {
			return apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "otp_code", Message: "Verification code must be 6 digits."},
			})
		}
	}

	return nil
}
