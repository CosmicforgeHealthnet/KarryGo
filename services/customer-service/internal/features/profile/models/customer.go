package profilemodels

import "time"

const (
	StatusActive            = "active"
	OnboardingProfileNeeded = "profile_required"
	OnboardingComplete      = "complete"
)

type Customer struct {
	ID               string
	Phone            string
	FirstName        *string
	LastName         *string
	OnboardingStatus string
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type PublicCustomer struct {
	ID               string  `json:"id"`
	Phone            string  `json:"phone"`
	FirstName        *string `json:"first_name"`
	LastName         *string `json:"last_name"`
	OnboardingStatus string  `json:"onboarding_status"`
	Status           string  `json:"status,omitempty"`
}

func (c Customer) Public() PublicCustomer {
	return PublicCustomer{
		ID:               c.ID,
		Phone:            c.Phone,
		FirstName:        c.FirstName,
		LastName:         c.LastName,
		OnboardingStatus: c.OnboardingStatus,
		Status:           c.Status,
	}
}
