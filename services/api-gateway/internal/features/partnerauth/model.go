package partnerauth

import "time"

// APIKey mirrors a row in the partner_api_keys table.
// Only the bcrypt hash and prefix of a key are ever stored; the plaintext key
// is never persisted or logged.
//
// Description, ExpiresAt, and Permissions were added by migration 005.
// Permissions is never nil (the column defaults to '{}'), so callers can range
// over it directly without a nil check.
type APIKey struct {
	ID          string
	PartnerID   string
	KeyHash     string
	KeyPrefix   string
	Environment string
	Label       *string
	Description *string
	ExpiresAt   *time.Time
	Permissions []string
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

// Permission scopes an API key can hold. These correspond 1:1 to the
// "Permissions" checkboxes on the Generate API Key form (Create Deliveries,
// Read Deliveries, Track Deliveries, Manage Webhooks).
const (
	PermissionCreateDeliveries = "create_deliveries"
	PermissionReadDeliveries   = "read_deliveries"
	PermissionTrackDeliveries  = "track_deliveries"
	PermissionManageWebhooks   = "manage_webhooks"
)

var allowedPermissions = map[string]struct{}{
	PermissionCreateDeliveries: {},
	PermissionReadDeliveries:   {},
	PermissionTrackDeliveries:  {},
	PermissionManageWebhooks:   {},
}

// ValidPermission reports whether p is a recognized permission scope string.
// Used by partneraccount.Service.CreateKey to reject unknown values on the
// Generate API Key form before anything is persisted.
func ValidPermission(p string) bool {
	_, ok := allowedPermissions[p]
	return ok
}

// AllPermissions returns every recognized permission scope. Used to grant full
// access to the sandbox key auto-issued at Register - a brand-new partner
// never fills out the Generate API Key form for that first key, so it cannot
// be scoped down by user choice, and defaulting it to zero permissions would
// make it unusable against every permission-gated route immediately after
// signup.
func AllPermissions() []string {
	return []string{
		PermissionCreateDeliveries,
		PermissionReadDeliveries,
		PermissionTrackDeliveries,
		PermissionManageWebhooks,
	}
}