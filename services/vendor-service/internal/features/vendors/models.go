package vendors

import "time"

type BusinessStatus string

const (
	BusinessDraft               BusinessStatus = "DRAFT"
	BusinessPendingVerification BusinessStatus = "PENDING_VERIFICATION"
	BusinessPendingReview       BusinessStatus = "PENDING_REVIEW"
	BusinessActive              BusinessStatus = "ACTIVE"
	BusinessPaused              BusinessStatus = "PAUSED"
	BusinessSuspended           BusinessStatus = "SUSPENDED"
	BusinessClosed              BusinessStatus = "CLOSED"
)

type CapabilityType string

const (
	CapabilityRestaurant   CapabilityType = "RESTAURANT"
	CapabilityFastFood     CapabilityType = "FAST_FOOD"
	CapabilityCloudKitchen CapabilityType = "CLOUD_KITCHEN"
	CapabilityPastryBakery CapabilityType = "PASTRY_BAKERY"
	CapabilityShopGrocery  CapabilityType = "SHOP_GROCERY"
	CapabilityMarket       CapabilityType = "MARKET"
	CapabilityWholesale    CapabilityType = "WHOLESALE"
	// Catering (bulk food) and Manufacturer produce to order rather than
	// serving a menu or a shelf. Section 11 specifies both, the readiness
	// engine has carried their substates since 000117, and migration 000121
	// admits them to the capability vocabulary.
	CapabilityCatering     CapabilityType = "CATERING"
	CapabilityManufacturer CapabilityType = "MANUFACTURER"
)

var SupportedCapabilityTypes = []CapabilityType{
	CapabilityRestaurant, CapabilityFastFood, CapabilityCloudKitchen,
	CapabilityPastryBakery, CapabilityShopGrocery, CapabilityMarket, CapabilityWholesale,
	CapabilityCatering, CapabilityManufacturer,
}

type CapabilityStatus string

const (
	CapabilityDraft         CapabilityStatus = "DRAFT"
	CapabilityPendingReview CapabilityStatus = "PENDING_REVIEW"
	CapabilityActive        CapabilityStatus = "ACTIVE"
	CapabilityPaused        CapabilityStatus = "PAUSED"
	CapabilitySuspended     CapabilityStatus = "SUSPENDED"
	CapabilityRejected      CapabilityStatus = "REJECTED"
)

type VerificationState string

const (
	VerificationNotStarted    VerificationState = "NOT_STARTED"
	VerificationInProgress    VerificationState = "IN_PROGRESS"
	VerificationPendingReview VerificationState = "PENDING_REVIEW"
	VerificationVerified      VerificationState = "VERIFIED"
	VerificationRejected      VerificationState = "REJECTED"
	VerificationExpired       VerificationState = "EXPIRED"
	VerificationSuspended     VerificationState = "SUSPENDED"
)

type BranchStatus string

const (
	BranchDraft     BranchStatus = "DRAFT"
	BranchActive    BranchStatus = "ACTIVE"
	BranchPaused    BranchStatus = "PAUSED"
	BranchSuspended BranchStatus = "SUSPENDED"
	BranchClosed    BranchStatus = "CLOSED"
)

type BranchCapabilityStatus string

const (
	BranchCapabilityDraft     BranchCapabilityStatus = "DRAFT"
	BranchCapabilityActive    BranchCapabilityStatus = "ACTIVE"
	BranchCapabilityPaused    BranchCapabilityStatus = "PAUSED"
	BranchCapabilitySuspended BranchCapabilityStatus = "SUSPENDED"
)

type Business struct {
	ID           string         `json:"id"`
	LegalName    string         `json:"legal_name"`
	TradingName  string         `json:"trading_name,omitempty"`
	Status       BusinessStatus `json:"status"`
	OwnerRef     string         `json:"owner_ref"`
	SupportEmail string         `json:"support_email,omitempty"`
	SupportPhone string         `json:"support_phone,omitempty"`
	Currency     string         `json:"currency"`
	Timezone     string         `json:"timezone"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Version      int64          `json:"version"`
	Capabilities []Capability   `json:"capabilities,omitempty"`
}

type Capability struct {
	ID             string           `json:"id"`
	BusinessID     string           `json:"business_id"`
	CapabilityType CapabilityType   `json:"capability_type"`
	Status         CapabilityStatus `json:"status"`
	StatusReason   string           `json:"status_reason,omitempty"`
	RequestedAt    time.Time        `json:"requested_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	Version        int64            `json:"version"`
}

type MediaReference struct {
	Kind     string `json:"kind"`
	MediaRef string `json:"media_ref"`
}

type Brand struct {
	ID                string            `json:"id"`
	BusinessID        string            `json:"business_id"`
	DisplayName       string            `json:"display_name"`
	Description       string            `json:"description"`
	MediaRefs         []MediaReference  `json:"media_refs"`
	VerificationState VerificationState `json:"verification_state"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	Version           int64             `json:"version"`
}

type Address struct {
	Line1       string `json:"line1"`
	Line2       string `json:"line2,omitempty"`
	City        string `json:"city"`
	Region      string `json:"region"`
	PostalCode  string `json:"postal_code,omitempty"`
	CountryCode string `json:"country_code"`
}

type OperatingWindow struct {
	ID       string `json:"id,omitempty"`
	Weekday  int    `json:"weekday"`
	IsClosed bool   `json:"is_closed"`
	OpensAt  string `json:"opens_at,omitempty"`
	ClosesAt string `json:"closes_at,omitempty"`
	Version  int64  `json:"version,omitempty"`
}

type ServiceArea struct {
	Mode            string  `json:"mode"`
	CenterLatitude  float64 `json:"center_latitude"`
	CenterLongitude float64 `json:"center_longitude"`
	RadiusMeters    int     `json:"radius_meters"`
	Version         int64   `json:"version"`
}

type OperationsSettings struct {
	SchemaVersion      int   `json:"schema_version"`
	AcceptingNewOrders bool  `json:"accepting_new_orders"`
	PickupEnabled      bool  `json:"pickup_enabled"`
	Version            int64 `json:"version"`
}

type BranchCapability struct {
	ID                 string                 `json:"id"`
	BusinessID         string                 `json:"business_id"`
	BranchID           string                 `json:"branch_id"`
	VendorCapabilityID string                 `json:"vendor_capability_id"`
	CapabilityType     CapabilityType         `json:"capability_type"`
	Status             BranchCapabilityStatus `json:"status"`
	Effective          bool                   `json:"effective"`
	EffectiveReason    string                 `json:"effective_reason"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	Version            int64                  `json:"version"`
}

type Branch struct {
	ID           string              `json:"id"`
	BusinessID   string              `json:"business_id"`
	BrandID      string              `json:"brand_id,omitempty"`
	DisplayName  string              `json:"display_name"`
	Address      Address             `json:"address"`
	Latitude     float64             `json:"latitude"`
	Longitude    float64             `json:"longitude"`
	Timezone     string              `json:"timezone"`
	Status       BranchStatus        `json:"status"`
	Hours        []OperatingWindow   `json:"hours,omitempty"`
	ServiceArea  *ServiceArea        `json:"service_area,omitempty"`
	Settings     *OperationsSettings `json:"operations_settings,omitempty"`
	Capabilities []BranchCapability  `json:"capabilities,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	Version      int64               `json:"version"`
}

func IsSupportedCapability(t CapabilityType) bool {
	for _, supported := range SupportedCapabilityTypes {
		if t == supported {
			return true
		}
	}
	return false
}

func CapabilityFamily(t CapabilityType) string {
	switch t {
	case CapabilityRestaurant, CapabilityFastFood, CapabilityCloudKitchen, CapabilityPastryBakery:
		return "FOOD"
	case CapabilityShopGrocery, CapabilityMarket, CapabilityWholesale:
		return "SHOP"
	// Produced to order: neither a menu nor a shelf. The family is coarse --
	// it describes what the business does, not which catalogue module admits
	// it, which is why CLOUD_KITCHEN is FOOD without being a food-menu
	// capability.
	case CapabilityCatering:
		return "FOOD"
	case CapabilityManufacturer:
		return "GOODS"
	default:
		return "UNKNOWN"
	}
}

func EffectiveCapabilityState(business BusinessStatus, capability CapabilityStatus, branch BranchStatus, branchCapability BranchCapabilityStatus) (bool, string) {
	if business != BusinessActive {
		return false, "BUSINESS_NOT_ACTIVE"
	}
	if capability != CapabilityActive {
		return false, "BUSINESS_CAPABILITY_NOT_ACTIVE"
	}
	if branch != BranchActive {
		return false, "BRANCH_NOT_ACTIVE"
	}
	if branchCapability != BranchCapabilityActive {
		return false, "BRANCH_CAPABILITY_NOT_ACTIVE"
	}
	return true, "ACTIVE"
}

func vendorBusinessTransitionAllowed(from, to BusinessStatus) bool {
	return (from == BusinessActive && to == BusinessPaused) || (from == BusinessPaused && to == BusinessActive)
}

func capabilityTransitionAllowed(from, to CapabilityStatus) bool {
	return (from == CapabilityActive && to == CapabilityPaused) || (from == CapabilityPaused && to == CapabilityActive)
}

func branchTransitionAllowed(from, to BranchStatus) bool {
	return (from == BranchActive && to == BranchPaused) || (from == BranchPaused && to == BranchActive)
}

func branchCapabilityTransitionAllowed(from, to BranchCapabilityStatus) bool {
	return (from == BranchCapabilityActive && to == BranchCapabilityPaused) || (from == BranchCapabilityPaused && to == BranchCapabilityActive)
}
