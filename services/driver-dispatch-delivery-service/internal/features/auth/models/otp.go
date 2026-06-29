package authmodels

import (
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

type OTP struct {
	ID          string
	PhoneNumber string
	// Email is set only for signup OTPs; nil for login OTPs.
	// It is retrieved during Verify(purpose=signup) to create the identity with the email.
	Email *string
	// Purpose distinguishes signup OTPs from login OTPs for audit.
	// Values: "login" (default), "signup".
	Purpose     string
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
