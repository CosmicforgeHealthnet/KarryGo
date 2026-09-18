// Package parcelfit is Dispatch's one hard-eligibility engine: it decides
// whether a specific bike can physically carry a specific parcel.
//
// It exists because Dispatch's candidate SQL answers a much weaker question.
// The clause it currently uses,
//
//	($2='MOTORCYCLE' AND b.bike_type IN ('motorcycle','dispatch_bike','scooter','electric_bike'))
//
// establishes that a machine is roughly the right kind of vehicle. It cannot
// establish that an 18kg parcel is within the machine's payload, that a
// 700mm box fits a 400mm carrier, that a chilled order will stay chilled, or
// that a fragile item will not be strapped to a bare rack. The commerce
// mission has carried parcel weight, volume, dimensions and handling codes
// since migration 000042; nothing has ever read them for eligibility.
//
// Like Hauling's cargofit, this is deliberately separate from ranking.
// Evaluate answers a physical yes/no and nothing else, so a nearer rider can
// never be scored back into a job their bike cannot do.
package parcelfit

import (
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/capabilityfit"
)

// Resolution is how a bike's capability record came back.
//
// The three failure modes are genuinely different and the current candidate
// SQL flattens all of them into "the EXISTS clause matched or it did not":
//
//	err != nil      -> the store did not answer     -> DEPENDENCY_UNAVAILABLE
//	record missing  -> never surveyed               -> CAPABILITY_UNKNOWN
//	record present  -> evaluate it
//
// Only the middle one is a fact about the bike, and only the last is a fact
// about the parcel.
type Resolution int

const (
	// ResolutionUnattempted means no lookup was wired. It is a dependency
	// failure, not permission to skip the check.
	ResolutionUnattempted Resolution = iota
	// ResolutionFailed means the lookup errored.
	ResolutionFailed
	// ResolutionMissing means the bike has no capability record.
	ResolutionMissing
	// ResolutionFound means a record is available.
	ResolutionFound
)

// Capability is the surveyed physical truth about one bike.
//
// Every optional fact is either a capabilityfit.Tri (whose zero value is
// Unknown) or a measurement paired with a *Known flag. A plain bool or a bare
// zero would turn "never surveyed" into "declared false" or "no limit", which
// is the conflation this whole package exists to prevent.
type Capability struct {
	BikeID     string
	ProviderID string
	BikeType   string

	// Active and Verified fold in the non-physical gates so one Evaluate call
	// answers the whole question and no caller can forget the second check.
	Active   bool
	Verified bool

	MaxPayloadGrams      int64
	MaxPayloadGramsKnown bool

	BoxLengthMm  int64
	BoxWidthMm   int64
	BoxHeightMm  int64
	BoxDimsKnown bool

	EnclosedBox      capabilityfit.Tri
	WeatherProtected capabilityfit.Tri
	UprightSafe      capabilityfit.Tri
	FragileSafe      capabilityfit.Tri

	ThermalBag       capabilityfit.Tri
	TemperatureClass string

	// ProductCode / ProductEligibility carry the bike's admission to the
	// product this work runs under, projected by the caller before evaluation.
	// Unknown and No are treated identically: a bike with no recorded
	// admission is not admitted, which is the entire point of requiring one.
	ProductCode            string
	ProductEligibility     capabilityfit.Tri
	ProductEligibilityNote string

	CapabilityVersion int64
	SourceVersion     int64
	ObservedAt        *time.Time
}

// Resolvable reports whether this is a real, currently usable bike.
func (c Capability) Resolvable() bool {
	return strings.TrimSpace(c.BikeID) != "" && c.Active && c.Verified
}

// HoldsTemperatureClass reports whether the bike's thermal equipment can hold
// the requested class.
//
// An unrecorded class on a bag-equipped bike is unknown, not "anything": a
// thermal bag that keeps a pizza warm does not keep ice cream frozen.
func (c Capability) HoldsTemperatureClass(required string) capabilityfit.Tri {
	wanted := strings.ToUpper(strings.TrimSpace(required))
	if wanted == "" || wanted == "AMBIENT" {
		return capabilityfit.Yes
	}
	if c.ThermalBag != capabilityfit.Yes {
		return c.ThermalBag
	}
	declared := strings.ToUpper(strings.TrimSpace(c.TemperatureClass))
	if declared == "" {
		return capabilityfit.Unknown
	}
	if declared == wanted {
		return capabilityfit.Yes
	}
	// A frozen-capable bag also serves chilled; the reverse is not true.
	if declared == "FROZEN" && wanted == "CHILLED" {
		return capabilityfit.Yes
	}
	return capabilityfit.No
}

// Requirement is one parcel's physical demands.
//
// It is built from the commerce mission's own parcel rows, which have carried
// these numbers since migration 000042, so no new data collection is needed to
// start enforcing it.
type Requirement struct {
	// WeightGrams is the total load. Zero means "not stated", which
	// constrains nothing rather than meaning weightless.
	WeightGrams int64

	// LengthMm / WidthMm / HeightMm are the largest single parcel's footprint.
	// A carrier must fit the biggest thing going into it.
	LengthMm int64
	WidthMm  int64
	HeightMm int64

	VolumeMl     int64
	PackageCount int64

	// DimensionsRequired makes the box check hard even when the parcel states
	// no dimensions, for products whose policy demands them.
	DimensionsRequired bool

	Fragile                   bool
	UprightRequired           bool
	WeatherProtectionRequired bool
	EnclosureRequired         bool

	TemperatureClass string

	// SecureHandover means the parcel must be released against a verified
	// credential. It never downgrades: a rider whose app cannot run the
	// challenge is not offered the job.
	SecureHandover bool

	VehicleClass string

	// ProductCode and CertificationRequired name the service product and
	// whether this deployment demands formal admission to it.
	ProductCode           string
	CertificationRequired bool
}

// HasDimensions reports whether any dimension was stated.
func (r Requirement) HasDimensions() bool {
	return r.LengthMm > 0 || r.WidthMm > 0 || r.HeightMm > 0
}

// NeedsThermal reports whether the temperature requirement needs equipment.
func (r Requirement) NeedsThermal() bool {
	switch strings.ToUpper(strings.TrimSpace(r.TemperatureClass)) {
	case "", "AMBIENT":
		return false
	default:
		return true
	}
}

// Evaluate decides whether one bike may carry one parcel.
//
// Order is deliberate: resolution, then structural gates, then certification,
// then the physical envelope, then handling. An operator reading the reasons
// sees the most fundamental problem first, and a bike that is simply
// unverified does not also generate four dimension complaints.
func Evaluate(resolution Resolution, capability Capability, requirement Requirement) *capabilityfit.Decision {
	decision := capabilityfit.NewDecision()

	switch resolution {
	case ResolutionFound:
		// fall through
	case ResolutionMissing:
		decision.Refuse(capabilityfit.ReasonCapabilityUnknown,
			"no capability record for this bike")
		return decision
	default:
		// An outage is never a statement about the bike. Keeping it separate
		// is what lets an operator tell "our capability store is down" from
		// "that bike cannot do this", and stops a transient failure being
		// recorded as a permanent incompatibility.
		decision.Refuse(capabilityfit.ReasonDependencyUnavailable,
			"bike capability could not be resolved")
		return decision
	}

	if !capability.Resolvable() {
		decision.Refuse(capabilityfit.ReasonProviderNotEligible,
			"bike is inactive or unverified")
		return decision
	}

	// A bike_type is not a certificate.
	if requirement.CertificationRequired && capability.ProductEligibility != capabilityfit.Yes {
		note := capability.ProductEligibilityNote
		if note == "" {
			note = "no valid eligibility for product " + requirement.ProductCode
		}
		decision.Refuse(capabilityfit.ReasonAssetNotCertified, note)
	}

	// Payload.
	decision.RequireBound(
		requirement.WeightGrams, capability.MaxPayloadGrams, capability.MaxPayloadGramsKnown,
		capabilityfit.ReasonCapacityExceeded,
		fmt.Sprintf("parcel %dg vs payload %dg", requirement.WeightGrams, capability.MaxPayloadGrams))

	evaluateBox(decision, capability, requirement)

	// Handling. Each is only checked when the parcel actually asks for it, so
	// an unsurveyed bike stays eligible for the ordinary work it can do.
	decision.RequireCapability(requirement.EnclosureRequired,
		capability.EnclosedBox, "enclosed carrier required")
	decision.RequireCapability(requirement.WeatherProtectionRequired,
		// An enclosed box is weatherproof by construction, so either fact
		// satisfies the weaker requirement.
		anyOf(capability.WeatherProtected, capability.EnclosedBox),
		"weather protection required")
	decision.RequireCapability(requirement.UprightRequired,
		capability.UprightSafe, "parcel must stay upright")
	decision.RequireCapability(requirement.Fragile,
		capability.FragileSafe, "fragile handling required")

	if requirement.NeedsThermal() {
		decision.RequireCapability(true,
			capability.HoldsTemperatureClass(requirement.TemperatureClass),
			"temperature class "+strings.ToUpper(requirement.TemperatureClass)+" required")
	}

	return decision
}

// evaluateBox checks the parcel against the carrier.
//
// Unlike a truck, a bike's box has no separate opening: the box IS the
// opening, so a parcel must fit its three internal dimensions in some
// orientation. Sorting both triples and comparing them pairwise is what makes
// "in some orientation" precise rather than an accident of which axis the
// caller happened to call length.
func evaluateBox(decision *capabilityfit.Decision, capability Capability, requirement Requirement) {
	if !requirement.HasDimensions() && !requirement.DimensionsRequired {
		return
	}
	if !capability.BoxDimsKnown {
		decision.Refuse(capabilityfit.ReasonCapabilityUnknown,
			"bike carrier dimensions not recorded")
		return
	}
	if requirement.DimensionsRequired && !requirement.HasDimensions() {
		decision.Refuse(capabilityfit.ReasonCapabilityUnknown,
			"product requires parcel dimensions and none were stated")
		return
	}

	parcel := sortedTriple(requirement.LengthMm, requirement.WidthMm, requirement.HeightMm)
	box := sortedTriple(capability.BoxLengthMm, capability.BoxWidthMm, capability.BoxHeightMm)
	for axis := 0; axis < 3; axis++ {
		if parcel[axis] > box[axis] {
			decision.Refuse(capabilityfit.ReasonCargoFitFailed,
				fmt.Sprintf("parcel %dx%dx%dmm does not fit a %dx%dx%dmm carrier",
					parcel[0], parcel[1], parcel[2], box[0], box[1], box[2]))
			return
		}
	}

	// An upright-only parcel cannot be rotated to fit, so it must clear the
	// box on its own axes as presented.
	if requirement.UprightRequired {
		if requirement.LengthMm > capability.BoxLengthMm ||
			requirement.WidthMm > capability.BoxWidthMm ||
			requirement.HeightMm > capability.BoxHeightMm {
			decision.Refuse(capabilityfit.ReasonCargoFitFailed,
				"upright parcel does not fit the carrier without rotating")
		}
	}
}

func sortedTriple(a, b, c int64) [3]int64 {
	triple := [3]int64{a, b, c}
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if triple[j] < triple[i] {
				triple[i], triple[j] = triple[j], triple[i]
			}
		}
	}
	return triple
}

// anyOf is the three-valued OR: Yes when anything is Yes, No when everything
// is No, Unknown otherwise. Treating Unknown as No here -- the obvious
// shortcut -- would turn a data gap into a confident refusal.
func anyOf(facts ...capabilityfit.Tri) capabilityfit.Tri {
	sawUnknown := false
	for _, fact := range facts {
		switch fact {
		case capabilityfit.Yes:
			return capabilityfit.Yes
		case capabilityfit.Unknown:
			sawUnknown = true
		}
	}
	if sawUnknown {
		return capabilityfit.Unknown
	}
	return capabilityfit.No
}
