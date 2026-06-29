package authmodels

import "time"

const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusDeleted   = "deleted"

	// RoleDispatchProvider is the only role served by this auth feature.
	RoleDispatchProvider = "dispatch_provider"
)

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
