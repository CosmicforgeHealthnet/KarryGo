// Package partneraccount implements the partner DASHBOARD auth system: email +
// password login, browser session JWTs, API key management, and internal
// service-to-service account controls. This is entirely separate from the
// partner API KEY auth (partnerauth) used by partner backends for delivery calls.
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
type Account struct {
	ID            string    `json:"id"`
	BusinessName  string    `json:"business_name"`
	BusinessEmail string    `json:"business_email"`
	Status        string    `json:"status"`
	Plan          string    `json:"plan"`
	CreatedAt     time.Time `json:"created_at"`
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
type KeyView struct {
	ID          string     `json:"id"`
	Prefix      string     `json:"prefix"`
	Environment string     `json:"environment"`
	Label       string     `json:"label"`
	Status      string     `json:"status"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

func isActive(status string) bool { return status == StatusActive }
