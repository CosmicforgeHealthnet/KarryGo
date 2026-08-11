package vendorauthmodels

import "time"

// Session is a revocable refresh session, anchored to exactly one vendor_staff
// row. That anchor is what fixes the session's vendor/branch scope — nothing
// about scope is ever taken from a request body.
type Session struct {
	ID               string
	StaffID          string
	RefreshTokenHash string
	DeviceID         *string
	UserAgent        string
	IPAddress        string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	RevokedReason    *string
	CreatedAt        time.Time
	LastUsedAt       *time.Time
}

func (s Session) IsActive(now time.Time) bool {
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}

// StepUpGrant is a short-lived, single-purpose elevation produced by a
// verified step_up OTP. It authorizes exactly one purpose, for exactly one
// session/staff/vendor, and is consumed on first use.
type StepUpGrant struct {
	ID         string
	SessionID  string
	StaffID    string
	VendorID   string
	Purpose    string
	Status     string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

const (
	StepUpStatusActive   = "active"
	StepUpStatusConsumed = "consumed"
	StepUpStatusExpired  = "expired"
	StepUpStatusRevoked  = "revoked"
)

// Usable reports whether this grant may be consumed right now for exactly the
// given session, staff, vendor and purpose — every dimension must match
// precisely; a grant scoped to one purpose can never authorize another, and a
// grant for one session/staff/vendor can never be used from another, even if
// the same physical person is behind both.
func (g StepUpGrant) Usable(sessionID, staffID, vendorID, purpose string, now time.Time) bool {
	return g.Status == StepUpStatusActive &&
		g.ExpiresAt.After(now) &&
		g.SessionID == sessionID &&
		g.StaffID == staffID &&
		g.VendorID == vendorID &&
		g.Purpose == purpose
}
