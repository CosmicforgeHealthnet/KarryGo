package partneraccount

import (
	"context"
	"errors"
	"time"
)

// ErrEmailTaken is returned by CreateAccount when the business_email already exists.
var ErrEmailTaken = errors.New("partner email already exists")

// CreateAccountParams is the input for creating a partner account.
type CreateAccountParams struct {
	BusinessName  *string
	BusinessEmail string
	PasswordHash  *string
	Status        string
	Plan          string
}

// CreateOTPChallengeParams is the input for storing a hashed signup OTP.
type CreateOTPChallengeParams struct {
	BusinessEmail string
	OTPCodeHash   string
	ExpiresAt     time.Time
}

// UpdateProfileParams is the input for updating the editable profile fields.
// Pointer fields distinguish "leave unchanged" (nil) from "set this value".
type UpdateProfileParams struct {
	BusinessName    *string
	BusinessPhone   *string
	State           *string
	City            *string
	BusinessLogoURL *string
	ProfilePhotoURL *string
}

// AccountRepository persists partner accounts.
type AccountRepository interface {
	// CreateAccount inserts a new account. It returns ErrEmailTaken on a unique
	// business_email violation.
	CreateAccount(ctx context.Context, params CreateAccountParams) (Account, error)
	// FindByEmail returns the account and its nullable bcrypt password hash, or
	// (nil, nil, nil) when no account has that email.
	FindByEmail(ctx context.Context, email string) (*Account, *string, error)
	// FindByID returns the account, or (nil, nil) when absent.
	FindByID(ctx context.Context, id string) (*Account, error)
	// UpdatePlan sets the plan; found is false when no such account exists.
	UpdatePlan(ctx context.Context, id, plan string) (found bool, err error)
	// UpdateStatus sets the status; found is false when no such account exists.
	UpdateStatus(ctx context.Context, id, status string) (found bool, err error)
	// UpdateProfile applies a partial update to the editable profile fields and
	// returns the account as it is after the update. found is false when no
	// such account exists.
	UpdateProfile(ctx context.Context, id string, params UpdateProfileParams) (account *Account, found bool, err error)
	DeleteAccount(ctx context.Context, partnerID string) (bool, error)
}

// OTPChallengeRepository persists OTP signup challenges.
type OTPChallengeRepository interface {
	CreateOTPChallenge(ctx context.Context, params CreateOTPChallengeParams) (OTPChallenge, error)
	FindLatestOTPChallenge(ctx context.Context, businessEmail string) (*OTPChallenge, error)
	IncrementOTPAttempts(ctx context.Context, challengeID string) (attempts int, err error)
	ConsumeOTPChallenge(ctx context.Context, challengeID string) error
	DeleteOTPChallenge(ctx context.Context, challengeID string) error
}

// CreateSessionParams is the input for a new dashboard session.
type CreateSessionParams struct {
	PartnerID string
	TokenHash string
	IPAddress string
	UserAgent string
	ExpiresAt time.Time
}

// SessionRepository persists partner dashboard sessions (token hashes only).
type SessionRepository interface {
	CreateSession(ctx context.Context, params CreateSessionParams) error
	// FindByTokenHash returns the session, or (nil, nil) when absent.
	FindByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	// Revoke marks the session revoked; idempotent (no error if already revoked/absent).
	Revoke(ctx context.Context, tokenHash string) error
}

// CreateKeyParams is the input for storing a new API key (hash + prefix, plus
// the description/expiry/permissions added by migration 005).
type CreateKeyParams struct {
	PartnerID   string
	KeyHash     string
	KeyPrefix   string
	Environment string
	Label       string
	Status      string
	Description string
	ExpiresAt   *time.Time
	Permissions []string
}

// APIKeyStore manages partner API keys for the dashboard (create/list/revoke).
type APIKeyStore interface {
	CreateKey(ctx context.Context, params CreateKeyParams) (id string, err error)
	ListByPartner(ctx context.Context, partnerID string) ([]KeyView, error)
	// RevokeByPartner revokes a partner's key; found is false when the key does
	// not exist or is not owned by that partner.
	RevokeByPartner(ctx context.Context, partnerID, keyID string) (found bool, err error)
}
