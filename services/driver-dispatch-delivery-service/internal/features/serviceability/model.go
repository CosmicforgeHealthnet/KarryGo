package serviceability

import (
	"fmt"
	"strings"
	"time"
)

type Decision string

const (
	DecisionServiceable               Decision = "SERVICEABLE"
	DecisionServiceableWithConditions Decision = "SERVICEABLE_WITH_CONDITIONS"
	DecisionOutsideServiceArea        Decision = "OUTSIDE_SERVICE_AREA"
	DecisionOperationalReview         Decision = "OPERATIONAL_REVIEW"
)

const (
	ReasonPickupOutsideZone    = "PICKUP_OUTSIDE_ZONE"
	ReasonDropoffOutsideZone   = "DROPOFF_OUTSIDE_ZONE"
	ReasonRouteUnavailable     = "ROUTE_UNAVAILABLE"
	ReasonRouteTooLong         = "ROUTE_TOO_LONG"
	ReasonVehicleUnsupported   = "VEHICLE_UNSUPPORTED"
	ReasonRestrictedArea       = "RESTRICTED_AREA"
	ReasonPackageReview        = "PACKAGE_REQUIRES_REVIEW"
	ReasonPackageRefused       = "PACKAGE_REFUSED"
	ReasonDegradedRouting      = "DEGRADED_ROUTING"
	ReasonPolicyReviewRequired = "POLICY_REVIEW_REQUIRED"
	ReasonServiceable          = "SERVICEABLE"
)

type AdmissionResult string

const (
	AdmissionAccepted               AdmissionResult = "ACCEPTED"
	AdmissionAcceptedWithConditions AdmissionResult = "ACCEPTED_WITH_CONDITIONS"
	AdmissionRequiresRepack         AdmissionResult = "REQUIRES_REPACK"
	AdmissionRequiresReprice        AdmissionResult = "REQUIRES_REPRICE"
	AdmissionRefusedProhibited      AdmissionResult = "REFUSED_PROHIBITED"
	AdmissionRefusedUnsafe          AdmissionResult = "REFUSED_UNSAFE"
	AdmissionOperationalReview      AdmissionResult = "OPERATIONAL_REVIEW"
)

type Point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (p Point) Validate() error {
	if p.Latitude < -90 || p.Latitude > 90 || p.Longitude < -180 || p.Longitude > 180 || (p.Latitude == 0 && p.Longitude == 0) {
		return fmt.Errorf("invalid coordinates")
	}
	return nil
}

type PackageDeclaration struct {
	PackageType             string `json:"package_type"`
	SizeCategory            string `json:"size_category"`
	CategoryCode            string `json:"category_code"`
	DeclaredWeightGrams     int64  `json:"declared_weight_grams"`
	MeasuredWeightGrams     *int64 `json:"measured_weight_grams,omitempty"`
	LengthMM                int64  `json:"length_mm,omitempty"`
	WidthMM                 int64  `json:"width_mm,omitempty"`
	HeightMM                int64  `json:"height_mm,omitempty"`
	ItemCount               int64  `json:"item_count"`
	Fragile                 bool   `json:"fragile,omitempty"`
	PackagingCondition      string `json:"packaging_condition,omitempty"`
	DeclaredValueKobo       int64  `json:"declared_value_kobo,omitempty"`
	HighValueClassification string `json:"high_value_classification,omitempty"`
	InsuranceRequested      bool   `json:"insurance_requested,omitempty"`
	InsuranceEligible       *bool  `json:"insurance_eligible,omitempty"`
	InsuranceStatus         string `json:"insurance_status,omitempty"`
	SpecialHandling         string `json:"special_handling,omitempty"`
	Medical                 bool   `json:"medical,omitempty"`
	Perishable              bool   `json:"perishable,omitempty"`
	TemperatureSensitive    bool   `json:"temperature_sensitive,omitempty"`
	Restricted              bool   `json:"restricted,omitempty"`
	Prohibited              bool   `json:"prohibited,omitempty"`
	HazardClass             string `json:"hazard_class,omitempty"`
	Description             string `json:"description,omitempty"`
}

func (p PackageDeclaration) Validate() error {
	if strings.TrimSpace(p.PackageType) == "" || strings.TrimSpace(p.SizeCategory) == "" || p.DeclaredWeightGrams <= 0 || p.ItemCount <= 0 {
		return fmt.Errorf("package type, size, positive weight and item count are required")
	}
	if p.DeclaredWeightGrams > 25_000 || p.LengthMM < 0 || p.WidthMM < 0 || p.HeightMM < 0 || p.DeclaredValueKobo < 0 {
		return fmt.Errorf("package declaration is outside supported bounds")
	}
	if p.MeasuredWeightGrams != nil && *p.MeasuredWeightGrams <= 0 {
		return fmt.Errorf("measured weight must be positive")
	}
	return nil
}

type PackageAdmission struct {
	Result        AdmissionResult `json:"result"`
	ReasonCodes   []string        `json:"reason_codes"`
	Conditions    []string        `json:"conditions,omitempty"`
	PolicyVersion string          `json:"policy_version"`
	MismatchClass string          `json:"mismatch_class,omitempty"`
	EvaluatedAt   time.Time       `json:"evaluated_at"`
}

type Request struct {
	SourceRef     string             `json:"source_ref,omitempty"`
	SourceVersion int64              `json:"source_version,omitempty"`
	Pickup        Point              `json:"pickup"`
	Dropoff       Point              `json:"dropoff"`
	VehicleClass  string             `json:"vehicle_class"`
	ServiceTier   string             `json:"service_tier"`
	Package       PackageDeclaration `json:"package"`
}

type RouteResult struct {
	DistanceMeters  int64
	DurationSeconds int64
	Polyline        string
	Source          string
	EvidenceVersion string
	Confidence      string
	Degraded        bool
}

type Quote struct {
	ID                 string             `json:"serviceability_id"`
	CustomerID         string             `json:"-"`
	IdempotencyKey     string             `json:"-"`
	RequestFingerprint string             `json:"-"`
	SourceType         string             `json:"source_type"`
	SourceRef          string             `json:"source_ref,omitempty"`
	SourceVersion      int64              `json:"source_version"`
	Pickup             Point              `json:"pickup"`
	Dropoff            Point              `json:"dropoff"`
	VehicleClass       string             `json:"vehicle_class"`
	Decision           Decision           `json:"decision"`
	ReasonCodes        []string           `json:"reason_codes"`
	PolicyVersion      string             `json:"policy_version"`
	Route              RouteResult        `json:"route"`
	Package            PackageDeclaration `json:"package"`
	Admission          PackageAdmission   `json:"package_admission"`
	PricingInputs      map[string]any     `json:"pricing_inputs"`
	FareEstimateKobo   int64              `json:"fare_estimate_kobo"`
	Currency           string             `json:"currency,omitempty"`
	ExpiresAt          time.Time          `json:"expires_at"`
	CreatedAt          time.Time          `json:"created_at"`
	Version            int64              `json:"version"`
}

func (q Quote) Expired(now time.Time) bool { return !now.Before(q.ExpiresAt) }
