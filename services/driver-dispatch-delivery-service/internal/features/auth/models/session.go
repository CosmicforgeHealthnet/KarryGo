package authmodels

import "time"

type Session struct {
	ID               string
	DispatchRiderID  string
	PhoneNumber      string
	RefreshTokenHash string
	DeviceID         *string
	DeviceType       *string
	IPAddress        string
	UserAgent        string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (s Session) IsActive(now time.Time) bool {
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}
