package fulfilment

// The delivery requirement.
//
// One canonical, typed statement of what a load actually needs, assembled
// from the physical evidence the platform genuinely holds. It is the sole
// input to vehicle selection, which is why it is a struct and not a bag of
// loose arguments: a selector that can be called with a weight and no
// provenance is a selector that will eventually be called that way.
//
// Three rules govern this file.
//
//  1. ESTIMATED AND CONFIRMED ARE BOTH KEPT. Packing does not overwrite the
//     catalogue estimate; it is recorded beside it. A later question about
//     why a plan escalated needs both numbers, not the surviving one.
//
//  2. UNKNOWN IS NEVER ZERO. Absent evidence is reported as absent and
//     answered by a configured policy, never by a total that happens to be
//     small enough for a motorcycle.
//
//  3. ARITHMETIC CANNOT WRAP. Every total is accumulated through checked
//     integer arithmetic. A load whose measurements overflow is refused as
//     implausible rather than silently becoming a negative weight, because a
//     negative weight fits on anything.

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// MeasurementState distinguishes what the vendor expects to hand over from
// what the vendor actually packed.
type MeasurementState string

const (
	// MeasurementEstimated is pre-packing evidence: catalogue profiles and
	// vendor declarations. It is what checkout plans against.
	MeasurementEstimated MeasurementState = "ESTIMATED"

	// MeasurementConfirmed is post-packing evidence measured off the real
	// parcels.
	MeasurementConfirmed MeasurementState = "CONFIRMED"
)

// Valid reports whether the measurement state is known.
func (m MeasurementState) Valid() bool {
	return m == MeasurementEstimated || m == MeasurementConfirmed
}

// MeasurementSource is the provenance of a measurement.
//
// It exists for support and audit: "why was this a van" is answerable only if
// the answer can say whether the number came from a catalogue row, a vendor
// declaration, a packing scale, or the platform's own derivation.
type MeasurementSource string

const (
	// SourceCatalogEstimate came from item or variant logistics profiles.
	SourceCatalogEstimate MeasurementSource = "CATALOG_ESTIMATE"

	// SourceVendorDeclared came from something the vendor stated directly,
	// such as a vehicle hint or a large-load flag.
	SourceVendorDeclared MeasurementSource = "VENDOR_DECLARED"

	// SourcePackingConfirmed came from the packed parcels themselves.
	SourcePackingConfirmed MeasurementSource = "PACKING_CONFIRMED"

	// SourceSystemCalculated was derived by the platform from other stated
	// values, such as a bounding volume computed from stated dimensions.
	SourceSystemCalculated MeasurementSource = "SYSTEM_CALCULATED"
)

// Valid reports whether the source is known.
func (m MeasurementSource) Valid() bool {
	switch m {
	case SourceCatalogEstimate, SourceVendorDeclared,
		SourcePackingConfirmed, SourceSystemCalculated:
		return true
	default:
		return false
	}
}

// RiskClass is the load's value and risk classification.
type RiskClass string

const (
	RiskStandard  RiskClass = "STANDARD"
	RiskHighValue RiskClass = "HIGH_VALUE"
)

// Valid reports whether the risk class is known. Empty is allowed and means
// the classification was never stated.
func (r RiskClass) Valid() bool {
	return r == "" || r == RiskStandard || r == RiskHighValue
}

// SpecialHandling is a controlled special-handling classification.
//
// The vocabulary is closed on purpose. An open string set would let a
// catalogue row nobody reviewed decide which vehicle carries a load, which is
// the opposite of a deterministic engine. Anything outside this set is
// recorded as unrecognized evidence and answered by the missing-metadata
// policy: never silently ignored, and never able to select a vehicle.
type SpecialHandling string

const (
	// SpecialHandlingOdorSensitive must not share a compartment with strong
	// odours. Shop declares it as ODOR_SENSITIVE.
	SpecialHandlingOdorSensitive SpecialHandling = "ODOR_SENSITIVE"

	// SpecialHandlingRawProtein is raw meat or fish, which Shop separates
	// from ready-to-eat goods.
	SpecialHandlingRawProtein SpecialHandling = "RAW_PROTEIN"

	// SpecialHandlingKeepDry must not be exposed to rain in transit.
	SpecialHandlingKeepDry SpecialHandling = "KEEP_DRY"

	// SpecialHandlingHighValue is the handling consequence of a high-value
	// classification, kept separate from RiskClass so a vehicle capability
	// can state competence for it explicitly.
	SpecialHandlingHighValue SpecialHandling = "HIGH_VALUE"
)

// KnownSpecialHandling reports whether a classification is in the controlled
// vocabulary, returning the canonical value when it is.
func KnownSpecialHandling(value string) (SpecialHandling, bool) {
	switch SpecialHandling(strings.ToUpper(strings.TrimSpace(value))) {
	case SpecialHandlingOdorSensitive:
		return SpecialHandlingOdorSensitive, true
	case SpecialHandlingRawProtein:
		return SpecialHandlingRawProtein, true
	case SpecialHandlingKeepDry:
		return SpecialHandlingKeepDry, true
	case SpecialHandlingHighValue:
		return SpecialHandlingHighValue, true
	default:
		return "", false
	}
}

// Errors returned when a requirement cannot be built at all.
var (
	// ErrRequirementInvalid means the input is not a physically meaningful
	// statement: a negative weight, an inverted ready window, an unknown
	// enum. It is a fault in the caller, not an unserviceable load.
	ErrRequirementInvalid = errors.New(
		"fulfilment: invalid delivery requirement",
	)

	// ErrRequirementOverflow means the measurements are large enough that
	// accumulating them would wrap a 64-bit integer. Refused rather than
	// wrapped, because a wrapped weight is a negative weight and a negative
	// weight fits on anything.
	ErrRequirementOverflow = errors.New(
		"fulfilment: delivery requirement overflows",
	)
)

// Sanity ceilings. Nothing this platform delivers approaches them; they exist
// so a corrupted or hostile measurement is refused at the door instead of
// propagating into arithmetic that then has to defend itself everywhere.
const (
	// MaxPlausibleWeightGrams is one thousand tonnes.
	MaxPlausibleWeightGrams int64 = 1_000_000_000

	// MaxPlausibleVolumeMl is one thousand cubic metres.
	MaxPlausibleVolumeMl int64 = 1_000_000_000

	// MaxPlausibleDimensionMm is one hundred metres.
	MaxPlausibleDimensionMm int = 100_000

	// MaxPlausiblePackageCount bounds one delivery obligation.
	MaxPlausiblePackageCount int64 = 100_000

	// MaxPlausibleDistanceMeters is five thousand kilometres.
	MaxPlausibleDistanceMeters int64 = 5_000_000

	// MaxPlausibleValueMinor bounds a declared value in minor units.
	MaxPlausibleValueMinor int64 = 1_000_000_000_000
)

// addChecked adds two non-negative values, reporting overflow rather than
// wrapping.
func addChecked(a, b int64) (int64, bool) {
	if a < 0 || b < 0 {
		return 0, false
	}

	sum := a + b
	if sum < a {
		return 0, false
	}

	return sum, true
}

// mulChecked multiplies two non-negative values, reporting overflow.
func mulChecked(a, b int64) (int64, bool) {
	if a < 0 || b < 0 {
		return 0, false
	}

	if a == 0 || b == 0 {
		return 0, true
	}

	product := a * b
	if product/b != a {
		return 0, false
	}

	return product, true
}

// PhysicalMeasurement is one coherent set of measurements at one point in the
// order's life.
//
// Every field is stated in a canonical integer unit - grams, millilitres,
// millimetres - because a threshold compared in floating point is a threshold
// that behaves differently on either side of a rounding error.
type PhysicalMeasurement struct {
	State  MeasurementState  `json:"state"`
	Source MeasurementSource `json:"source"`

	PackageCount int64 `json:"package_count"`

	TotalWeightGrams int64    `json:"total_weight_grams"`
	WeightEvidence   Evidence `json:"weight_evidence"`

	TotalVolumeMl  int64    `json:"total_volume_ml"`
	VolumeEvidence Evidence `json:"volume_evidence"`

	// LongestDimensionMm is the longest single edge of any ONE package. It is
	// deliberately not a sum: stacking cannot make a long item shorter, so
	// this is what answers whether it will physically go in.
	LongestDimensionMm int      `json:"longest_dimension_mm"`
	DimensionEvidence  Evidence `json:"dimension_evidence"`

	// MaxSinglePackageWeightGrams is the heaviest individual package. A
	// carrier bounded by what one person can lift is bounded by this, not by
	// the total.
	MaxSinglePackageWeightGrams int64 `json:"max_single_package_weight_grams,omitempty"`

	// Stackable is false when at least one package may not be stacked, which
	// derates the usable volume of every vehicle. See
	// CapabilityPolicy.NonStackableVolumeFactorBP.
	Stackable bool `json:"stackable"`

	LargeLoad bool `json:"large_load,omitempty"`
}

// Measured reports whether the measurement carries any physical evidence at
// all. A measurement with none is the case the missing-metadata policy exists
// for.
func (m PhysicalMeasurement) Measured() bool {
	return m.TotalWeightGrams > 0 ||
		m.TotalVolumeMl > 0 ||
		m.LongestDimensionMm > 0
}

// WeaklyEvidenced reports whether any dimension of the measurement is known
// to be missing rather than merely unstated.
func (m PhysicalMeasurement) WeaklyEvidenced() bool {
	return m.WeightEvidence == EvidenceAbsent ||
		m.VolumeEvidence == EvidenceAbsent ||
		m.DimensionEvidence == EvidenceAbsent
}

// Validate reports the first reason a measurement is not physically
// meaningful.
func (m PhysicalMeasurement) Validate() error {
	switch {
	case m.State != "" && !m.State.Valid():
		return ErrRequirementInvalid
	case m.Source != "" && !m.Source.Valid():
		return ErrRequirementInvalid
	case m.PackageCount < 0 || m.PackageCount > MaxPlausiblePackageCount:
		return ErrRequirementInvalid
	case m.TotalWeightGrams < 0 || m.TotalWeightGrams > MaxPlausibleWeightGrams:
		return ErrRequirementInvalid
	case m.TotalVolumeMl < 0 || m.TotalVolumeMl > MaxPlausibleVolumeMl:
		return ErrRequirementInvalid
	case m.LongestDimensionMm < 0 ||
		m.LongestDimensionMm > MaxPlausibleDimensionMm:
		return ErrRequirementInvalid
	case m.MaxSinglePackageWeightGrams < 0 ||
		m.MaxSinglePackageWeightGrams > MaxPlausibleWeightGrams:
		return ErrRequirementInvalid
	case m.TotalWeightGrams > 0 &&
		m.MaxSinglePackageWeightGrams > m.TotalWeightGrams:
		// One package heavier than the whole load is not a measurement, it is
		// a mistake.
		return ErrRequirementInvalid
	}

	return nil
}

// HandlingProfile is what the carrier must actually be competent to do.
type HandlingProfile struct {
	Capabilities []HandlingCapability `json:"capabilities,omitempty"`

	SpecialHandling []SpecialHandling `json:"special_handling,omitempty"`

	// UnrecognizedHandling records classifications the platform could not
	// interpret. They never select a vehicle; they make the requirement
	// incompletely evidenced, which the configured policy then answers.
	UnrecognizedHandling []string `json:"unrecognized_handling,omitempty"`

	// SeparationGroups are vendor declarations that items must not travel
	// together. More than one in a single load is a conflict.
	SeparationGroups []string `json:"separation_groups,omitempty"`
}

// Has reports whether a handling capability is required.
func (h HandlingProfile) Has(want HandlingCapability) bool {
	for _, capability := range h.Capabilities {
		if capability == want {
			return true
		}
	}

	return false
}

// HasSpecial reports whether a special-handling classification applies.
func (h HandlingProfile) HasSpecial(want SpecialHandling) bool {
	for _, handling := range h.SpecialHandling {
		if handling == want {
			return true
		}
	}

	return false
}

// TemperatureConflict reports incompatible temperature requirements inside
// ONE physical load.
//
// The conflicting pairs are stated explicitly rather than collapsed into a
// generic temperature_required flag, because the whole point is that hot and
// frozen goods in one compartment is a different problem from either on its
// own. INSULATED is deliberately not in conflict with HOT_HOLD: an insulated
// box carrying a hot meal and a cold drink is what a motorcycle courier does
// today, and calling that a conflict would refuse ordinary work.
func (h HandlingProfile) TemperatureConflict() bool {
	hot := h.Has(CapabilityHotHold)
	chilled := h.Has(CapabilityColdChain)
	frozen := h.Has(CapabilityFrozenChain)

	switch {
	case hot && chilled:
		return true
	case hot && frozen:
		return true
	case chilled && frozen:
		// Two actively controlled bands cannot both be held in one
		// compartment: a frozen chain would freeze the chilled goods.
		return true
	default:
		return false
	}
}

// SeparationConflict reports that the load carries items the vendor declared
// must not travel together.
func (h HandlingProfile) SeparationConflict() bool {
	return len(h.SeparationGroups) > 1
}

// ValueProfile is the load's declared value and risk classification.
//
// It is RISK METADATA ONLY. Nothing here creates, moves or reconciles money:
// payment-wallet-service remains the sole financial authority, and a declared
// value never becomes a ledger entry, a fee or an insurance charge.
type ValueProfile struct {
	DeclaredValueMinor int64  `json:"declared_value_minor,omitempty"`
	Currency           string `json:"currency,omitempty"`

	RiskClass RiskClass `json:"risk_class,omitempty"`
}

// HighValue reports whether the load is classified high value.
func (v ValueProfile) HighValue() bool { return v.RiskClass == RiskHighValue }

// Validate reports the first reason a value profile is not usable.
func (v ValueProfile) Validate() error {
	switch {
	case !v.RiskClass.Valid():
		return ErrRequirementInvalid
	case v.DeclaredValueMinor < 0 ||
		v.DeclaredValueMinor > MaxPlausibleValueMinor:
		return ErrRequirementInvalid
	case v.Currency != "" && len(v.Currency) != 3:
		return ErrRequirementInvalid
	case v.DeclaredValueMinor > 0 && v.Currency == "":
		// A value without a currency cannot be compared to anything, and
		// mixing two currencies into one total is how a risk band silently
		// becomes meaningless.
		return ErrRequirementInvalid
	}

	return nil
}

// TimingProfile is when the load can be collected and when it must arrive.
//
// EstimatedTransitMinutes is an INPUT, not something this package computes.
// The logistics authorities already answer route time in their own quotes and
// the delivery policy already holds vendor estimates; deriving a second
// number here would create a rival ETA nobody agreed to.
type TimingProfile struct {
	ReadyFrom  *time.Time `json:"ready_from,omitempty"`
	ReadyUntil *time.Time `json:"ready_until,omitempty"`

	PromisedBy *time.Time `json:"promised_by,omitempty"`

	// LoadingDurationSeconds is optional vendor/operations evidence for the
	// time required to hand the load to the carrier. It can affect scheduling
	// and authority ETA, never pricing chosen by Vendor Service.
	LoadingDurationSeconds int64 `json:"loading_duration_seconds,omitempty"`

	EstimatedTransitMinutes int `json:"estimated_transit_minutes,omitempty"`
}

// Validate reports the first reason a timing profile is not meaningful.
func (t TimingProfile) Validate() error {
	if t.EstimatedTransitMinutes < 0 {
		return ErrRequirementInvalid
	}
	if t.LoadingDurationSeconds < 0 ||
		t.LoadingDurationSeconds > 8*60*60 {
		return ErrRequirementInvalid
	}

	if t.ReadyFrom != nil && t.ReadyUntil != nil &&
		t.ReadyUntil.Before(*t.ReadyFrom) {
		return ErrRequirementInvalid
	}

	if t.PromisedBy != nil && t.ReadyFrom != nil &&
		t.PromisedBy.Before(*t.ReadyFrom) {
		// A delivery promised before the goods can even be collected is not a
		// tight SLA, it is an invalid one.
		return ErrRequirementInvalid
	}

	return nil
}

// SLASatisfiable reports whether the promise can still be met, and whether
// the question was answerable at all.
//
// Both timestamps are compared in UTC against an explicit now, so the answer
// never depends on the server's local zone.
func (t TimingProfile) SLASatisfiable(now time.Time) (ok bool, known bool) {
	if t.PromisedBy == nil || t.EstimatedTransitMinutes <= 0 {
		return true, false
	}

	departure := now.UTC()

	if t.ReadyFrom != nil && t.ReadyFrom.UTC().After(departure) {
		// Transit cannot start before the vendor is ready.
		departure = t.ReadyFrom.UTC()
	}

	arrival := departure.Add(
		time.Duration(t.EstimatedTransitMinutes) * time.Minute,
	)

	return !arrival.After(t.PromisedBy.UTC()), true
}

// RouteProfile is the shape of the journey.
type RouteProfile struct {
	DistanceMeters int64 `json:"distance_meters,omitempty"`

	// PickupCount is how many distinct collection points the mission needs.
	// More than one requires a runtime that can actually drive it; see
	// ServiceabilitySnapshot.MultiPickup.
	PickupCount int `json:"pickup_count"`
}

// Validate reports the first reason a route profile is not meaningful.
func (r RouteProfile) Validate() error {
	switch {
	case r.DistanceMeters < 0 || r.DistanceMeters > MaxPlausibleDistanceMeters:
		return ErrRequirementInvalid
	case r.PickupCount < 0:
		return ErrRequirementInvalid
	}

	return nil
}

// DeliveryRequirement is the complete, validated statement of what a load
// needs. It is the sole input to vehicle selection.
type DeliveryRequirement struct {
	BusinessID string `json:"business_id,omitempty"`
	BranchID   string `json:"branch_id,omitempty"`

	Estimated PhysicalMeasurement `json:"estimated"`

	// Confirmed is nil until the load has actually been packed. It never
	// replaces Estimated; both are kept so an escalation can be explained.
	Confirmed *PhysicalMeasurement `json:"confirmed,omitempty"`

	Handling HandlingProfile `json:"handling"`
	Value    ValueProfile    `json:"value"`
	Timing   TimingProfile   `json:"timing"`
	Route    RouteProfile    `json:"route"`

	Urgency Urgency `json:"urgency,omitempty"`

	// VehicleHints is the vendor's own suitability statement. It can only
	// ever raise the class, never lower it.
	VehicleHints []string `json:"vehicle_hints,omitempty"`
}

// Authoritative returns the measurement selection must actually be made
// against.
//
// Confirmed wins when it exists, because a packed parcel on a scale outranks
// a catalogue row. The estimate is not discarded: it stays on the
// requirement, and the snapshot records which one was used.
func (d DeliveryRequirement) Authoritative() PhysicalMeasurement {
	if d.Confirmed != nil {
		return *d.Confirmed
	}

	return d.Estimated
}

// PackingConfirmed reports whether selection is being made against packed
// evidence rather than a catalogue estimate.
func (d DeliveryRequirement) PackingConfirmed() bool {
	return d.Confirmed != nil
}

// Validate reports the first reason the requirement is not usable.
func (d DeliveryRequirement) Validate() error {
	if err := d.Estimated.Validate(); err != nil {
		return err
	}

	if d.Confirmed != nil {
		if err := d.Confirmed.Validate(); err != nil {
			return err
		}
	}

	if err := d.Value.Validate(); err != nil {
		return err
	}

	if err := d.Timing.Validate(); err != nil {
		return err
	}

	if err := d.Route.Validate(); err != nil {
		return err
	}

	for _, capability := range d.Handling.Capabilities {
		if !knownHandlingCapability(capability) {
			return ErrRequirementInvalid
		}
	}

	for _, handling := range d.Handling.SpecialHandling {
		if _, ok := KnownSpecialHandling(string(handling)); !ok {
			return ErrRequirementInvalid
		}
	}

	return nil
}

func knownHandlingCapability(value HandlingCapability) bool {
	switch value {
	case CapabilityHotHold, CapabilityInsulated, CapabilityColdChain,
		CapabilityFrozenChain, CapabilityUprightCarry, CapabilityFragileCarry,
		CapabilityLeakContainment:
		return true
	default:
		return false
	}
}

// PackedParcel is one physically packed box presented as confirmed evidence.
//
// It mirrors what the fulfilment group's parcel actually records, so the
// adapter that feeds this is a field copy rather than a translation that can
// drift away from the stored row.
type PackedParcel struct {
	ParcelID string

	WeightGrams int64

	LengthMm int64
	WidthMm  int64
	HeightMm int64

	Fragile            bool
	TemperatureControl string

	Stackable bool

	SpecialHandlingRefs []string
}

// ConfirmFromPacking builds the CONFIRMED measurement and the handling
// profile implied by a set of packed parcels.
//
// The estimate is untouched: this returns a new measurement to be attached
// beside it. Volume is derived from the stated bounding box and marked
// SYSTEM_CALCULATED, because a bounding box is a derivation and calling it a
// measured volume would overstate the evidence.
func ConfirmFromPacking(
	parcels []PackedParcel,
) (PhysicalMeasurement, HandlingProfile, error) {
	measurement := PhysicalMeasurement{
		State:     MeasurementConfirmed,
		Source:    SourcePackingConfirmed,
		Stackable: true,
	}

	profile := HandlingProfile{}

	if len(parcels) == 0 {
		return measurement, profile, nil
	}

	var (
		weighed          int
		dimensioned      int
		volumed          int
		capabilitiesSeen = map[HandlingCapability]bool{}
		specialSeen      = map[SpecialHandling]bool{}
		unrecognized     []string
		ok               bool
	)

	for _, parcel := range parcels {
		if parcel.WeightGrams < 0 || parcel.LengthMm < 0 ||
			parcel.WidthMm < 0 || parcel.HeightMm < 0 {
			return PhysicalMeasurement{}, HandlingProfile{},
				ErrRequirementInvalid
		}

		if measurement.PackageCount, ok = addChecked(
			measurement.PackageCount, 1,
		); !ok {
			return PhysicalMeasurement{}, HandlingProfile{},
				ErrRequirementOverflow
		}

		if parcel.WeightGrams > 0 {
			weighed++

			if measurement.TotalWeightGrams, ok = addChecked(
				measurement.TotalWeightGrams, parcel.WeightGrams,
			); !ok {
				return PhysicalMeasurement{}, HandlingProfile{},
					ErrRequirementOverflow
			}

			if parcel.WeightGrams > measurement.MaxSinglePackageWeightGrams {
				measurement.MaxSinglePackageWeightGrams = parcel.WeightGrams
			}
		}

		longest := parcel.LengthMm

		for _, edge := range []int64{parcel.WidthMm, parcel.HeightMm} {
			if edge > longest {
				longest = edge
			}
		}

		if longest > 0 {
			dimensioned++

			if longest > int64(MaxPlausibleDimensionMm) {
				return PhysicalMeasurement{}, HandlingProfile{},
					ErrRequirementInvalid
			}

			if int(longest) > measurement.LongestDimensionMm {
				measurement.LongestDimensionMm = int(longest)
			}
		}

		// A bounding box in cubic millimetres is a thousand times a
		// millilitre. Both multiplications are checked because three stated
		// edges are three caller-supplied numbers.
		if parcel.LengthMm > 0 && parcel.WidthMm > 0 && parcel.HeightMm > 0 {
			area, areaOK := mulChecked(parcel.LengthMm, parcel.WidthMm)
			if !areaOK {
				return PhysicalMeasurement{}, HandlingProfile{},
					ErrRequirementOverflow
			}

			cubicMm, cubicOK := mulChecked(area, parcel.HeightMm)
			if !cubicOK {
				return PhysicalMeasurement{}, HandlingProfile{},
					ErrRequirementOverflow
			}

			volumed++

			if measurement.TotalVolumeMl, ok = addChecked(
				measurement.TotalVolumeMl, cubicMm/1000,
			); !ok {
				return PhysicalMeasurement{}, HandlingProfile{},
					ErrRequirementOverflow
			}
		}

		if !parcel.Stackable {
			measurement.Stackable = false
		}

		if parcel.Fragile {
			capabilitiesSeen[CapabilityFragileCarry] = true
		}

		switch strings.ToUpper(strings.TrimSpace(parcel.TemperatureControl)) {
		case "HOT":
			capabilitiesSeen[CapabilityHotHold] = true
		case "CHILLED":
			capabilitiesSeen[CapabilityColdChain] = true
		case "FROZEN":
			capabilitiesSeen[CapabilityFrozenChain] = true
		}

		for _, ref := range parcel.SpecialHandlingRefs {
			if handling, known := KnownSpecialHandling(ref); known {
				specialSeen[handling] = true

				continue
			}

			if trimmed := strings.TrimSpace(ref); trimmed != "" {
				unrecognized = appendUnique(unrecognized, trimmed)
			}
		}
	}

	total := len(parcels)
	measurement.WeightEvidence = evidenceFor(weighed, total)
	measurement.DimensionEvidence = evidenceFor(dimensioned, total)
	measurement.VolumeEvidence = evidenceFor(volumed, total)

	if volumed > 0 {
		// Every millilitre here came from a bounding box, never from a stated
		// volume, so the whole total is a derivation.
		measurement.Source = SourceSystemCalculated
	}

	for capability := range capabilitiesSeen {
		profile.Capabilities = append(profile.Capabilities, capability)
	}

	for handling := range specialSeen {
		profile.SpecialHandling = append(profile.SpecialHandling, handling)
	}

	profile.UnrecognizedHandling = unrecognized

	sortHandling(&profile)

	if err := measurement.Validate(); err != nil {
		return PhysicalMeasurement{}, HandlingProfile{}, err
	}

	return measurement, profile, nil
}

// sortHandling makes a handling profile deterministic. These values are
// persisted and compared across replans, so map iteration order must never
// reach them.
func sortHandling(profile *HandlingProfile) {
	sort.Slice(profile.Capabilities, func(i, j int) bool {
		return profile.Capabilities[i] < profile.Capabilities[j]
	})

	sort.Slice(profile.SpecialHandling, func(i, j int) bool {
		return profile.SpecialHandling[i] < profile.SpecialHandling[j]
	})

	sort.Strings(profile.UnrecognizedHandling)
	sort.Strings(profile.SeparationGroups)
}

// RequirementFrom builds a DeliveryRequirement from an aggregated checkout
// requirement.
//
// This is the ordinary path: checkout aggregates catalogue evidence into a
// Requirement, and this turns it into the canonical engine input without
// inventing anything the aggregation did not establish.
func RequirementFrom(requirement Requirement) DeliveryRequirement {
	handling := HandlingProfile{
		Capabilities: append(
			[]HandlingCapability(nil), requirement.Capabilities...,
		),
		SeparationGroups: append(
			[]string(nil), requirement.SeparationGroups...,
		),
	}

	sortHandling(&handling)

	return DeliveryRequirement{
		BusinessID: requirement.BusinessID,
		BranchID:   requirement.BranchID,
		Estimated: PhysicalMeasurement{
			State:              MeasurementEstimated,
			Source:             SourceCatalogEstimate,
			PackageCount:       requirement.PackageCount,
			TotalWeightGrams:   requirement.TotalWeightGrams,
			WeightEvidence:     requirement.WeightEvidence,
			TotalVolumeMl:      requirement.TotalVolumeMl,
			VolumeEvidence:     requirement.VolumeEvidence,
			LongestDimensionMm: requirement.LongestDimensionMm,
			DimensionEvidence:  requirement.DimensionEvidence,
			// The aggregation holds no per-package stacking evidence; a
			// stack-restricted line already arrives as FRAGILE_CARRY.
			Stackable: true,
			LargeLoad: requirement.LargeLoad,
		},
		Handling:     handling,
		Urgency:      requirement.Urgency,
		VehicleHints: append([]string(nil), requirement.VehicleHints...),
	}
}

// WithConfirmedPacking returns a copy of the requirement carrying confirmed
// packing evidence.
//
// Handling requirements are merged CONSERVATIVELY: a capability established
// by either the estimate or the packing stands. Packing that forgot to mark a
// frozen item frozen must not be able to unmark it.
func (d DeliveryRequirement) WithConfirmedPacking(
	measurement PhysicalMeasurement,
	packed HandlingProfile,
) DeliveryRequirement {
	confirmed := measurement
	d.Confirmed = &confirmed

	merged := HandlingProfile{
		Capabilities: append(
			[]HandlingCapability(nil), d.Handling.Capabilities...,
		),
		SpecialHandling: append(
			[]SpecialHandling(nil), d.Handling.SpecialHandling...,
		),
		UnrecognizedHandling: append(
			[]string(nil), d.Handling.UnrecognizedHandling...,
		),
		SeparationGroups: append(
			[]string(nil), d.Handling.SeparationGroups...,
		),
	}

	for _, capability := range packed.Capabilities {
		if !merged.Has(capability) {
			merged.Capabilities = append(merged.Capabilities, capability)
		}
	}

	for _, handling := range packed.SpecialHandling {
		if !merged.HasSpecial(handling) {
			merged.SpecialHandling = append(merged.SpecialHandling, handling)
		}
	}

	for _, value := range packed.UnrecognizedHandling {
		merged.UnrecognizedHandling = appendUnique(
			merged.UnrecognizedHandling, value,
		)
	}

	sortHandling(&merged)
	d.Handling = merged

	return d
}
