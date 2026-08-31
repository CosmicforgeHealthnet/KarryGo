package delivery

import (
	"errors"
	"time"
)

// DeliveryMode is the customer-facing execution preference configured for a
// branch. PLATFORM never attempts a vendor rider; VENDOR never silently falls
// through to platform; HYBRID follows the explicit fallback triggers.
type DeliveryMode string

const (
	ModePlatform DeliveryMode = "PLATFORM"
	ModeVendor   DeliveryMode = "VENDOR"
	ModeHybrid   DeliveryMode = "HYBRID"
)

// FeeRuleKind identifies the deterministic vendor-owned fee rule.
type FeeRuleKind string

const (
	FeeFlat         FeeRuleKind = "FLAT"
	FeeZoneBased    FeeRuleKind = "ZONE_BASED"
	FeeDistanceTier FeeRuleKind = "DISTANCE_TIER"
)

// OverflowTrigger is a bounded reason for a HYBRID platform fallback.
type OverflowTrigger string

const (
	TriggerOwnRiderFirst OverflowTrigger = "OWN_RIDER_FIRST"
	TriggerDistance      OverflowTrigger = "DISTANCE_OVERFLOW"
	TriggerTime          OverflowTrigger = "TIME_OVERFLOW"
	TriggerCapacity      OverflowTrigger = "CAPACITY_OVERFLOW"
	TriggerVehicle       OverflowTrigger = "VEHICLE_OVERFLOW"
	TriggerSLA           OverflowTrigger = "SLA_OVERFLOW"
)

// FallbackFundingSponsor attributes the automatic HYBRID cost gap. Platform
// and combined values require a pre-authorized platform approval reference;
// ordinary Vendor staff may configure only Vendor funding.
type FallbackFundingSponsor string

const (
	FallbackFundingVendor   FallbackFundingSponsor = "VENDOR"
	FallbackFundingPlatform FallbackFundingSponsor = "PLATFORM"
	FallbackFundingCombined FallbackFundingSponsor = "COMBINED"
)

// RiderAvailability is deliberately tri-state. UNKNOWN is fail-closed and
// can never be treated as an available vendor execution resource.
type RiderAvailability string

const (
	RiderAvailable   RiderAvailability = "AVAILABLE"
	RiderUnavailable RiderAvailability = "UNAVAILABLE"
	RiderUnknown     RiderAvailability = "UNKNOWN"
)

// ServiceZone is a structured, branch-owned zone. Polygon authority is not
// invented here; the existing location/distance authority resolves ZoneID.
// Distance bounds provide a deterministic structured fallback where only
// radius evidence exists.
type ServiceZone struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Enabled           bool   `json:"enabled"`
	MinDistanceMeters int64  `json:"min_distance_meters"`
	MaxDistanceMeters int64  `json:"max_distance_meters"`
	FeeMinor          int64  `json:"fee_minor"`
	Priority          int    `json:"priority"`
}

// DistanceTier uses half-open bounds [Min, Max), except a zero Max which
// means unbounded. Integer metres avoid floating-point boundary ambiguity.
type DistanceTier struct {
	MinDistanceMeters int64 `json:"min_distance_meters"`
	MaxDistanceMeters int64 `json:"max_distance_meters"`
	FeeMinor          int64 `json:"fee_minor"`
}

// OperatingPeriod is a local-clock delivery window. End <= Start means an
// overnight period (for example 22:00 -> 02:00). Weekday uses ISO 1..7.
type OperatingPeriod struct {
	Weekday     int `json:"weekday"`
	StartMinute int `json:"start_minute"`
	EndMinute   int `json:"end_minute"`
}

// Permission codes seeded by migration 000077.
const (
	PermPolicyRead    = "vendor.delivery.policy.read"
	PermPolicyManage  = "vendor.delivery.policy.manage"
	PermVendorOperate = "vendor.delivery.operate"
)

// Audit action and outbox event type for a policy mutation.
const (
	AuditPolicyUpserted     = "delivery.policy.upserted"
	EventPolicyChanged      = "vendor.delivery.policy.changed"
	EventVendorSelected     = "fulfilment.vendor_selected"
	EventPlatformSelected   = "fulfilment.platform_selected"
	EventHybridFallback     = "fulfilment.hybrid_fallback"
	EventCustodyTransferred = "fulfilment.custody_transferred"
	EventReturnRequired     = "fulfilment.return_required"

	AggregateType = "vendor_delivery_policy"
)

var (
	// ErrForbidden is authorization failure: the actor lacks the permission,
	// or holds it but not for the branch it is being used against.
	ErrForbidden = errors.New("delivery: forbidden")

	// ErrNotFound is a missing policy, or one belonging to another tenant —
	// deliberately the same error, so a probe cannot tell the two apart.
	ErrNotFound = errors.New("delivery: not found")

	// ErrInvalid is malformed or incoherent input.
	ErrInvalid = errors.New("delivery: invalid input")

	// ErrVersionConflict is an optimistic-concurrency failure.
	ErrVersionConflict = errors.New("delivery: version conflict")

	// ErrDisabled means VENDOR_OWN_DELIVERY_ENABLED is off for this
	// deployment. It is distinct from a vendor switching its own policy off.
	ErrDisabled = errors.New("delivery: vendor own delivery is not enabled")
)

// Policy is one branch's Vendor Own Delivery configuration.
//
// It configures reach, charge and the minimum/free rules. It deliberately
// carries no beneficiary field: see doc.go.
type Policy struct {
	ID string `json:"id"`

	BusinessID string `json:"-"`
	BranchID   string `json:"branch_id"`

	Mode    DeliveryMode `json:"delivery_mode"`
	FeeRule FeeRuleKind  `json:"fee_rule"`

	Enabled bool `json:"enabled"`

	Currency string `json:"currency"`

	BaseFeeMinor  int64 `json:"base_fee_minor"`
	PerKmFeeMinor int64 `json:"per_km_fee_minor"`

	MinimumOrderMinor int64 `json:"minimum_order_minor"`

	// Nil means this vendor never gives free delivery, which is different
	// from a threshold of zero ("always free").
	FreeDeliveryThresholdMinor *int64 `json:"free_delivery_threshold_minor,omitempty"`

	// Nil means "use the branch service area unchanged". A value may only
	// narrow it.
	MaxDeliveryRadiusMeters *int `json:"max_delivery_radius_meters,omitempty"`

	MaxPackageWeightGrams     *int64                 `json:"max_package_weight_grams,omitempty"`
	MaxPackageCount           *int64                 `json:"max_package_count,omitempty"`
	MaxVolumeMl               *int64                 `json:"max_volume_ml,omitempty"`
	OperatingTimezone         string                 `json:"operating_timezone"`
	ServiceZones              []ServiceZone          `json:"service_zones,omitempty"`
	DistanceTiers             []DistanceTier         `json:"distance_tiers,omitempty"`
	OperatingPeriods          []OperatingPeriod      `json:"operating_periods,omitempty"`
	SupportedVehicleClasses   []string               `json:"supported_vehicle_classes,omitempty"`
	FallbackTriggers          []OverflowTrigger      `json:"fallback_triggers,omitempty"`
	HybridFallbackFunding     FallbackFundingSponsor `json:"hybrid_fallback_funding_sponsor"`
	HybridVendorShareBPS      int                    `json:"hybrid_vendor_share_bps"`
	HybridPlatformApprovalRef string                 `json:"hybrid_platform_approval_ref,omitempty"`
	RiderAvailability         RiderAvailability      `json:"rider_availability"`

	EstimatedMinutesBase  int `json:"estimated_minutes_base"`
	EstimatedMinutesPerKm int `json:"estimated_minutes_per_km"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int64     `json:"version"`
}

// PolicyInput is a create-or-update request body.
//
// business_id is absent by design: the tenant comes from the authenticated
// session and is never read from the request.
type PolicyInput struct {
	Enabled bool         `json:"enabled"`
	Mode    DeliveryMode `json:"delivery_mode"`
	FeeRule FeeRuleKind  `json:"fee_rule"`

	Currency string `json:"currency"`

	BaseFeeMinor  int64 `json:"base_fee_minor"`
	PerKmFeeMinor int64 `json:"per_km_fee_minor"`

	MinimumOrderMinor int64 `json:"minimum_order_minor"`

	FreeDeliveryThresholdMinor *int64 `json:"free_delivery_threshold_minor"`

	MaxDeliveryRadiusMeters *int `json:"max_delivery_radius_meters"`

	MaxPackageWeightGrams     *int64                 `json:"max_package_weight_grams"`
	MaxPackageCount           *int64                 `json:"max_package_count"`
	MaxVolumeMl               *int64                 `json:"max_volume_ml"`
	OperatingTimezone         string                 `json:"operating_timezone"`
	ServiceZones              []ServiceZone          `json:"service_zones"`
	DistanceTiers             []DistanceTier         `json:"distance_tiers"`
	OperatingPeriods          []OperatingPeriod      `json:"operating_periods"`
	SupportedVehicleClasses   []string               `json:"supported_vehicle_classes"`
	FallbackTriggers          []OverflowTrigger      `json:"fallback_triggers"`
	HybridFallbackFunding     FallbackFundingSponsor `json:"hybrid_fallback_funding_sponsor"`
	HybridVendorShareBPS      int                    `json:"hybrid_vendor_share_bps"`
	HybridPlatformApprovalRef string                 `json:"hybrid_platform_approval_ref"`
	RiderAvailability         RiderAvailability      `json:"rider_availability"`

	EstimatedMinutesBase  int `json:"estimated_minutes_base"`
	EstimatedMinutesPerKm int `json:"estimated_minutes_per_km"`

	Reason string `json:"reason"`
}

// ServiceArea is the Section 6 branch service area, the single authority for
// how far a branch reaches.
type ServiceArea struct {
	CenterLatitude  float64
	CenterLongitude float64
	RadiusMeters    int
}

// FeeBeneficiary names who the delivery fee is owed to.
//
// It is derived from the execution authority, never configured. Vendor Own
// Delivery is the only case in this platform where the answer is the vendor.
type FeeBeneficiary string

const (
	// BeneficiaryVendor is Vendor Own Delivery: the vendor performed the
	// delivery, so the delivery money lands additively on the vendor payable.
	BeneficiaryVendor FeeBeneficiary = "VENDOR"
)

// Refusal reason codes for a vendor-own delivery evaluation.
//
// Every one of these is a normal planning answer, not a fault: it means this
// group has no vendor-own option and the planner must consider platform
// logistics or report the group unserviceable.
const (
	ReasonServiceable = "SERVICEABLE"

	ReasonPlatformDisabled  = "VENDOR_OWN_DELIVERY_PLATFORM_DISABLED"
	ReasonVendorDisabled    = "VENDOR_OWN_DELIVERY_DISABLED"
	ReasonNoServiceArea     = "VENDOR_SERVICE_AREA_NOT_CONFIGURED"
	ReasonOutsideArea       = "OUTSIDE_VENDOR_SERVICE_AREA"
	ReasonBelowMinimum      = "BELOW_VENDOR_MINIMUM_ORDER"
	ReasonOverWeightLimit   = "OVER_VENDOR_WEIGHT_LIMIT"
	ReasonCurrencyMismatch  = "VENDOR_DELIVERY_CURRENCY_MISMATCH"
	ReasonInvalidCoordinate = "INVALID_DELIVERY_COORDINATE"
)

// FundingSource attributes each part of a delivery charge.
//
// A vendor free-delivery threshold is a VENDOR-FUNDED promotion: the delivery
// still costs what it costs and the vendor absorbs it. Recording the split
// here is what keeps "who actually paid for this delivery" answerable at
// settlement instead of being re-derived from business rules afterwards.
type Funding struct {
	// GrossFeeMinor is what the delivery actually costs.
	GrossFeeMinor int64 `json:"gross_fee_minor"`

	// CustomerFeeMinor is what the customer is asked to pay.
	CustomerFeeMinor int64 `json:"customer_fee_minor"`

	// VendorFundedMinor is what the vendor absorbs through its own promotion.
	VendorFundedMinor int64 `json:"vendor_funded_minor"`

	// PlatformFundedMinor is what a platform promotion absorbs. Section 17C
	// never populates it — no platform delivery promotion exists yet — but it
	// is present so the invariant below is stated in full rather than
	// rediscovered later.
	PlatformFundedMinor int64 `json:"platform_funded_minor"`
}

// Balanced states the funding invariant: every minor unit of the real cost is
// attributed to exactly one payer.
func (f Funding) Balanced() bool {
	return f.CustomerFeeMinor+f.VendorFundedMinor+f.PlatformFundedMinor ==
		f.GrossFeeMinor
}

// Decision is the vendor-own delivery answer for one vendor group.
type Decision struct {
	Serviceable bool   `json:"serviceable"`
	ReasonCode  string `json:"reason_code"`

	// FeeBeneficiary is only meaningful when Serviceable.
	FeeBeneficiary FeeBeneficiary `json:"fee_beneficiary,omitempty"`

	Currency string `json:"currency,omitempty"`

	Funding Funding `json:"funding"`

	DistanceKm       float64 `json:"distance_km"`
	EstimatedMinutes int     `json:"estimated_minutes"`

	// PolicyVersion is the exact policy revision this decision was computed
	// against, so a plan can be checked against the configuration that
	// produced it rather than against whatever the vendor has since edited.
	PolicyVersion int64 `json:"policy_version"`
}

// QuoteRequest is one vendor group presented for a vendor-own delivery
// decision.
//
// Every commercial input here is server-resolved: OrderValueMinor comes from
// the revalidated Section 17B group total, never from a request body.
type QuoteRequest struct {
	// PlatformEnabled is VENDOR_OWN_DELIVERY_ENABLED. Both it and the
	// vendor's own Enabled flag must be true.
	PlatformEnabled bool

	Policy Policy

	// Area is the Section 6 branch service area. HasArea false means the
	// branch never configured one, which is a refusal rather than an
	// unlimited reach.
	Area    ServiceArea
	HasArea bool

	// Pickup is the branch location; Dropoff is the customer's snapshotted
	// delivery address.
	PickupLatitude   float64
	PickupLongitude  float64
	DropoffLatitude  float64
	DropoffLongitude float64

	OrderValueMinor int64
	Currency        string

	TotalWeightGrams int64
}
