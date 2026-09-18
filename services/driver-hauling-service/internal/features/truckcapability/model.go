// Package truckcapability owns Hauling's record of what each truck can
// physically do, which service products it is admitted to, the local policy
// snapshots matching reads, and the evidence trail explaining why a candidate
// was or was not eligible.
//
// It is a projection-and-record feature, not a decision one: the decision
// lives in internal/cargofit so that direct bookings and Commerce missions
// share exactly one engine. This package's job is to make sure that engine is
// fed the truth rather than a guess.
package truckcapability

import (
	"strings"
	"time"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/services/hauling-service/internal/truckcatalog"
	"cosmicforge/logistics/shared/go/capabilityfit"
)

// EligibilityStatus is the lifecycle of a truck's admission to one service
// product. The vocabulary matches the canonical one already used across the
// platform's other eligibility projections.
type EligibilityStatus string

const (
	// EligibilityPending means certification has been requested and not
	// decided. It is NOT eligible: pending work must not dispatch.
	EligibilityPending EligibilityStatus = "PENDING"
	// EligibilityEligible means admitted and currently valid.
	EligibilityEligible EligibilityStatus = "ELIGIBLE"
	// EligibilitySuspended means admitted but temporarily withdrawn.
	EligibilitySuspended EligibilityStatus = "SUSPENDED"
	// EligibilityExpired means the admission window has passed.
	EligibilityExpired EligibilityStatus = "EXPIRED"
	// EligibilityRejected means certification was refused.
	EligibilityRejected EligibilityStatus = "REJECTED"
)

// Valid reports whether the status is one of the five.
func (s EligibilityStatus) Valid() bool {
	switch s {
	case EligibilityPending, EligibilityEligible, EligibilitySuspended,
		EligibilityExpired, EligibilityRejected:
		return true
	default:
		return false
	}
}

// ParseEligibilityStatus normalizes a caller-supplied status.
func ParseEligibilityStatus(raw string) (EligibilityStatus, bool) {
	status := EligibilityStatus(strings.ToUpper(strings.TrimSpace(raw)))
	return status, status.Valid()
}

// ProductEligibility is one truck's admission to one product.
type ProductEligibility struct {
	ID               string
	TruckID          string
	ProviderID       string
	ProductCode      string
	Status           EligibilityStatus
	PolicyVersion    string
	CertificationRef string
	EffectiveAt      *time.Time
	ExpiresAt        *time.Time
	ReasonCode       string
	SourceVersion    int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// UsableAt reports whether this admission actually permits work at a moment.
//
// The window is checked here rather than trusted from the status column
// because an expiry is a fact about time, and a row only becomes EXPIRED when
// the sweeper next runs. Reading the status alone would let a certification
// that lapsed an hour ago still dispatch until a background job noticed.
func (e ProductEligibility) UsableAt(now time.Time) bool {
	if e.Status != EligibilityEligible {
		return false
	}
	if e.EffectiveAt != nil && now.Before(*e.EffectiveAt) {
		return false
	}
	if e.ExpiresAt != nil && !now.Before(*e.ExpiresAt) {
		return false
	}
	return true
}

// Public is the provider-facing projection. It carries no internal row id and
// no source version: those are platform bookkeeping, not provider business.
type PublicProductEligibility struct {
	ProductCode      string     `json:"product_code"`
	Status           string     `json:"status"`
	CertificationRef string     `json:"certification_ref,omitempty"`
	EffectiveAt      *time.Time `json:"effective_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	ReasonCode       string     `json:"reason_code,omitempty"`
}

// Public projects the eligibility for a provider.
func (e ProductEligibility) Public() PublicProductEligibility {
	return PublicProductEligibility{
		ProductCode:      e.ProductCode,
		Status:           string(e.Status),
		CertificationRef: e.CertificationRef,
		EffectiveAt:      e.EffectiveAt,
		ExpiresAt:        e.ExpiresAt,
		ReasonCode:       e.ReasonCode,
	}
}

// Record is a truck's stored capability row, before the catalogue's
// type-derived facts are layered on.
//
// Every optional physical fact is a pointer. That is not stylistic: a *bool
// round-trips SQL NULL faithfully, and a plain bool would turn "never
// surveyed" into "declared false" on the first scan, which is the exact
// conflation the whole fit engine is built to avoid.
type Record struct {
	TruckID    string
	ProviderID string
	TruckType  string
	Active     bool
	Verified   bool

	MaxPayloadGrams *int64

	InternalLengthMm *int64
	InternalWidthMm  *int64
	InternalHeightMm *int64
	UsableVolumeMl   *int64

	OpeningWidthMm  *int64
	OpeningHeightMm *int64

	Enclosed         *bool
	WeatherProtected *bool
	UprightSafe      *bool
	FragileSafe      *bool

	Refrigerated     *bool
	TemperatureMinC  *int
	TemperatureMaxC  *int
	TemperatureClass string

	TailLift    *bool
	Crane       *bool
	Hydraulic   *bool
	LoadingRamp *bool

	AxleCount               *int
	TrailerCapable          *bool
	SupportedContainerSizes []string

	SupportedGoodsClasses  []string
	ProhibitedGoodsClasses []string

	InsuranceValid   *bool
	InspectionRef    string
	CertificationRef string

	CapabilityVersion       int64
	CapabilitySourceVersion int64
	ObservedAt              *time.Time
}

// ToFit converts the stored row into the fit engine's input.
//
// The *Known flags are derived from pointer presence in one place here, so no
// caller can accidentally pass a zero measurement as a known one.
func (r Record) ToFit() cargofit.TruckCapability {
	capability := cargofit.TruckCapability{
		TruckID:    r.TruckID,
		ProviderID: r.ProviderID,
		TruckType:  r.TruckType,
		BodyFamily: string(truckcatalog.FactsFor(r.TruckType).Family),
		Active:     r.Active,
		Verified:   r.Verified,

		Enclosed:         capabilityfit.TriFromPointer(r.Enclosed),
		WeatherProtected: capabilityfit.TriFromPointer(r.WeatherProtected),
		UprightSafe:      capabilityfit.TriFromPointer(r.UprightSafe),
		FragileSafe:      capabilityfit.TriFromPointer(r.FragileSafe),
		Refrigerated:     capabilityfit.TriFromPointer(r.Refrigerated),
		TemperatureMinC:  r.TemperatureMinC,
		TemperatureMaxC:  r.TemperatureMaxC,
		TemperatureClass: r.TemperatureClass,

		TailLift:    capabilityfit.TriFromPointer(r.TailLift),
		Crane:       capabilityfit.TriFromPointer(r.Crane),
		Hydraulic:   capabilityfit.TriFromPointer(r.Hydraulic),
		LoadingRamp: capabilityfit.TriFromPointer(r.LoadingRamp),

		TrailerCapable:          capabilityfit.TriFromPointer(r.TrailerCapable),
		SupportedContainerSizes: r.SupportedContainerSizes,
		SupportedGoodsClasses:   r.SupportedGoodsClasses,
		ProhibitedGoodsClasses:  r.ProhibitedGoodsClasses,

		InsuranceValid:   capabilityfit.TriFromPointer(r.InsuranceValid),
		InspectionRef:    r.InspectionRef,
		CertificationRef: r.CertificationRef,

		CapabilityVersion: r.CapabilityVersion,
		SourceVersion:     r.CapabilitySourceVersion,
		ObservedAt:        r.ObservedAt,
	}
	if r.MaxPayloadGrams != nil {
		capability.MaxPayloadGrams = *r.MaxPayloadGrams
		capability.MaxPayloadGramsKnown = true
	}
	// All three internal dimensions must be present for the body to be
	// measured. A body with a length but no width has not been surveyed, and
	// treating it as measured would let the two missing axes default to zero
	// and refuse everything with a bogus CARGO_FIT_FAILED.
	if r.InternalLengthMm != nil && r.InternalWidthMm != nil && r.InternalHeightMm != nil {
		capability.InternalLengthMm = *r.InternalLengthMm
		capability.InternalWidthMm = *r.InternalWidthMm
		capability.InternalHeightMm = *r.InternalHeightMm
		capability.InternalDimsKnown = true
	}
	if r.UsableVolumeMl != nil {
		capability.UsableVolumeMl = *r.UsableVolumeMl
		capability.UsableVolumeMlKnown = true
	}
	if r.OpeningWidthMm != nil && r.OpeningHeightMm != nil {
		capability.OpeningWidthMm = *r.OpeningWidthMm
		capability.OpeningHeightMm = *r.OpeningHeightMm
		capability.OpeningDimsKnown = true
	}
	if r.AxleCount != nil {
		capability.Axles = *r.AxleCount
	}
	return capability
}

// PublicRecord is the provider-facing capability projection.
type PublicRecord struct {
	TruckID    string `json:"truck_id"`
	TruckType  string `json:"truck_type"`
	BodyFamily string `json:"body_family"`

	MaxPayloadGrams *int64 `json:"max_payload_grams,omitempty"`

	InternalLengthMm *int64 `json:"internal_length_mm,omitempty"`
	InternalWidthMm  *int64 `json:"internal_width_mm,omitempty"`
	InternalHeightMm *int64 `json:"internal_height_mm,omitempty"`
	UsableVolumeMl   *int64 `json:"usable_volume_ml,omitempty"`

	OpeningWidthMm  *int64 `json:"opening_width_mm,omitempty"`
	OpeningHeightMm *int64 `json:"opening_height_mm,omitempty"`

	Enclosed         *bool `json:"enclosed,omitempty"`
	WeatherProtected *bool `json:"weather_protected,omitempty"`
	UprightSafe      *bool `json:"upright_safe,omitempty"`
	FragileSafe      *bool `json:"fragile_safe,omitempty"`

	Refrigerated     *bool  `json:"refrigerated,omitempty"`
	TemperatureMinC  *int   `json:"temperature_min_c,omitempty"`
	TemperatureMaxC  *int   `json:"temperature_max_c,omitempty"`
	TemperatureClass string `json:"temperature_class,omitempty"`

	TailLift    *bool `json:"tail_lift,omitempty"`
	Crane       *bool `json:"crane,omitempty"`
	Hydraulic   *bool `json:"hydraulic,omitempty"`
	LoadingRamp *bool `json:"loading_ramp,omitempty"`

	AxleCount               *int     `json:"axle_count,omitempty"`
	TrailerCapable          *bool    `json:"trailer_capable,omitempty"`
	SupportedContainerSizes []string `json:"supported_container_sizes"`

	SupportedGoodsClasses  []string `json:"supported_goods_classes"`
	ProhibitedGoodsClasses []string `json:"prohibited_goods_classes"`

	InsuranceValid *bool  `json:"insurance_valid,omitempty"`
	InspectionRef  string `json:"inspection_ref,omitempty"`

	// CapabilityVersion is echoed so a provider (or an internal caller) can
	// send it back as an expected version and have a concurrent edit refused
	// rather than silently overwritten.
	CapabilityVersion int64      `json:"capability_version"`
	ObservedAt        *time.Time `json:"observed_at,omitempty"`

	// Completeness is the share of hard-fit-relevant facts that have been
	// recorded, 0-100. It exists to answer the only question that matters
	// before enforcement is switched on: how much of the fleet would suddenly
	// stop matching.
	Completeness int `json:"completeness"`
}

// Public projects the record.
func (r Record) Public() PublicRecord {
	return PublicRecord{
		TruckID:                 r.TruckID,
		TruckType:               r.TruckType,
		BodyFamily:              string(truckcatalog.FactsFor(r.TruckType).Family),
		MaxPayloadGrams:         r.MaxPayloadGrams,
		InternalLengthMm:        r.InternalLengthMm,
		InternalWidthMm:         r.InternalWidthMm,
		InternalHeightMm:        r.InternalHeightMm,
		UsableVolumeMl:          r.UsableVolumeMl,
		OpeningWidthMm:          r.OpeningWidthMm,
		OpeningHeightMm:         r.OpeningHeightMm,
		Enclosed:                r.Enclosed,
		WeatherProtected:        r.WeatherProtected,
		UprightSafe:             r.UprightSafe,
		FragileSafe:             r.FragileSafe,
		Refrigerated:            r.Refrigerated,
		TemperatureMinC:         r.TemperatureMinC,
		TemperatureMaxC:         r.TemperatureMaxC,
		TemperatureClass:        r.TemperatureClass,
		TailLift:                r.TailLift,
		Crane:                   r.Crane,
		Hydraulic:               r.Hydraulic,
		LoadingRamp:             r.LoadingRamp,
		AxleCount:               r.AxleCount,
		TrailerCapable:          r.TrailerCapable,
		SupportedContainerSizes: nonNil(r.SupportedContainerSizes),
		SupportedGoodsClasses:   nonNil(r.SupportedGoodsClasses),
		ProhibitedGoodsClasses:  nonNil(r.ProhibitedGoodsClasses),
		InsuranceValid:          r.InsuranceValid,
		InspectionRef:           r.InspectionRef,
		CapabilityVersion:       r.CapabilityVersion,
		ObservedAt:              r.ObservedAt,
		Completeness:            r.Completeness(),
	}
}

// Completeness is the share of hard-fit-relevant facts recorded, 0-100.
//
// The facts counted are exactly those the fit engine can refuse on. It is the
// rollout metric: enforcement is safe to switch on when the fleet's
// completeness is high enough that fail-closed refusals are rare, and the only
// honest way to know that is to measure it before flipping the flag.
func (r Record) Completeness() int {
	facts := []bool{
		r.MaxPayloadGrams != nil,
		r.InternalLengthMm != nil && r.InternalWidthMm != nil && r.InternalHeightMm != nil,
		r.UsableVolumeMl != nil,
		r.OpeningWidthMm != nil && r.OpeningHeightMm != nil,
		r.Enclosed != nil,
		r.WeatherProtected != nil,
		r.Refrigerated != nil,
		r.UprightSafe != nil,
		r.FragileSafe != nil,
		r.TailLift != nil,
		r.Crane != nil,
		r.LoadingRamp != nil,
	}
	recorded := 0
	for _, present := range facts {
		if present {
			recorded++
		}
	}
	return recorded * 100 / len(facts)
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// UpdateInput is a provider-declared capability edit.
//
// Every field is a pointer so an absent field means "leave as it was" and an
// explicitly null one means "I no longer know". Without that distinction a
// partial update would blank every fact the form did not happen to render,
// which is how a survey gets silently destroyed by an unrelated edit.
type UpdateInput struct {
	TruckID    string
	ProviderID string

	// ExpectedVersion, when non-zero, makes the write conditional. A mismatch
	// is a conflict rather than a last-writer-wins overwrite.
	ExpectedVersion int64

	MaxPayloadGrams  Nullable[int64]
	InternalLengthMm Nullable[int64]
	InternalWidthMm  Nullable[int64]
	InternalHeightMm Nullable[int64]
	UsableVolumeMl   Nullable[int64]
	OpeningWidthMm   Nullable[int64]
	OpeningHeightMm  Nullable[int64]

	Enclosed         Nullable[bool]
	WeatherProtected Nullable[bool]
	UprightSafe      Nullable[bool]
	FragileSafe      Nullable[bool]

	Refrigerated     Nullable[bool]
	TemperatureMinC  Nullable[int]
	TemperatureMaxC  Nullable[int]
	TemperatureClass *string

	TailLift    Nullable[bool]
	Crane       Nullable[bool]
	Hydraulic   Nullable[bool]
	LoadingRamp Nullable[bool]

	AxleCount               Nullable[int]
	TrailerCapable          Nullable[bool]
	SupportedContainerSizes *[]string

	SupportedGoodsClasses  *[]string
	ProhibitedGoodsClasses *[]string

	InspectionRef *string
}

// Nullable distinguishes the three states a JSON field can arrive in: absent,
// explicitly null, and present with a value.
type Nullable[T any] struct {
	Set   bool
	Value *T
}

// Absent builds the "leave unchanged" state.
func Absent[T any]() Nullable[T] { return Nullable[T]{} }

// Cleared builds the "I no longer know this" state.
func Cleared[T any]() Nullable[T] { return Nullable[T]{Set: true} }

// Known builds a stated value.
func Known[T any](value T) Nullable[T] { return Nullable[T]{Set: true, Value: &value} }

// MatchDecision is one candidate's hard-fit outcome, recorded for operations.
type MatchDecision struct {
	BookingID         string
	MissionRef        string
	ProviderID        string
	TruckID           string
	Decision          string
	ReasonCodes       string
	Evidence          map[string]string
	DistanceM         int64
	CapabilityVersion int64
	PolicyVersion     string
	Shadow            bool
}

// Decision outcomes.
const (
	DecisionEligible = "ELIGIBLE"
	DecisionRejected = "REJECTED"
	DecisionSelected = "SELECTED"
)

// PolicySnapshot is a local immutable copy of a network or product policy.
type PolicySnapshot struct {
	PolicyKind    string
	PolicyVersion string
	Payload       []byte
	Checksum      string
	EffectiveAt   time.Time
	AppliedAt     time.Time
}

// Policy kinds Hauling stores.
const (
	// PolicyKindCapability governs which capability facts are hard
	// requirements for which products.
	PolicyKindCapability = "HAULING_CAPABILITY"
	// PolicyKindProduct governs the product catalogue itself.
	PolicyKindProduct = "HAULING_PRODUCT"
)

// DefaultCapabilityPolicyVersion is the version matching pins when no snapshot
// has been pulled yet.
//
// A built-in default matters: without one, a deployment that has never reached
// the admin service would record an empty policy version on every decision,
// and evidence with no policy version cannot be reasoned about later.
const DefaultCapabilityPolicyVersion = "hauling_capability_v1"
