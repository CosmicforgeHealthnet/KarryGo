package profilemodels

import "time"

const (
	StatusActive            = "active"
	OnboardingProfileNeeded = "profile_required"
	OnboardingComplete      = "complete"
)

type EmergencyContact struct {
	ID           string
	CustomerID   string
	Name         string
	Phone        string
	Relationship string
	CreatedAt    time.Time
}

type PublicEmergencyContact struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Phone        string `json:"phone"`
	Relationship string `json:"relationship"`
	CreatedAt    time.Time `json:"created_at"`
}

func (e EmergencyContact) Public() PublicEmergencyContact {
	return PublicEmergencyContact{
		ID:           e.ID,
		Name:         e.Name,
		Phone:        e.Phone,
		Relationship: e.Relationship,
		CreatedAt:    e.CreatedAt,
	}
}

type Customer struct {
	ID                 string
	Phone              string
	Email              string
	FirstName          *string
	LastName           *string
	OnboardingStatus   string
	Status             string
	ProfilePhotoURL    *string
	ProfilePhotoAssetID *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type PublicCustomer struct {
	ID               string  `json:"id"`
	Phone            string  `json:"phone,omitempty"`
	Email            string  `json:"email,omitempty"`
	FirstName        *string `json:"first_name"`
	LastName         *string `json:"last_name"`
	OnboardingStatus string  `json:"onboarding_status"`
	Status           string  `json:"status,omitempty"`
	ProfilePhotoURL  *string `json:"profile_photo_url,omitempty"`
}

func (c Customer) Public() PublicCustomer {
	return PublicCustomer{
		ID:              c.ID,
		Phone:           c.Phone,
		Email:           c.Email,
		FirstName:       c.FirstName,
		LastName:        c.LastName,
		OnboardingStatus: c.OnboardingStatus,
		Status:          c.Status,
		ProfilePhotoURL: c.ProfilePhotoURL,
	}
}
