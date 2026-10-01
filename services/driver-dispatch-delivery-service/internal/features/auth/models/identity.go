package authmodels

import "time"

const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusDeleted   = "deleted"

	// RoleDispatchProvider is the only role served by this auth feature.
	RoleDispatchProvider = "dispatch_provider"
)

// Signup is what a sign-up carried besides the identifiers themselves. It is
// used only to announce a brand-new rider to the referral programme.
type Signup struct {
	// LinkCode is a referral code from a shared link; it wins over TypedCode.
	LinkCode  string
	TypedCode string
	DeviceID  string
}

type Identity struct {
	ID          string
	PhoneNumber string
	Email       *string // nullable; set during signup, nil for legacy phone-only accounts
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (i Identity) CanCreateSession() bool {
	return i.Status == StatusActive
}
