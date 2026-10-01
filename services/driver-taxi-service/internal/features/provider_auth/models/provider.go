package providerauthmodels

import "time"

// Signup is what a sign-up carried besides the phone itself. It is used only
// to announce a brand-new driver to the referral programme.
type Signup struct {
	// LinkCode is a referral code from a shared link; it wins over TypedCode.
	LinkCode  string
	TypedCode string
	DeviceID  string
}

// Addendum §11.1: six independent status dimensions. No single generic status
// field is expected to represent all of them — a provider can be
// onboarding-complete and identity-verified while their vehicle document has
// expired, and each of those must be checkable on its own.
const (
	AccountStatusActive    = "active"
	AccountStatusSuspended = "suspended"

	OnboardingProfileNeeded = "profile_required"
	OnboardingComplete      = "complete"

	IdentityVerificationNotStarted = "not_started"
	IdentityVerificationPending    = "pending"
	IdentityVerificationVerified   = "verified"
	IdentityVerificationFailed     = "failed"

	VehicleOperationalNone     = "none"
	VehicleOperationalActive   = "active"
	VehicleOperationalInactive = "inactive"

	VehicleVerificationNotStarted = "not_started"
	VehicleVerificationPending    = "pending"
	VehicleVerificationVerified   = "verified"
	VehicleVerificationFailed     = "failed"

	DocumentComplianceNotStarted = "not_started"
	DocumentCompliancePending    = "pending"
	DocumentComplianceCompliant  = "compliant"
	DocumentComplianceExpired    = "expired"
)

// Provider is a taxi driver. Document references are plain URLs pointing at
// media-file-service objects; this service stores no binary content and owns no
// verification decision — it only records the status those other systems hand
// back.
type Provider struct {
	ID              string
	Phone           string
	Email           string
	FirstName       string
	LastName        string
	ProfilePhotoURL *string
	PhotoAssetID    *string
	Rating          float64
	TotalTrips      int

	// The six compliance/readiness dimensions. See the constants above for the
	// valid values of each.
	AccountStatus              string
	OnboardingStatus           string
	IdentityVerificationStatus string
	VehicleOperationalStatus   string
	VehicleVerificationStatus  string
	DocumentComplianceStatus   string
	// AcceptsParcels is the driver's opt-in to carrying packages. It is NOT
	// a compliance dimension and deliberately not part of CanGoAvailable --
	// going online to drive passengers only is entirely legitimate.
	AcceptsParcels bool

	LocationState string
	LocationCity  string
	Language      string
	ServiceType   string
	OperationMode string

	DriverLicenseNumber string
	LicenseExpiryDate   string
	GovIDURL            string
	DriverLicenseURL    string
	VehicleRegURL       string

	GuarantorName  string
	GuarantorPhone string

	EmergencyContactName         string
	EmergencyContactPhone        string
	EmergencyContactRelationship string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type PublicProvider struct {
	ID              string  `json:"id"`
	Phone           string  `json:"phone,omitempty"`
	Email           string  `json:"email,omitempty"`
	FirstName       string  `json:"first_name"`
	LastName        string  `json:"last_name"`
	ProfilePhotoURL *string `json:"profile_photo_url,omitempty"`
	Rating          float64 `json:"rating"`
	TotalTrips      int     `json:"total_trips"`

	AccountStatus              string `json:"account_status"`
	OnboardingStatus           string `json:"onboarding_status"`
	IdentityVerificationStatus string `json:"identity_verification_status"`
	VehicleOperationalStatus   string `json:"vehicle_operational_status"`
	VehicleVerificationStatus  string `json:"vehicle_verification_status"`
	DocumentComplianceStatus   string `json:"document_compliance_status"`
	AcceptsParcels             bool   `json:"accepts_parcels"`

	LocationState string `json:"location_state"`
	LocationCity  string `json:"location_city"`
	Language      string `json:"language"`
	ServiceType   string `json:"service_type"`
	OperationMode string `json:"operation_mode"`

	DriverLicenseNumber string `json:"driver_license_number"`
	LicenseExpiryDate   string `json:"license_expiry_date"`
	GovIDURL            string `json:"gov_id_url"`
	DriverLicenseURL    string `json:"driver_license_url"`
	VehicleRegURL       string `json:"vehicle_reg_url"`

	GuarantorName  string `json:"guarantor_name"`
	GuarantorPhone string `json:"guarantor_phone"`

	EmergencyContactName         string `json:"emergency_contact_name"`
	EmergencyContactPhone        string `json:"emergency_contact_phone"`
	EmergencyContactRelationship string `json:"emergency_contact_relationship"`

	CreatedAt time.Time `json:"created_at"`
}

func (p Provider) Public() PublicProvider {
	return PublicProvider{
		ID:              p.ID,
		Phone:           p.Phone,
		Email:           p.Email,
		FirstName:       p.FirstName,
		LastName:        p.LastName,
		ProfilePhotoURL: p.ProfilePhotoURL,
		Rating:          p.Rating,
		TotalTrips:      p.TotalTrips,

		AccountStatus:              p.AccountStatus,
		OnboardingStatus:           p.OnboardingStatus,
		IdentityVerificationStatus: p.IdentityVerificationStatus,
		VehicleOperationalStatus:   p.VehicleOperationalStatus,
		VehicleVerificationStatus:  p.VehicleVerificationStatus,
		DocumentComplianceStatus:   p.DocumentComplianceStatus,
		AcceptsParcels:             p.AcceptsParcels,

		LocationState: p.LocationState,
		LocationCity:  p.LocationCity,
		Language:      p.Language,
		ServiceType:   p.ServiceType,
		OperationMode: p.OperationMode,

		DriverLicenseNumber: p.DriverLicenseNumber,
		LicenseExpiryDate:   p.LicenseExpiryDate,
		GovIDURL:            p.GovIDURL,
		DriverLicenseURL:    p.DriverLicenseURL,
		VehicleRegURL:       p.VehicleRegURL,

		GuarantorName:  p.GuarantorName,
		GuarantorPhone: p.GuarantorPhone,

		EmergencyContactName:         p.EmergencyContactName,
		EmergencyContactPhone:        p.EmergencyContactPhone,
		EmergencyContactRelationship: p.EmergencyContactRelationship,

		CreatedAt: p.CreatedAt,
	}
}

func (p Provider) RequiresProfile() bool {
	return p.OnboardingStatus == OnboardingProfileNeeded
}

func (p Provider) IsSuspended() bool {
	return p.AccountStatus == AccountStatusSuspended
}

// Reason codes CanGoAvailable returns, exported so callers (the availability
// usecase in particular) reference the same symbols rather than matching
// string literals by coincidence.
const (
	ReasonAccountSuspended             = "account_suspended"
	ReasonOnboardingIncomplete         = "onboarding_incomplete"
	ReasonIdentityVerificationRequired = "identity_verification_required"
	ReasonVehicleVerificationRequired  = "vehicle_verification_required"
	ReasonDocumentComplianceRequired   = "document_compliance_required"
)

// CanGoAvailable centralizes five of the six compliance dimensions so the
// availability usecase and any future admin view read one function instead of
// scattering comparisons across callers. It deliberately excludes
// VehicleOperationalStatus: that dimension has a second, more trustworthy
// source of truth (a live COUNT against the vehicles table, already built and
// tested in provider_profile/usecases), and gating on a denormalized column
// here as well would let the two drift apart — the vehicles table could gain
// an active vehicle while this cached field still said "none" after a write
// that updated one but not the other. The availability usecase checks vehicle
// readiness itself, live, and calls this method only for the other five.
//
// ok is false as soon as the first failing dimension is found; reason names
// exactly which one, so the caller can return a specific error code rather
// than a generic "cannot go online".
func (p Provider) CanGoAvailable() (ok bool, reason string) {
	switch {
	case p.AccountStatus == AccountStatusSuspended:
		return false, ReasonAccountSuspended
	case p.OnboardingStatus != OnboardingComplete:
		return false, ReasonOnboardingIncomplete
	case p.IdentityVerificationStatus != IdentityVerificationVerified:
		return false, ReasonIdentityVerificationRequired
	case p.VehicleVerificationStatus != VehicleVerificationVerified:
		return false, ReasonVehicleVerificationRequired
	case p.DocumentComplianceStatus != DocumentComplianceCompliant:
		return false, ReasonDocumentComplianceRequired
	default:
		return true, ""
	}
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
