package providerauthmodels

import "time"

const (
	ProviderStatusActive    = "active"
	ProviderStatusSuspended = "suspended"

	OnboardingProfileNeeded       = "profile_required"
	OnboardingPendingVerification = "pending_verification"
	OnboardingComplete            = "complete"
)

type Provider struct {
	ID               string
	Phone            string
	Email            string
	FirstName        string
	LastName         string
	OnboardingStatus string
	Status           string
	ProfilePhotoURL  *string
	PhotoAssetID     *string
	Rating           float64
	TotalTrips       int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type PublicProvider struct {
	ID               string  `json:"id"`
	Phone            string  `json:"phone,omitempty"`
	Email            string  `json:"email,omitempty"`
	FirstName        string  `json:"first_name"`
	LastName         string  `json:"last_name"`
	OnboardingStatus string  `json:"onboarding_status"`
	Status           string  `json:"status"`
	ProfilePhotoURL  *string `json:"profile_photo_url,omitempty"`
	Rating           float64 `json:"rating"`
	TotalTrips       int     `json:"total_trips"`
}

func (p Provider) Public() PublicProvider {
	return PublicProvider{
		ID:               p.ID,
		Phone:            p.Phone,
		Email:            p.Email,
		FirstName:        p.FirstName,
		LastName:         p.LastName,
		OnboardingStatus: p.OnboardingStatus,
		Status:           p.Status,
		ProfilePhotoURL:  p.ProfilePhotoURL,
		Rating:           p.Rating,
		TotalTrips:       p.TotalTrips,
	}
}

func (p Provider) RequiresProfile() bool {
	return p.OnboardingStatus == OnboardingProfileNeeded
}

type RefreshSession struct {
	ID               string
	ProviderID       string
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
