package profile

import "time"

type ProviderSettings struct {
	ProviderID      string    `json:"provider_id"`
	PushEnabled     bool      `json:"push_enabled"`
	SMSEnabled      bool      `json:"sms_enabled"`
	Language        string    `json:"language"`
	DarkModeEnabled bool      `json:"dark_mode_enabled"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type UpdateSettingsInput struct {
	PushEnabled     *bool   `json:"push_enabled,omitempty"`
	SMSEnabled      *bool   `json:"sms_enabled,omitempty"`
	Language        *string `json:"language,omitempty"`
	DarkModeEnabled *bool   `json:"dark_mode_enabled,omitempty"`
}

type OperationType string

const (
	OperationIndividual OperationType = "individual"
	OperationFleet      OperationType = "fleet"
)

type VerificationStatus string

const (
	StatusUnverified    VerificationStatus = "unverified"
	StatusPendingReview VerificationStatus = "pending_review"
	StatusVerified      VerificationStatus = "verified"
	StatusSuspended     VerificationStatus = "suspended"
	StatusRejected      VerificationStatus = "rejected"
)

type Provider struct {
	ID                 string             `json:"id"`
	Phone              string             `json:"phone"`
	FullName           *string            `json:"full_name,omitempty"`
	Email              *string            `json:"email,omitempty"`
	State              *string            `json:"state,omitempty"`
	City               *string            `json:"city,omitempty"`
	Country            string             `json:"country"`
	ProfilePhotoURL    *string            `json:"profile_photo_url,omitempty"`
	OperationType      *OperationType     `json:"operation_type,omitempty"`
	VerificationStatus VerificationStatus `json:"verification_status"`
	AvgRating          float64            `json:"avg_rating"`
	TotalTrips         int                `json:"total_trips"`
	IsActive           bool               `json:"is_active"`
	OnboardingComplete bool               `json:"onboarding_complete"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

type EmergencyContact struct {
	ID           string    `json:"id"`
	ProviderID   string    `json:"provider_id"`
	FullName     string    `json:"full_name"`
	Phone        string    `json:"phone"`
	Relationship string    `json:"relationship"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Guarantor struct {
	ID         string    `json:"id"`
	ProviderID string    `json:"provider_id"`
	FullName   string    `json:"full_name"`
	Phone      string    `json:"phone"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Rating struct {
	ID                string    `json:"id"`
	ProviderID        string    `json:"provider_id"`
	BookingID         string    `json:"booking_id"`
	RatedByCustomerID string    `json:"rated_by_customer_id"`
	Score             int       `json:"score"`
	Comment           *string   `json:"comment,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type PublicProfile struct {
	ProviderID         string             `json:"provider_id"`
	FullName           *string            `json:"full_name,omitempty"`
	ProfilePhotoURL    *string            `json:"profile_photo_url,omitempty"`
	VerificationStatus VerificationStatus `json:"verification_status"`
	AvgRating          float64            `json:"avg_rating"`
	TotalTrips         int                `json:"total_trips"`
}

type Stats struct {
	TotalTrips         int                `json:"total_trips"`
	AvgRating          float64            `json:"avg_rating"`
	RatingsCount       int                `json:"ratings_count"`
	CompletionRate     float64            `json:"completion_rate"`
	IsActive           bool               `json:"is_active"`
	VerificationStatus VerificationStatus `json:"verification_status"`
}

type MeResponse struct {
	ProviderID          string             `json:"provider_id"`
	SupportID           string             `json:"support_id"`
	Phone               string             `json:"phone"`
	FullName            *string            `json:"full_name,omitempty"`
	Email               *string            `json:"email,omitempty"`
	State               *string            `json:"state,omitempty"`
	City                *string            `json:"city,omitempty"`
	Country             string             `json:"country"`
	ProfilePhotoURL     *string            `json:"profile_photo_url,omitempty"`
	OperationType       *OperationType     `json:"operation_type,omitempty"`
	VerificationStatus  VerificationStatus `json:"verification_status"`
	AvgRating           float64            `json:"avg_rating"`
	TotalTrips          int                `json:"total_trips"`
	IsActive            bool               `json:"is_active"`
	OnboardingComplete  bool               `json:"onboarding_complete"`
	HasEmergencyContact bool               `json:"has_emergency_contact"`
	HasGuarantor        bool               `json:"has_guarantor"`
	CreatedAt           time.Time          `json:"created_at"`
}
