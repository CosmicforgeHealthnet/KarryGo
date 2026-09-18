package cargofit

import (
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/capabilityfit"
)

// TruckCapability is the physical truth about one truck.
//
// Two kinds of field appear here and they behave differently on purpose:
//
//   - capabilityfit.Tri fields are facts that may genuinely not be recorded.
//     Their zero value is Unknown, so a truck nobody has surveyed refuses any
//     job that depends on the missing fact instead of passing it.
//
//   - int64 measurements pair with a *Known flag for the same reason. Zero is
//     not "no limit"; an unmeasured payload is unknown, and an unknown payload
//     must not carry a tonne.
//
// The alternative -- plain bools and bare zeroes -- is what let the previous
// matcher treat `CapacityKg > 0` as the only gate and silently skip the check
// entirely for every truck whose capacity had never been entered.
type TruckCapability struct {
	// ── Identity ──────────────────────────────────────────────────────────
	TruckID    string
	ProviderID string
	TruckType  string
	BodyFamily string

	// Active and Verified are the non-physical gates. They are here rather
	// than checked separately so one Evaluate call answers "may this truck
	// take this job" completely.
	Active   bool
	Verified bool

	// ── Payload ───────────────────────────────────────────────────────────
	MaxPayloadGrams      int64
	MaxPayloadGramsKnown bool

	// ── Cargo body ────────────────────────────────────────────────────────
	InternalLengthMm  int64
	InternalWidthMm   int64
	InternalHeightMm  int64
	InternalDimsKnown bool

	UsableVolumeMl      int64
	UsableVolumeMlKnown bool

	// ── Openings ──────────────────────────────────────────────────────────
	//
	// A load that fits the body may still not fit through the door. Openings
	// are tracked separately because they are a different constraint, and on
	// an open deck they do not exist at all.
	OpeningWidthMm   int64
	OpeningHeightMm  int64
	OpeningDimsKnown bool

	// ── Protection ────────────────────────────────────────────────────────
	Enclosed         capabilityfit.Tri
	WeatherProtected capabilityfit.Tri

	// ── Temperature ───────────────────────────────────────────────────────
	Refrigerated     capabilityfit.Tri
	TemperatureMinC  *int
	TemperatureMaxC  *int
	TemperatureClass string

	// ── Loading equipment ─────────────────────────────────────────────────
	TailLift    capabilityfit.Tri
	Crane       capabilityfit.Tri
	Hydraulic   capabilityfit.Tri
	LoadingRamp capabilityfit.Tri

	// ── Configuration ─────────────────────────────────────────────────────
	Axles                   int
	TrailerCapable          capabilityfit.Tri
	SupportedContainerSizes []string

	// ── Goods ─────────────────────────────────────────────────────────────
	//
	// Supported is a permit list and Prohibited a deny list. Both exist
	// because they answer different questions: an empty Supported means "no
	// restriction stated", while a non-empty Prohibited is an explicit refusal
	// that must win even if the class also appears as supported.
	SupportedGoodsClasses  []string
	ProhibitedGoodsClasses []string

	// ── Orientation ───────────────────────────────────────────────────────
	UprightSafe capabilityfit.Tri
	FragileSafe capabilityfit.Tri

	// ── Product eligibility ───────────────────────────────────────────────
	//
	// ProductEligibility is the truck's admission to ProductCode, projected by
	// the caller before evaluation. It is three-valued, but note that Unknown
	// and No are treated identically by the engine: a truck with no recorded
	// admission is NOT admitted. That is the whole point of requiring a
	// certificate -- absence of evidence is exactly what it refuses.
	//
	// A store that could not be read is signalled by Resolution, not here, so
	// an outage never masquerades as an uncertified truck.
	ProductCode            string
	ProductEligibility     capabilityfit.Tri
	ProductEligibilityNote string

	// ── Compliance / provenance ───────────────────────────────────────────
	InsuranceValid   capabilityfit.Tri
	InspectionRef    string
	CertificationRef string

	// CapabilityVersion increments on every recorded change, so evidence can
	// pin what a decision was made against and a stale write can be refused.
	CapabilityVersion int64

	// SourceVersion is the version of the upstream record this projection was
	// built from, for callback ordering.
	SourceVersion int64

	// ObservedAt is when these facts were last confirmed.
	ObservedAt *time.Time
}

// Resolvable reports whether the capability record represents a real,
// currently usable truck.
func (c TruckCapability) Resolvable() bool {
	return strings.TrimSpace(c.TruckID) != "" && c.Active && c.Verified
}

// SupportsContainerSize reports whether the truck carries this ISO size.
func (c TruckCapability) SupportsContainerSize(size string) bool {
	return containsCode(c.SupportedContainerSizes, strings.ToUpper(strings.TrimSpace(size)))
}

// ProhibitsGoodsClass reports an explicit refusal.
func (c TruckCapability) ProhibitsGoodsClass(class string) bool {
	return containsCode(c.ProhibitedGoodsClasses, strings.ToUpper(strings.TrimSpace(class)))
}

// SupportsGoodsClass reports whether an explicitly declared permit list covers
// the class.
//
// It is only meaningful when SupportedGoodsClasses is non-empty. An empty list
// means the provider stated no restriction at all, which is not the same as
// "nothing is permitted"; the engine falls back to what the body family
// carries by construction, because the alternative refuses a tanker its own
// bulk liquid and offers it pallets instead.
func (c TruckCapability) SupportsGoodsClass(class string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(class))
	if normalized == "" {
		return true
	}
	return containsCode(c.SupportedGoodsClasses, normalized)
}

// DeclaresGoodsClasses reports whether a permit list was stated at all.
func (c TruckCapability) DeclaresGoodsClasses() bool {
	return len(c.SupportedGoodsClasses) > 0
}

// HoldsTemperature reports whether the truck's declared band covers the
// requested one.
//
// A nil bound on the truck is unknown, not unlimited: a reefer whose range was
// never recorded cannot promise -18C. Callers reach this only after
// Refrigerated has already been confirmed Yes.
func (c TruckCapability) HoldsTemperature(minC, maxC *int) (ok bool, known bool) {
	if minC == nil && maxC == nil {
		return true, true
	}
	if c.TemperatureMinC == nil || c.TemperatureMaxC == nil {
		return false, false
	}
	if minC != nil && *minC < *c.TemperatureMinC {
		return false, true
	}
	if maxC != nil && *maxC > *c.TemperatureMaxC {
		return false, true
	}
	return true, true
}
