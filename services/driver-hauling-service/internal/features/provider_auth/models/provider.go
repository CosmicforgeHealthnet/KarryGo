package providerauthmodels

import "time"

// Signup is what a sign-up carried besides the identifier itself. It is used
// only to announce a brand-new provider to the referral programme.
type Signup struct {
	// LinkCode is a referral code from a shared link; it wins over TypedCode.
	LinkCode  string
	TypedCode string
	DeviceID  string
}

const (
	ProviderStatusActive    = "active"
	ProviderStatusSuspended = "suspended"

	OnboardingProfileNeeded       = "profile_required"
	OnboardingPendingVerification = "pending_verification"
	OnboardingComplete            = "complete"
)

type Provider struct {
	ID                  string
	Phone               string
	Email               string
	FirstName           string
	LastName            string
	OnboardingStatus    string
	Status              string
	ProfilePhotoURL     *string
	PhotoAssetID        *string
	Rating              float64
	TotalTrips          int
	LocationState       string
	LocationCity        string
	Language            string
	ServiceType         string
	OperationMode       string
	DriverLicenseNumber string
	LicenseExpiryYear   string
	LicenseExpiryDate   string
	GovIDURL            string
	DriverLicenseURL    string
	VehicleRegURL       string
	GuarantorName       string
	GuarantorPhone      string

	EmergencyContactName         string
	EmergencyContactPhone        string
	EmergencyContactRelationship string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type PublicProvider struct {
	ID                  string  `json:"id"`
	Phone               string  `json:"phone,omitempty"`
	Email               string  `json:"email,omitempty"`
	FirstName           string  `json:"first_name"`
	LastName            string  `json:"last_name"`
	OnboardingStatus    string  `json:"onboarding_status"`
	Status              string  `json:"status"`
	ProfilePhotoURL     *string `json:"profile_photo_url,omitempty"`
	Rating              float64 `json:"rating"`
	TotalTrips          int     `json:"total_trips"`
	LocationState       string  `json:"location_state"`
	LocationCity        string  `json:"location_city"`
	Language            string  `json:"language"`
	ServiceType         string  `json:"service_type"`
	OperationMode       string  `json:"operation_mode"`
	DriverLicenseNumber string  `json:"driver_license_number"`
	LicenseExpiryYear   string  `json:"license_expiry_year"`
	LicenseExpiryDate   string  `json:"license_expiry_date"`
	GovIDURL            string  `json:"gov_id_url"`
	DriverLicenseURL    string  `json:"driver_license_url"`
	VehicleRegURL       string  `json:"vehicle_reg_url"`
	GuarantorName       string  `json:"guarantor_name"`
	GuarantorPhone      string  `json:"guarantor_phone"`

	EmergencyContactName         string `json:"emergency_contact_name"`
	EmergencyContactPhone        string `json:"emergency_contact_phone"`
	EmergencyContactRelationship string `json:"emergency_contact_relationship"`

	CreatedAt time.Time `json:"created_at"`
}

func (p Provider) Public() PublicProvider {
	return PublicProvider{
		ID:                  p.ID,
		Phone:               p.Phone,
		Email:               p.Email,
		FirstName:           p.FirstName,
		LastName:            p.LastName,
		OnboardingStatus:    p.OnboardingStatus,
		Status:              p.Status,
		ProfilePhotoURL:     p.ProfilePhotoURL,
		Rating:              p.Rating,
		TotalTrips:          p.TotalTrips,
		LocationState:       p.LocationState,
		LocationCity:        p.LocationCity,
		Language:            p.Language,
		ServiceType:         p.ServiceType,
		OperationMode:       p.OperationMode,
		DriverLicenseNumber: p.DriverLicenseNumber,
		LicenseExpiryYear:   p.LicenseExpiryYear,
		LicenseExpiryDate:   p.LicenseExpiryDate,
		GovIDURL:            p.GovIDURL,
		DriverLicenseURL:    p.DriverLicenseURL,
		VehicleRegURL:       p.VehicleRegURL,
		GuarantorName:       p.GuarantorName,
		GuarantorPhone:      p.GuarantorPhone,

		EmergencyContactName:         p.EmergencyContactName,
		EmergencyContactPhone:        p.EmergencyContactPhone,
		EmergencyContactRelationship: p.EmergencyContactRelationship,

		CreatedAt: p.CreatedAt,
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
