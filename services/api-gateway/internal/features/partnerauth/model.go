package partnerauth

import "time"

// APIKey mirrors a row in the partner_api_keys table.
// Only the bcrypt hash and prefix of a key are ever stored; the plaintext key
// is never persisted or logged.
type APIKey struct {
	ID          string
	PartnerID   string
	KeyHash     string
	KeyPrefix   string
	Environment string
	Label       *string
	Status      string
	LastUsedAt  *time.Time
	CreatedAt   time.Time
	RevokedAt   *time.Time
}

const (
	APIKeyStatusActive  = "active"
	APIKeyStatusRevoked = "revoked"

	EnvironmentSandbox = "sandbox"
	EnvironmentLive    = "live"
)
