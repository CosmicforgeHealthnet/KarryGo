// Package partneraccount implements the partner DASHBOARD auth system: email +
// password login, browser session JWTs, API key management, OTP web signup,
// and internal service-to-service account controls. This is entirely separate
// from the partner API KEY auth (partnerauth) used by partner backends for
// delivery calls.
package partneraccount

import "time"

// Account statuses.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusPending   = "pending"
	StatusDisabled  = "disabled"
)

// Plans.
const (
	PlanSandbox = "sandbox"
	PlanLive    = "live"
)

// Environments.
const (
	EnvironmentSandbox = "sandbox"
	EnvironmentLive    = "live"
)

// Account is the public partner account view. It NEVER contains password_hash.
// BusinessName and the profile fields are nullable because OTP-created
// onboarding accounts may exist before the partner has filled in profile data.
type Account struct {
	ID              string    `json:"id"`
	BusinessName    *string   `json:"business_name"`
	BusinessEmail   string    `json:"business_email"`
	BusinessPhone   *string   `json:"business_phone"`
	State           *string   `json:"state"`
	City            *string   `json:"city"`
	BusinessLogoURL *string   `json:"business_logo_url"`
	ProfilePhotoURL *string   `json:"profile_photo_url"`
	Status          string    `json:"status"`
	Plan            string    `json:"plan"`
	CreatedAt       time.Time `json:"created_at"`
}

// OTPChallenge mirrors a partner_otp_challenges row. OTPCodeHash is SHA-256 hex;
// plaintext OTP codes are never stored.
type OTPChallenge struct {
	ID            string
	BusinessEmail string
	OTPCodeHash   string
	Attempts      int
	ConsumedAt    *time.Time
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

// Session mirrors a partner_sessions row.
type Session struct {
	ID        string
	PartnerID string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// KeyView is the safe API key listing shape (no key_hash, no raw key).
//
// Description, ExpiresAt, and Permissions were added by migration 005 (the
// Generate API Key form's description/expiration/permissions fields).
// Permissions is never nil (defaults to '{}' at the DB level).
type KeyView struct {
	ID          string     `json:"id"`
	Prefix      string     `json:"prefix"`
	Environment string     `json:"environment"`
	Label       string     `json:"label"`
	Description string     `json:"description"`
	ExpiresAt   *time.Time `json:"expires_at"`
	Permissions []string   `json:"permissions"`
	Status      string     `json:"status"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

func isActive(status string) bool { return status == StatusActive }
