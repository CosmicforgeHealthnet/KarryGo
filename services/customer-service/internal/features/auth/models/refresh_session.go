package authmodels

import "time"

type RefreshSession struct {
	ID               string
	CustomerID       string
	RefreshTokenHash string
	DeviceID         *string
	UserAgent        string
	IPAddress        string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	CreatedAt        time.Time
}

func (s RefreshSession) IsActive(now time.Time) bool {
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}
