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
	BusinessName  string
	BusinessEmail string
	PasswordHash  string
	Status        string
	Plan          string
}

// AccountRepository persists partner accounts.
type AccountRepository interface {
	// CreateAccount inserts a new account. It returns ErrEmailTaken on a unique
	// business_email violation.
	CreateAccount(ctx context.Context, params CreateAccountParams) (Account, error)
	// FindByEmail returns the account and its bcrypt password hash, or (nil, "", nil)
	// when no account has that email.
	FindByEmail(ctx context.Context, email string) (*Account, string, error)
	// FindByID returns the account, or (nil, nil) when absent.
	FindByID(ctx context.Context, id string) (*Account, error)
	// UpdatePlan sets the plan; found is false when no such account exists.
	UpdatePlan(ctx context.Context, id, plan string) (found bool, err error)
	// UpdateStatus sets the status; found is false when no such account exists.
	UpdateStatus(ctx context.Context, id, status string) (found bool, err error)
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

// CreateKeyParams is the input for storing a new API key (hash + prefix only).
type CreateKeyParams struct {
	PartnerID   string
	KeyHash     string
	KeyPrefix   string
	Environment string
	Label       string
	Status      string
}

// APIKeyStore manages partner API keys for the dashboard (create/list/revoke).
type APIKeyStore interface {
	CreateKey(ctx context.Context, params CreateKeyParams) (id string, err error)
	ListByPartner(ctx context.Context, partnerID string) ([]KeyView, error)
	// RevokeByPartner revokes a partner's key; found is false when the key does
	// not exist or is not owned by that partner.
	RevokeByPartner(ctx context.Context, partnerID, keyID string) (found bool, err error)
}
