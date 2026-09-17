// Package capability is Taxi's pure, dependency-free eligibility engine. It
// decides whether a provider/vehicle MAY serve a passenger booking or a
// Taxi Goods mission, before any ranking or scoring happens. It holds no
// pricing and no persistence: fares belong to the pricing boundary, and
// facts are loaded by the capabilities feature.
package capability

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/commercequote"
)

// Stable, machine-readable Taxi reason codes. Where the shared platform
// vocabulary already has an equivalent, the shared spelling is used.
const (
	ReasonEligible                     = "ELIGIBLE"
	ReasonProductNotEligible           = "PRODUCT_NOT_ELIGIBLE"
	ReasonVehicleNotCertified          = "VEHICLE_NOT_CERTIFIED"
	ReasonPassengerCapacityExceeded    = "PASSENGER_CAPACITY_EXCEEDED"
	ReasonAccessibilityUnsupported     = "ACCESSIBILITY_UNSUPPORTED"
	ReasonCargoWeightExceeded          = "CARGO_WEIGHT_EXCEEDED"
	ReasonCargoVolumeExceeded          = "CARGO_VOLUME_EXCEEDED"
	ReasonCargoDimensionExceeded       = "CARGO_DIMENSION_EXCEEDED"
	ReasonCargoPackageCountExceeded    = "CARGO_PACKAGE_COUNT_EXCEEDED"
	ReasonFragileHandlingUnsupported   = "FRAGILE_HANDLING_UNSUPPORTED"
	ReasonUprightHandlingUnsupported   = "UPRIGHT_HANDLING_UNSUPPORTED"
	ReasonTemperatureUnsupported       = "TEMPERATURE_CONTROL_UNSUPPORTED"
	ReasonWeatherProtectionUnsupported = "WEATHER_PROTECTION_UNSUPPORTED"
	ReasonSecureCargoUnsupported       = "SECURE_CARGO_UNSUPPORTED"
	ReasonCargoSeparationUnsupported   = "CARGO_SEPARATION_UNSUPPORTED"
	ReasonMultiPickupUnsupported       = "MULTI_PICKUP_UNSUPPORTED" // shared with Vendor fulfilment vocabulary
	ReasonPickupCountExceeded          = "PICKUP_COUNT_EXCEEDED"
	ReasonRouteConstraintFailed        = "ROUTE_CONSTRAINT_FAILED"
	ReasonProviderNotCompliant         = "PROVIDER_NOT_COMPLIANT"
	ReasonProviderNotOperational       = "PROVIDER_NOT_OPERATIONAL"
	ReasonVehicleNotOperational        = "VEHICLE_NOT_OPERATIONAL"
	ReasonCapabilityUnknown            = "CAPABILITY_UNKNOWN"
	ReasonNoEligibleProvider           = commercequote.ReasonNoProviderAvailable
)

// CoarseReason maps a granular Taxi reason to the shared commerce quote
// vocabulary for callers that only understand the coarse codes.
func CoarseReason(reason string) string {
	switch reason {
	case ReasonEligible:
		return commercequote.ReasonServiceable
	case ReasonCargoWeightExceeded, ReasonCargoVolumeExceeded, ReasonCargoDimensionExceeded, ReasonCargoPackageCountExceeded:
		return commercequote.ReasonLoadExceedsCapacity
	case ReasonFragileHandlingUnsupported, ReasonUprightHandlingUnsupported, ReasonTemperatureUnsupported,
		ReasonWeatherProtectionUnsupported, ReasonSecureCargoUnsupported, ReasonCargoSeparationUnsupported:
		return commercequote.ReasonHandlingUnsupported
	case ReasonRouteConstraintFailed:
		return commercequote.ReasonInvalidRoute
	}
	return commercequote.ReasonNoProviderAvailable
}

const (
	PurposePassenger = "PASSENGER"
	PurposeGoods     = "GOODS"

	EligibilityPending   = "PENDING"
	EligibilityEligible  = "ELIGIBLE"
	EligibilitySuspended = "SUSPENDED"
	EligibilityExpired   = "EXPIRED"
	EligibilityRejected  = "REJECTED"

	PolicyKindServiceProducts = "SERVICE_PRODUCTS"
)

// Product is one published Taxi service product inside a policy snapshot.
// No monetary field exists here by design.
type Product struct {
	Code                   string   `json:"code"`
	Version                int64    `json:"version"`
	Purpose                string   `json:"purpose"`
	Label                  string   `json:"label"`
	Active                 bool     `json:"active"`
	Markets                []string `json:"markets,omitempty"`
	MinPassengerCapacity   int      `json:"min_passenger_capacity,omitempty"`
	MaxPassengers          int      `json:"max_passengers,omitempty"`
	Features               []string `json:"features,omitempty"`
	MaxPickups             int      `json:"max_pickups,omitempty"`
	MaxDropoffs            int      `json:"max_dropoffs,omitempty"`
	SecurePickupHandover   bool     `json:"secure_pickup_handover,omitempty"`
	SecureDeliveryHandover bool     `json:"secure_delivery_handover,omitempty"`
	LegacyVehicleClass     string   `json:"legacy_vehicle_class,omitempty"`
	// GuaranteeProfileVersion names the service guarantee profile a booking
	// is committed under. Empty means the product declares none.
	GuaranteeProfileVersion string `json:"guarantee_profile_version,omitempty"`
}

// Policy is an immutable service-product policy snapshot. Committed bookings
// keep the policy version they were created under.
type Policy struct {
	Version                     string    `json:"policy_version"`
	EffectiveAt                 time.Time `json:"effective_at"`
	Products                    []Product `json:"products"`
	CertificationRequiredFields []string  `json:"certification_required_fields"`
}

var ErrInvalidPolicy = errors.New("taxi service product policy is invalid")

// ParsePolicy validates a policy document. It fails closed on anything
// ambiguous, because a bad policy must never widen eligibility.
func ParsePolicy(raw []byte) (Policy, string, error) {
	var p Policy
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return Policy{}, "", fmt.Errorf("%w: %v", ErrInvalidPolicy, err)
	}
	if strings.TrimSpace(p.Version) == "" || p.EffectiveAt.IsZero() || len(p.Products) == 0 {
		return Policy{}, "", ErrInvalidPolicy
	}
	seen := map[string]bool{}
	for i := range p.Products {
		product := &p.Products[i]
		product.Code, product.Purpose = strings.ToUpper(strings.TrimSpace(product.Code)), strings.ToUpper(strings.TrimSpace(product.Purpose))
		if product.Code == "" || seen[product.Code] || product.Version < 1 {
			return Policy{}, "", ErrInvalidPolicy
		}
		seen[product.Code] = true
		switch product.Purpose {
		case PurposePassenger:
			if product.MaxPassengers < 1 || product.MinPassengerCapacity < 0 || product.MinPassengerCapacity > product.MaxPassengers {
				return Policy{}, "", ErrInvalidPolicy
			}
		case PurposeGoods:
			if product.MaxPickups < 1 || product.MaxDropoffs != 1 {
				return Policy{}, "", ErrInvalidPolicy
			}
		default:
			return Policy{}, "", ErrInvalidPolicy
		}
	}
	canonical, _ := json.Marshal(p)
	sum := sha256.Sum256(canonical)
	return p, hex.EncodeToString(sum[:]), nil
}

func (p Policy) Product(code string) (Product, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, product := range p.Products {
		if product.Code == code {
			return product, true
		}
	}
	return Product{}, false
}

// ActiveProducts lists active products for a purpose and optional market.
func (p Policy) ActiveProducts(purpose, market string) []Product {
	var out []Product
	for _, product := range p.Products {
		if !product.Active || (purpose != "" && product.Purpose != purpose) {
			continue
		}
		if market != "" && len(product.Markets) > 0 && !contains(product.Markets, market) {
			continue
		}
		out = append(out, product)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// MaxPickups returns the largest pickup count any active goods product in
// the market allows. Zero means no active goods product exists.
func (p Policy) MaxPickups(market string) int {
	max := 0
	for _, product := range p.ActiveProducts(PurposeGoods, market) {
		if product.MaxPickups > max {
			max = product.MaxPickups
		}
	}
	return max
}

// CertificationSensitive reports whether a provider edit to field must send
// eligibility back to PENDING review.
func (p Policy) CertificationSensitive(field string) bool {
	return contains(p.CertificationRequiredFields, field)
}

// VehicleCapability is the normalized physical capability of one vehicle.
// Nil pointers mean the value was never stated or verified: unknown, never
// "false" or "zero".
type VehicleCapability struct {
	VehicleID                string     `json:"vehicle_id"`
	TotalSeats               *int       `json:"total_seats"`
	PassengerCapacity        *int       `json:"passenger_capacity"`
	BodyStyle                string     `json:"body_style,omitempty"`
	DoorCount                *int       `json:"door_count"`
	ACStatus                 string     `json:"ac_status,omitempty"`
	LuggageClass             string     `json:"luggage_class,omitempty"`
	LuggageVolumeML          *int64     `json:"luggage_volume_ml"`
	WheelchairAccessible     *bool      `json:"wheelchair_accessible"`
	ChildSeatCapable         *bool      `json:"child_seat_capable"`
	PetEligible              *bool      `json:"pet_eligible"`
	GoodsMaxWeightGrams      *int64     `json:"goods_max_weight_grams"`
	GoodsMaxVolumeML         *int64     `json:"goods_max_volume_ml"`
	GoodsMaxLengthMM         *int64     `json:"goods_max_length_mm"`
	GoodsMaxWidthMM          *int64     `json:"goods_max_width_mm"`
	GoodsMaxHeightMM         *int64     `json:"goods_max_height_mm"`
	GoodsMaxPackageCount     *int64     `json:"goods_max_package_count"`
	UprightSafe              *bool      `json:"upright_safe"`
	FragileSafe              *bool      `json:"fragile_safe"`
	WeatherProtected         *bool      `json:"weather_protected"`
	TemperatureControlled    *bool      `json:"temperature_controlled"`
	TemperatureMinC          *float64   `json:"temperature_min_c"`
	TemperatureMaxC          *float64   `json:"temperature_max_c"`
	SecureCargoArea          *bool      `json:"secure_cargo_area"`
	CargoSeparationSupported *bool      `json:"cargo_separation_supported"`
	ObservedAt               *time.Time `json:"observed_at"`
	VerifiedAt               *time.Time `json:"verified_at"`
	Version                  int64      `json:"version"`
}

// Eligibility is one vehicle/product eligibility row.
type Eligibility struct {
	VehicleID        string     `json:"vehicle_id"`
	ProductCode      string     `json:"product_code"`
	Status           string     `json:"status"`
	PolicyVersion    string     `json:"policy_version"`
	CertificationRef string     `json:"certification_ref,omitempty"`
	EffectiveAt      *time.Time `json:"effective_at"`
	ExpiresAt        *time.Time `json:"expires_at"`
	ReasonCode       string     `json:"reason_code,omitempty"`
	Version          int64      `json:"version"`
}

// Active reports whether the eligibility grants service at now. Anything
// other than an unexpired ELIGIBLE row fails closed.
func (e Eligibility) Active(now time.Time) bool {
	if e.Status != EligibilityEligible {
		return false
	}
	if e.EffectiveAt != nil && now.Before(*e.EffectiveAt) {
		return false
	}
	return e.ExpiresAt == nil || now.Before(*e.ExpiresAt)
}

// Candidate is everything the hard gates need about one provider+vehicle.
type Candidate struct {
	ProviderID          string
	VehicleID           string
	ProviderOperational bool
	ProviderCompliant   bool
	VehicleOperational  bool
	Capability          *VehicleCapability
	Eligibility         map[string]Eligibility
}

// PassengerRequirement is a booking's frozen hard requirement snapshot.
type PassengerRequirement struct {
	ProductCode        string `json:"product_code"`
	PassengerCount     int    `json:"passenger_count"`
	WheelchairRequired bool   `json:"wheelchair_required"`
	ChildSeatRequired  bool   `json:"child_seat_required"`
	PetTravel          bool   `json:"pet_travel"`
}

// Decision is the outcome of the hard gates for one candidate.
type Decision struct {
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason_code"`
}

func reject(reason string) Decision { return Decision{Reason: reason} }

// commonGates enforces the fixed pre-scoring order shared by every purpose.
func commonGates(c Candidate, productCode string, now time.Time) (Decision, bool) {
	switch {
	case !c.ProviderOperational:
		return reject(ReasonProviderNotOperational), false
	case !c.ProviderCompliant:
		return reject(ReasonProviderNotCompliant), false
	case !c.VehicleOperational:
		return reject(ReasonVehicleNotOperational), false
	}
	eligibility, ok := c.Eligibility[strings.ToUpper(productCode)]
	if !ok {
		return reject(ReasonProductNotEligible), false
	}
	if !eligibility.Active(now) {
		if eligibility.Status == EligibilityPending {
			return reject(ReasonVehicleNotCertified), false
		}
		return reject(ReasonProductNotEligible), false
	}
	if c.Capability == nil {
		return reject(ReasonCapabilityUnknown), false
	}
	return Decision{}, true
}

// EvaluatePassenger runs the passenger hard gates in the mandated order:
// provider operational -> compliance -> vehicle operational -> product
// eligibility -> passenger capacity -> hard customer requirements. Unknown
// capability always fails closed.
func EvaluatePassenger(c Candidate, requirement PassengerRequirement, product Product, now time.Time) Decision {
	if decision, ok := commonGates(c, requirement.ProductCode, now); !ok {
		return decision
	}
	capacity := c.Capability.PassengerCapacity
	if capacity == nil {
		return reject(ReasonCapabilityUnknown)
	}
	if requirement.PassengerCount < 1 || requirement.PassengerCount > *capacity || (product.MaxPassengers > 0 && requirement.PassengerCount > product.MaxPassengers) {
		return reject(ReasonPassengerCapacityExceeded)
	}
	if product.MinPassengerCapacity > 0 && *capacity < product.MinPassengerCapacity {
		return reject(ReasonProductNotEligible)
	}
	if requirement.WheelchairRequired && !isTrue(c.Capability.WheelchairAccessible) {
		return reject(unknownOr(c.Capability.WheelchairAccessible, ReasonAccessibilityUnsupported))
	}
	if requirement.ChildSeatRequired && !isTrue(c.Capability.ChildSeatCapable) {
		return reject(unknownOr(c.Capability.ChildSeatCapable, ReasonAccessibilityUnsupported))
	}
	if requirement.PetTravel && !isTrue(c.Capability.PetEligible) {
		return reject(unknownOr(c.Capability.PetEligible, ReasonAccessibilityUnsupported))
	}
	return Decision{Eligible: true, Reason: ReasonEligible}
}

// Temperature classes for Taxi Goods cargo.
const (
	TemperatureAmbient = "AMBIENT"
	TemperatureHot     = "HOT"
	TemperatureChilled = "CHILLED"
	TemperatureFrozen  = "FROZEN"
)

// StopCargo is the cargo collected at one pickup stop.
type StopCargo struct {
	StopRef                   string   `json:"stop_ref"`
	WeightGrams               int64    `json:"weight_grams"`
	VolumeML                  int64    `json:"volume_ml"`
	MaxLengthMM               int64    `json:"max_length_mm"`
	MaxWidthMM                int64    `json:"max_width_mm"`
	MaxHeightMM               int64    `json:"max_height_mm"`
	PackageCount              int64    `json:"package_count"`
	Fragile                   bool     `json:"fragile"`
	UprightRequired           bool     `json:"upright_required"`
	WeatherProtectionRequired bool     `json:"weather_protection_required"`
	TemperatureClass          string   `json:"temperature_class,omitempty"`
	TemperatureMinC           *float64 `json:"temperature_min_c,omitempty"`
	TemperatureMaxC           *float64 `json:"temperature_max_c,omitempty"`
	SecureCargoRequired       bool     `json:"secure_cargo_required"`
	// MixedTemperatureClasses lists further controlled classes collected at
	// the same stop, so cumulative fit sees the need for cargo separation.
	MixedTemperatureClasses []string `json:"mixed_temperature_classes,omitempty"`
}

func (s StopCargo) Validate() error {
	if strings.TrimSpace(s.StopRef) == "" || s.WeightGrams < 0 || s.VolumeML < 0 || s.MaxLengthMM < 0 || s.MaxWidthMM < 0 || s.MaxHeightMM < 0 || s.PackageCount < 1 {
		return ErrInvalidCargo
	}
	switch s.temperatureClass() {
	case TemperatureAmbient, TemperatureHot, TemperatureChilled, TemperatureFrozen:
	default:
		return ErrInvalidCargo
	}
	if s.TemperatureMinC != nil && s.TemperatureMaxC != nil && *s.TemperatureMinC > *s.TemperatureMaxC {
		return ErrInvalidCargo
	}
	for _, class := range s.MixedTemperatureClasses {
		switch strings.ToUpper(strings.TrimSpace(class)) {
		case TemperatureHot, TemperatureChilled, TemperatureFrozen:
		default:
			return ErrInvalidCargo
		}
	}
	return nil
}

func (s StopCargo) temperatureClass() string {
	class := strings.ToUpper(strings.TrimSpace(s.TemperatureClass))
	if class == "" {
		return TemperatureAmbient
	}
	return class
}

var ErrInvalidCargo = errors.New("taxi goods cargo requirement is invalid")

// Aggregate is the cumulative load a vehicle carries after its last pickup.
// With a single final dropoff, peak load is the sum of every stop.
type Aggregate struct {
	WeightGrams         int64    `json:"weight_grams"`
	VolumeML            int64    `json:"volume_ml"`
	PackageCount        int64    `json:"package_count"`
	LongestSideMM       int64    `json:"longest_side_mm"`
	MiddleSideMM        int64    `json:"middle_side_mm"`
	ShortestSideMM      int64    `json:"shortest_side_mm"`
	Fragile             bool     `json:"fragile"`
	Upright             bool     `json:"upright"`
	WeatherProtection   bool     `json:"weather_protection"`
	SecureCargo         bool     `json:"secure_cargo"`
	TemperatureClasses  []string `json:"temperature_classes"`
	TemperatureMinC     *float64 `json:"temperature_min_c,omitempty"`
	TemperatureMaxC     *float64 `json:"temperature_max_c,omitempty"`
	RequiresSeparation  bool     `json:"requires_separation"`
	TemperatureConflict bool     `json:"temperature_conflict"`
}

// Accumulate sums stop cargo into the peak aggregate load.
func Accumulate(stops []StopCargo) (Aggregate, error) {
	var a Aggregate
	classes := map[string]bool{}
	for _, stop := range stops {
		if err := stop.Validate(); err != nil {
			return Aggregate{}, err
		}
		a.WeightGrams += stop.WeightGrams
		a.VolumeML += stop.VolumeML
		a.PackageCount += stop.PackageCount
		dims := sortedDims(stop.MaxLengthMM, stop.MaxWidthMM, stop.MaxHeightMM)
		a.LongestSideMM, a.MiddleSideMM, a.ShortestSideMM = max64(a.LongestSideMM, dims[0]), max64(a.MiddleSideMM, dims[1]), max64(a.ShortestSideMM, dims[2])
		a.Fragile = a.Fragile || stop.Fragile
		a.Upright = a.Upright || stop.UprightRequired
		a.WeatherProtection = a.WeatherProtection || stop.WeatherProtectionRequired
		a.SecureCargo = a.SecureCargo || stop.SecureCargoRequired
		class := stop.temperatureClass()
		classes[class] = true
		for _, mixed := range stop.MixedTemperatureClasses {
			classes[strings.ToUpper(strings.TrimSpace(mixed))] = true
		}
		if class != TemperatureAmbient {
			if stop.TemperatureMinC != nil && (a.TemperatureMinC == nil || *stop.TemperatureMinC > *a.TemperatureMinC) {
				v := *stop.TemperatureMinC
				a.TemperatureMinC = &v
			}
			if stop.TemperatureMaxC != nil && (a.TemperatureMaxC == nil || *stop.TemperatureMaxC < *a.TemperatureMaxC) {
				v := *stop.TemperatureMaxC
				a.TemperatureMaxC = &v
			}
		}
	}
	for class := range classes {
		a.TemperatureClasses = append(a.TemperatureClasses, class)
	}
	sort.Strings(a.TemperatureClasses)
	controlled := 0
	for class := range classes {
		if class != TemperatureAmbient {
			controlled++
		}
	}
	// Two different controlled classes (for example hot food with frozen
	// goods) need physical separation inside one vehicle.
	a.RequiresSeparation = controlled > 1
	a.TemperatureConflict = a.TemperatureMinC != nil && a.TemperatureMaxC != nil && *a.TemperatureMinC > *a.TemperatureMaxC
	return a, nil
}

// GoodsMission is a proposed or committed Taxi Goods mission.
type GoodsMission struct {
	ProductCode string      `json:"product_code"`
	Stops       []StopCargo `json:"pickups"`
	Dropoffs    int         `json:"dropoffs"`
}

// EvaluateGoodsPolicy checks what depends only on the product policy and the
// mission shape, never on a specific vehicle. It is safe for previews.
func EvaluateGoodsPolicy(mission GoodsMission, product Product, multiPickupEnabled bool) Decision {
	if product.Purpose != PurposeGoods || !product.Active {
		return reject(ReasonProductNotEligible)
	}
	if len(mission.Stops) > 1 && !multiPickupEnabled {
		return reject(ReasonMultiPickupUnsupported)
	}
	if len(mission.Stops) < 1 || mission.Dropoffs != 1 {
		return reject(ReasonRouteConstraintFailed)
	}
	if len(mission.Stops) > product.MaxPickups {
		return reject(ReasonPickupCountExceeded)
	}
	return Decision{Eligible: true, Reason: ReasonEligible}
}

// EvaluateGoods runs the Taxi Goods hard gates. Cargo fit is always judged on
// the cumulative load across every pickup, never stop by stop.
func EvaluateGoods(c Candidate, mission GoodsMission, product Product, multiPickupEnabled bool, now time.Time) Decision {
	if decision, ok := commonGates(c, mission.ProductCode, now); !ok {
		return decision
	}
	if decision := EvaluateGoodsPolicy(mission, product, multiPickupEnabled); !decision.Eligible {
		return decision
	}
	aggregate, err := Accumulate(mission.Stops)
	if err != nil {
		return reject(ReasonRouteConstraintFailed)
	}
	return FitAggregate(*c.Capability, aggregate)
}

// FitAggregate compares cumulative cargo with a vehicle. Required limits
// that are unknown fail closed with CAPABILITY_UNKNOWN.
func FitAggregate(v VehicleCapability, a Aggregate) Decision {
	limit := func(value *int64, need int64, reason string) (Decision, bool) {
		if need <= 0 {
			return Decision{}, true
		}
		if value == nil {
			return reject(ReasonCapabilityUnknown), false
		}
		if need > *value {
			return reject(reason), false
		}
		return Decision{}, true
	}
	if d, ok := limit(v.GoodsMaxWeightGrams, a.WeightGrams, ReasonCargoWeightExceeded); !ok {
		return d
	}
	if d, ok := limit(v.GoodsMaxVolumeML, a.VolumeML, ReasonCargoVolumeExceeded); !ok {
		return d
	}
	if v.GoodsMaxPackageCount != nil && a.PackageCount > *v.GoodsMaxPackageCount {
		return reject(ReasonCargoPackageCountExceeded)
	}
	if a.LongestSideMM > 0 {
		if v.GoodsMaxLengthMM == nil || v.GoodsMaxWidthMM == nil || v.GoodsMaxHeightMM == nil {
			return reject(ReasonCapabilityUnknown)
		}
		space := sortedDims(*v.GoodsMaxLengthMM, *v.GoodsMaxWidthMM, *v.GoodsMaxHeightMM)
		if a.LongestSideMM > space[0] || a.MiddleSideMM > space[1] || a.ShortestSideMM > space[2] {
			return reject(ReasonCargoDimensionExceeded)
		}
	}
	flag := func(required bool, value *bool, reason string) (Decision, bool) {
		if !required {
			return Decision{}, true
		}
		if value == nil {
			return reject(ReasonCapabilityUnknown), false
		}
		if !*value {
			return reject(reason), false
		}
		return Decision{}, true
	}
	if d, ok := flag(a.Fragile, v.FragileSafe, ReasonFragileHandlingUnsupported); !ok {
		return d
	}
	if d, ok := flag(a.Upright, v.UprightSafe, ReasonUprightHandlingUnsupported); !ok {
		return d
	}
	if d, ok := flag(a.WeatherProtection, v.WeatherProtected, ReasonWeatherProtectionUnsupported); !ok {
		return d
	}
	if d, ok := flag(a.SecureCargo, v.SecureCargoArea, ReasonSecureCargoUnsupported); !ok {
		return d
	}
	if a.TemperatureConflict {
		return reject(ReasonTemperatureUnsupported)
	}
	controlled := false
	for _, class := range a.TemperatureClasses {
		if class != TemperatureAmbient {
			controlled = true
		}
	}
	if controlled {
		if d, ok := flag(true, v.TemperatureControlled, ReasonTemperatureUnsupported); !ok {
			return d
		}
		if a.TemperatureMinC != nil && (v.TemperatureMinC == nil || *v.TemperatureMinC > *a.TemperatureMinC) {
			return reject(ReasonTemperatureUnsupported)
		}
		if a.TemperatureMaxC != nil && (v.TemperatureMaxC == nil || *v.TemperatureMaxC < *a.TemperatureMaxC) {
			return reject(ReasonTemperatureUnsupported)
		}
	}
	if d, ok := flag(a.RequiresSeparation, v.CargoSeparationSupported, ReasonCargoSeparationUnsupported); !ok {
		return d
	}
	return Decision{Eligible: true, Reason: ReasonEligible}
}

func sortedDims(values ...int64) [3]int64 {
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })
	return [3]int64{sorted[0], sorted[1], sorted[2]}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func isTrue(value *bool) bool { return value != nil && *value }

func unknownOr(value *bool, reason string) string {
	if value == nil {
		return ReasonCapabilityUnknown
	}
	return reason
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

// LegacyProductForClass maps the legacy vehicle_class to the product it may
// eventually be certified for. It never grants eligibility by itself.
func LegacyProductForClass(class string) string {
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "economy":
		return "TAXI_STANDARD"
	case "comfort":
		return "TAXI_COMFORT"
	case "xl":
		return "TAXI_XL"
	}
	return ""
}
