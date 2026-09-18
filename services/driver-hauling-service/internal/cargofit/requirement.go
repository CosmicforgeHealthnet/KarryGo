// Package cargofit is Hauling's one hard-eligibility engine: it decides
// whether a specific truck can physically carry a specific load.
//
// There is exactly one engine on purpose. Before this package, a direct
// booking was matched on two facts (capacity in kilograms, truck type as an
// exact string) while a Commerce mission carried parcel dimensions, handling
// codes and temperature requirements that nothing ever checked. The two paths
// therefore had different definitions of "can this truck do this job", and the
// richer one was the one being ignored. Both now call Evaluate.
//
// The engine is deliberately separate from ranking. Evaluate answers a yes/no
// physical question and nothing else -- no distance, no reliability, no score.
// A ranker runs afterwards over the survivors, so a high-scoring truck can
// never be talked back into a job it does not fit.
package cargofit

import (
	"strings"
	"time"
)

// Requirement is the normalized description of one load, shared by direct
// bookings and Commerce missions.
//
// Units are grams, millimetres, millilitres and whole degrees Celsius
// throughout, matching the rest of the platform's contracts. The legacy
// cargo_weight_kg column is converted on the way in rather than propagated, so
// there is one unit system inside the engine instead of two.
type Requirement struct {
	// WeightGrams is the total load weight. Zero means "not stated", which is
	// distinct from "weightless": a requirement that does not state a weight
	// simply does not constrain payload.
	WeightGrams int64

	// LengthMm / WidthMm / HeightMm are the largest single item's footprint,
	// not the sum of the load. A truck must fit the biggest thing going in it.
	LengthMm int64
	WidthMm  int64
	HeightMm int64

	// VolumeMl is the total load volume, which does aggregate.
	VolumeMl int64

	// PackageCount is how many discrete items.
	PackageCount int64

	// DimensionsRequired makes the dimension checks hard.
	//
	// It exists because "unknown dimensions" is a legitimate state for a
	// legacy direct booking -- the customer app has never collected them --
	// and refusing every such booking would take the service offline. When a
	// product or policy DOES require dimensions, this is set and an
	// unmeasured truck body then fails closed instead of waving the load
	// through.
	DimensionsRequired bool

	// Fragile, UprightRequired and Stackable describe handling.
	Fragile         bool
	UprightRequired bool
	Stackable       bool

	// WeatherProtectionRequired means the goods must not be exposed.
	WeatherProtectionRequired bool

	// EnclosureRequired means the body must be closed, which is stricter than
	// weather protection: a tarpaulin keeps rain off, a locked box also keeps
	// hands out.
	EnclosureRequired bool

	// TemperatureClass is a coarse code (AMBIENT, CHILLED, FROZEN). Anything
	// other than empty or AMBIENT requires active refrigeration.
	TemperatureClass string

	// MinTempC / MaxTempC are the setpoint band the goods must stay within.
	// Nil means unconstrained.
	MinTempC *int
	MaxTempC *int

	// HelpersRequired / HelperCount describe manual loading labour.
	HelpersRequired bool
	HelperCount     int

	// TailLiftRequired, CraneRequired, ForkliftLoading and LoadingBayRequired
	// are the loading-equipment requirements.
	//
	// ForkliftLoading means a forklift will load the truck, which needs a bed
	// a forklift can reach -- a ramp or a bay -- rather than equipment on the
	// truck itself.
	TailLiftRequired   bool
	CraneRequired      bool
	ForkliftLoading    bool
	LoadingBayRequired bool

	// GoodsClasses names what is being carried (GENERAL, LIQUID_BULK,
	// HAZARDOUS, PERISHABLE, ...). A truck must support every class present
	// and prohibit none of them.
	GoodsClasses []string

	// ContainerSize is set when the load is an ISO container.
	ContainerSize string

	// PickupWindowStart / PickupWindowEnd and LoadingDurationSeconds carry the
	// scheduling shape a Commerce mission commits to. The engine does not
	// schedule; it carries them so the one requirement object is complete and
	// callers do not need a second, parallel struct.
	PickupWindowStart      *time.Time
	PickupWindowEnd        *time.Time
	LoadingDurationSeconds int64

	// PreferredTruckType is the customer's type request, widened to its body
	// family by the catalogue. Empty means any truck.
	PreferredTruckType string

	// VehicleClass is the cross-service class requirement (VAN, TRUCK).
	VehicleClass string

	// ProductCode is the service product this work runs under.
	ProductCode string

	// CertificationRequired makes formal product eligibility a hard gate.
	//
	// It is set from deployment policy rather than inferred from the product
	// code, because which products need a certificate is an operational
	// decision that differs by market and must be changeable without a
	// release.
	CertificationRequired bool
}

// FromKilograms converts the legacy cargo_weight_kg column.
//
// It is a named helper rather than an inline multiplication so every
// conversion site is greppable: mixing the two units silently is a
// thousand-fold error, and a thousand-fold error in payload is the difference
// between a parcel and a pallet.
func FromKilograms(kg int) int64 {
	if kg <= 0 {
		return 0
	}
	return int64(kg) * 1000
}

// LongestDimensionMm is the largest single edge of the load.
func (r Requirement) LongestDimensionMm() int64 {
	longest := r.LengthMm
	if r.WidthMm > longest {
		longest = r.WidthMm
	}
	if r.HeightMm > longest {
		longest = r.HeightMm
	}
	return longest
}

// HasDimensions reports whether any dimension was stated.
func (r Requirement) HasDimensions() bool {
	return r.LengthMm > 0 || r.WidthMm > 0 || r.HeightMm > 0
}

// NeedsRefrigeration reports whether the temperature requirement can only be
// met by active cooling.
//
// An empty class and AMBIENT both mean no cooling. Any other class does, and
// so does an explicit maximum below normal ambient: a requirement that says
// "no warmer than 4C" needs a reefer whether or not anyone typed CHILLED.
func (r Requirement) NeedsRefrigeration() bool {
	switch strings.ToUpper(strings.TrimSpace(r.TemperatureClass)) {
	case "", "AMBIENT":
		break
	default:
		return true
	}
	return r.MaxTempC != nil && *r.MaxTempC < ambientCeilingC
}

// ambientCeilingC is the warmest setpoint a non-refrigerated body can be
// assumed to hold. Anything colder needs active cooling.
const ambientCeilingC = 15

// NormalizedGoodsClasses upper-cases and de-blanks the goods vocabulary.
func (r Requirement) NormalizedGoodsClasses() []string {
	return normalizeCodes(r.GoodsClasses)
}

func normalizeCodes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.ToUpper(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func containsCode(haystack []string, needle string) bool {
	for _, candidate := range haystack {
		if strings.EqualFold(strings.TrimSpace(candidate), needle) {
			return true
		}
	}
	return false
}
