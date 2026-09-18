// Package bikecapability owns Dispatch's record of what each bike can
// physically carry, which service products it is admitted to, the local policy
// snapshots assignment reads, and the evidence explaining why a candidate was
// or was not eligible.
//
// The decision itself lives in internal/parcelfit so the same engine serves
// assignment, the supply preview and serviceability. This package's job is to
// make sure that engine is fed the truth rather than a bike_type string.
package bikecapability

import (
	"strings"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/parcelfit"
	"cosmicforge/logistics/shared/go/capabilityfit"
)

// EligibilityStatus is a bike's admission lifecycle for one product, using the
// canonical vocabulary the platform's other eligibility projections already
// share.
type EligibilityStatus string

const (
	// EligibilityPending means requested and not decided. It is NOT eligible:
	// pending work must not dispatch.
	EligibilityPending EligibilityStatus = "PENDING"
	// EligibilityEligible means admitted and currently valid.
	EligibilityEligible EligibilityStatus = "ELIGIBLE"
	// EligibilitySuspended means admitted but temporarily withdrawn.
	EligibilitySuspended EligibilityStatus = "SUSPENDED"
	// EligibilityExpired means the window has closed.
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

// ProductEligibility is one bike's admission to one product.
type ProductEligibility struct {
	BikeID           string
	ProviderID       string
	ProductCode      string
	Status           EligibilityStatus
	PolicyVersion    string
	CertificationRef string
	EffectiveAt      *time.Time
	ExpiresAt        *time.Time
	ReasonCode       string
	SourceVersion    int64
	UpdatedAt        time.Time
}

// UsableAt reports whether this admission actually permits work right now.
//
// The window is evaluated here rather than trusted from the status column,
// because an expiry is a fact about time and a row only becomes EXPIRED when
// the sweeper next runs. Reading the status alone would let a certification
// that lapsed an hour ago keep dispatching until a background job noticed.
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

// PublicProductEligibility is the provider-facing projection. It carries no
// source version: that is platform bookkeeping, not provider business.
type PublicProductEligibility struct {
	ProductCode      string     `json:"product_code"`
	Status           string     `json:"status"`
	CertificationRef string     `json:"certification_ref,omitempty"`
	EffectiveAt      *time.Time `json:"effective_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	ReasonCode       string     `json:"reason_code,omitempty"`
}

// Public projects the eligibility.
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

// Record is a bike's stored capability row.
//
// Every optional fact is a pointer so SQL NULL round-trips faithfully. A plain
// bool would turn "never surveyed" into "declared false" on the first scan,
// which is exactly the conflation the fit engine is built to avoid.
type Record struct {
	BikeID     string
	ProviderID string
	BikeType   string
	Active     bool
	Verified   bool

	MaxPayloadGrams *int64

	BoxLengthMm *int64
	BoxWidthMm  *int64
	BoxHeightMm *int64

	EnclosedBox      *bool
	WeatherProtected *bool
	UprightSafe      *bool
	FragileSafe      *bool

	ThermalBag       *bool
	TemperatureClass string

	CapabilityVersion int64
	SourceVersion     int64
	ObservedAt        *time.Time

	// Surveyed is false when the bike has a registration row but no capability
	// row at all. It is what lets a caller report CAPABILITY_UNKNOWN rather
	// than a spurious physical refusal.
	Surveyed bool
}

// ToFit converts the stored row into the fit engine's input.
//
// The *Known flags are derived from pointer presence in one place here, so no
// caller can accidentally pass a zero measurement as a known one.
func (r Record) ToFit() parcelfit.Capability {
	capability := parcelfit.Capability{
		BikeID:            r.BikeID,
		ProviderID:        r.ProviderID,
		BikeType:          r.BikeType,
		Active:            r.Active,
		Verified:          r.Verified,
		EnclosedBox:       capabilityfit.TriFromPointer(r.EnclosedBox),
		WeatherProtected:  capabilityfit.TriFromPointer(r.WeatherProtected),
		UprightSafe:       capabilityfit.TriFromPointer(r.UprightSafe),
		FragileSafe:       capabilityfit.TriFromPointer(r.FragileSafe),
		ThermalBag:        capabilityfit.TriFromPointer(r.ThermalBag),
		TemperatureClass:  r.TemperatureClass,
		CapabilityVersion: r.CapabilityVersion,
		SourceVersion:     r.SourceVersion,
		ObservedAt:        r.ObservedAt,
	}
	if r.MaxPayloadGrams != nil {
		capability.MaxPayloadGrams = *r.MaxPayloadGrams
		capability.MaxPayloadGramsKnown = true
	}
	// All three box dimensions are needed for the box to count as measured. A
	// box with a length and no width has not been surveyed, and treating it as
	// measured would let the missing axes default to zero and refuse
	// everything with a spurious fit failure.
	if r.BoxLengthMm != nil && r.BoxWidthMm != nil && r.BoxHeightMm != nil {
		capability.BoxLengthMm = *r.BoxLengthMm
		capability.BoxWidthMm = *r.BoxWidthMm
		capability.BoxHeightMm = *r.BoxHeightMm
		capability.BoxDimsKnown = true
	}
	return capability
}

// PublicRecord is the provider-facing capability projection.
type PublicRecord struct {
	BikeID   string `json:"bike_id"`
	BikeType string `json:"bike_type"`

	MaxPayloadGrams *int64 `json:"max_payload_grams,omitempty"`

	BoxLengthMm *int64 `json:"box_length_mm,omitempty"`
	BoxWidthMm  *int64 `json:"box_width_mm,omitempty"`
	BoxHeightMm *int64 `json:"box_height_mm,omitempty"`

	EnclosedBox      *bool `json:"enclosed_box,omitempty"`
	WeatherProtected *bool `json:"weather_protected,omitempty"`
	UprightSafe      *bool `json:"upright_safe,omitempty"`
	FragileSafe      *bool `json:"fragile_safe,omitempty"`

	ThermalBag       *bool  `json:"thermal_bag,omitempty"`
	TemperatureClass string `json:"temperature_class,omitempty"`

	// CapabilityVersion is echoed so a caller can send it back as an expected
	// version and have a concurrent edit refused rather than overwritten.
	CapabilityVersion int64      `json:"capability_version"`
	ObservedAt        *time.Time `json:"observed_at,omitempty"`

	// Completeness is the share of hard-fit-relevant facts recorded, 0-100.
	// It answers the only question that matters before enforcement is switched
	// on: how much of the fleet would suddenly stop matching.
	Completeness int `json:"completeness"`
}

// Public projects the record.
func (r Record) Public() PublicRecord {
	return PublicRecord{
		BikeID:            r.BikeID,
		BikeType:          r.BikeType,
		MaxPayloadGrams:   r.MaxPayloadGrams,
		BoxLengthMm:       r.BoxLengthMm,
		BoxWidthMm:        r.BoxWidthMm,
		BoxHeightMm:       r.BoxHeightMm,
		EnclosedBox:       r.EnclosedBox,
		WeatherProtected:  r.WeatherProtected,
		UprightSafe:       r.UprightSafe,
		FragileSafe:       r.FragileSafe,
		ThermalBag:        r.ThermalBag,
		TemperatureClass:  r.TemperatureClass,
		CapabilityVersion: r.CapabilityVersion,
		ObservedAt:        r.ObservedAt,
		Completeness:      r.Completeness(),
	}
}

// Completeness is the share of hard-fit-relevant facts recorded, 0-100.
//
// The facts counted are exactly those the fit engine can refuse on, because
// this is the rollout metric: enforcement is safe when the fleet's
// completeness is high enough that fail-closed refusals are rare, and the only
// honest way to know that is to measure it before flipping the flag.
func (r Record) Completeness() int {
	facts := []bool{
		r.MaxPayloadGrams != nil,
		r.BoxLengthMm != nil && r.BoxWidthMm != nil && r.BoxHeightMm != nil,
		r.EnclosedBox != nil,
		r.WeatherProtected != nil,
		r.UprightSafe != nil,
		r.FragileSafe != nil,
		r.ThermalBag != nil,
	}
	recorded := 0
	for _, present := range facts {
		if present {
			recorded++
		}
	}
	return recorded * 100 / len(facts)
}

// Nullable distinguishes the three states a JSON field arrives in: absent,
// explicitly null, and present with a value.
//
// Without that distinction a partial update would blank every fact the client
// did not send, destroying a survey as a side effect of changing one field.
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

// UpdateInput is a provider-declared capability edit.
type UpdateInput struct {
	BikeID     string
	ProviderID string

	// ExpectedVersion, when non-zero, makes the write conditional. A mismatch
	// is a conflict rather than last-writer-wins.
	ExpectedVersion int64

	MaxPayloadGrams Nullable[int64]
	BoxLengthMm     Nullable[int64]
	BoxWidthMm      Nullable[int64]
	BoxHeightMm     Nullable[int64]

	EnclosedBox      Nullable[bool]
	WeatherProtected Nullable[bool]
	UprightSafe      Nullable[bool]
	FragileSafe      Nullable[bool]

	ThermalBag       Nullable[bool]
	TemperatureClass *string
}

// MatchDecision is one candidate's hard-fit outcome, recorded for operations.
type MatchDecision struct {
	MissionID         string
	BookingID         string
	ProviderID        string
	BikeID            string
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

// Policy kinds Dispatch stores locally.
const (
	// PolicyKindCapability governs which capability facts are hard
	// requirements for which products.
	PolicyKindCapability = "DISPATCH_CAPABILITY"
	// PolicyKindProduct governs the product catalogue itself.
	PolicyKindProduct = "DISPATCH_PRODUCT"
)

// DefaultCapabilityPolicyVersion is what matching pins when no snapshot has
// been pulled.
//
// A built-in default matters: without one, a deployment that has never reached
// the admin service would record an empty policy version on every decision,
// and evidence with no policy version cannot be reasoned about later.
const DefaultCapabilityPolicyVersion = "dispatch_capability_v1"

// Product codes Dispatch serves.
const (
	// ProductStandardDelivery is ordinary parcel delivery, requiring no
	// certification by default.
	ProductStandardDelivery = "DISPATCH_STANDARD"
	// ProductColdChain is temperature-controlled delivery, which a deployment
	// will normally add to DISPATCH_CERTIFICATION_REQUIRED_PRODUCTS.
	ProductColdChain = "DISPATCH_COLD_CHAIN"
)

// PolicySnapshot is a local immutable copy of a network or product policy.
type PolicySnapshot struct {
	PolicyKind    string
	PolicyVersion string
	Payload       []byte
	Checksum      string
	EffectiveAt   time.Time
}
