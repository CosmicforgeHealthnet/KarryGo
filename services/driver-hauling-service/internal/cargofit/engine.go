package cargofit

import (
	"fmt"
	"strings"

	"cosmicforge/logistics/services/hauling-service/internal/truckcatalog"
	"cosmicforge/logistics/shared/go/capabilityfit"
)

// Resolution is how a truck's capability record came back.
//
// It is a separate input from the record itself because the three failure
// modes below are genuinely different situations that the old matcher
// flattened into one silent fallback:
//
//	err != nil          -> the store did not answer     -> DEPENDENCY_UNAVAILABLE
//	record missing      -> the truck is not resolvable  -> CAPABILITY_UNKNOWN
//	record present      -> evaluate it
//
// The previous code did `if err == nil { ...checks... }` and otherwise fell
// through to proximity-only matching, so both an outage and a deleted truck
// produced a MATCH. That is the single most consequential behaviour this
// package changes.
type Resolution int

const (
	// ResolutionUnattempted means no lookup was wired at all. It is treated as
	// a dependency failure rather than as permission to skip the check.
	ResolutionUnattempted Resolution = iota
	// ResolutionFailed means the lookup errored.
	ResolutionFailed
	// ResolutionMissing means the lookup succeeded and found no truck.
	ResolutionMissing
	// ResolutionFound means a capability record is available.
	ResolutionFound
)

// Evaluate decides whether one truck may carry one load.
//
// The order is deliberate: structural gates first (resolvable, active,
// verified, insured), then class and body, then the physical envelope, then
// equipment and goods. An operator reading the reason list sees the most
// fundamental problem first, and a truck that is simply switched off does not
// also generate five dimension complaints.
func Evaluate(resolution Resolution, capability TruckCapability, requirement Requirement) *capabilityfit.Decision {
	decision := capabilityfit.NewDecision()

	// ── 1-2. Resolution ───────────────────────────────────────────────────
	switch resolution {
	case ResolutionFound:
		// fall through to the real checks
	case ResolutionMissing:
		decision.Refuse(capabilityfit.ReasonCapabilityUnknown,
			"no capability record for this truck")
		return decision
	default:
		// An outage is never a statement about the truck. Naming it
		// separately is what lets an operator tell "our capability store is
		// down" from "this truck cannot do this", and stops a transient
		// failure being recorded as a permanent incompatibility.
		decision.Refuse(capabilityfit.ReasonDependencyUnavailable,
			"truck capability could not be resolved")
		return decision
	}

	// ── 3-4. Provider and truck compliance ────────────────────────────────
	if !capability.Resolvable() {
		decision.Refuse(capabilityfit.ReasonProviderNotEligible,
			"truck is inactive or unverified")
		return decision
	}

	// ── 5. Body family and class ──────────────────────────────────────────
	if !truckcatalog.SatisfiesPreferredType(capability.TruckType, requirement.PreferredTruckType) {
		decision.Refuse(capabilityfit.ReasonWrongAsset,
			fmt.Sprintf("truck type %s does not serve requested %s",
				capability.TruckType, requirement.PreferredTruckType))
	}
	if !truckcatalog.MatchesVehicleClass(capability.TruckType, requirement.VehicleClass) {
		decision.Refuse(capabilityfit.ReasonWrongAsset,
			fmt.Sprintf("truck type %s is not vehicle class %s",
				capability.TruckType, requirement.VehicleClass))
	}

	// A tanker carries bulk liquid and nothing else. This is checked from the
	// catalogue rather than the goods lists because it is true of the body by
	// construction, and a provider cannot opt out of it by leaving their
	// prohibited-goods list empty.
	facts := truckcatalog.FactsFor(capability.TruckType)
	if facts.BulkLiquidOnly && !requirementIsBulkLiquid(requirement) {
		decision.Refuse(capabilityfit.ReasonWrongAsset,
			"tank body carries bulk liquid only")
	}

	// ── 5b. Product eligibility ───────────────────────────────────────────
	//
	// A truck TYPE is not a certificate. "refrigerated" says the body cools;
	// admission to the cold-chain product says somebody inspected it. When the
	// product demands the second, the first cannot stand in for it.
	if requirement.CertificationRequired && capability.ProductEligibility != capabilityfit.Yes {
		note := capability.ProductEligibilityNote
		if note == "" {
			note = "no valid eligibility for product " + requirement.ProductCode
		}
		decision.Refuse(capabilityfit.ReasonAssetNotCertified, note)
	}

	// ── 6. Payload ────────────────────────────────────────────────────────
	decision.RequireBound(
		requirement.WeightGrams, capability.MaxPayloadGrams, capability.MaxPayloadGramsKnown,
		capabilityfit.ReasonCapacityExceeded,
		fmt.Sprintf("load %dg vs payload %dg", requirement.WeightGrams, capability.MaxPayloadGrams))

	// ── 7-8. Cargo dimensions and volume ──────────────────────────────────
	evaluateDimensions(decision, capability, requirement)

	// ── 9. Openings ───────────────────────────────────────────────────────
	evaluateOpenings(decision, capability, requirement, facts)

	// ── 10-11. Enclosure and weather protection ───────────────────────────
	decision.RequireCapability(requirement.EnclosureRequired,
		resolveFact(capability.Enclosed, facts.Enclosed), "enclosed body required")
	decision.RequireCapability(requirement.WeatherProtectionRequired,
		resolveFact(capability.WeatherProtected, facts.WeatherProtected), "weather protection required")
	decision.RequireCapability(requirement.UprightRequired,
		resolveFact(capability.UprightSafe, facts.UprightSafe), "load must stay upright")
	decision.RequireCapability(requirement.Fragile,
		capability.FragileSafe, "fragile handling required")

	// ── 12. Temperature ───────────────────────────────────────────────────
	evaluateTemperature(decision, capability, requirement, facts)

	// ── 13. Loading equipment ─────────────────────────────────────────────
	decision.RequireCapability(requirement.TailLiftRequired, capability.TailLift, "tail lift required")
	decision.RequireCapability(requirement.CraneRequired, capability.Crane, "crane required")
	// Forklift loading needs a way to get a forklift to the bed. Either a ramp
	// or a hydraulic tail lift satisfies it, so the two are combined rather
	// than demanding one specific piece of equipment.
	if requirement.ForkliftLoading {
		decision.RequireCapability(true,
			anyOf(capability.LoadingRamp, capability.TailLift, capability.Hydraulic),
			"forklift loading needs a ramp or lift")
	}

	// ── 14. Containers ────────────────────────────────────────────────────
	if size := strings.TrimSpace(requirement.ContainerSize); size != "" {
		if !facts.CarriesContainers {
			decision.Refuse(capabilityfit.ReasonWrongAsset,
				"body does not carry ISO containers")
		} else if len(capability.SupportedContainerSizes) == 0 {
			decision.Refuse(capabilityfit.ReasonCapabilityUnknown,
				"supported container sizes not recorded")
		} else if !capability.SupportsContainerSize(size) {
			decision.Refuse(capabilityfit.ReasonCargoFitFailed,
				"container size "+size+" not supported")
		}
	}

	// ── 15. Goods classes ─────────────────────────────────────────────────
	for _, class := range requirement.NormalizedGoodsClasses() {
		// The deny list is checked first and cannot be overridden by the
		// permit list: an explicit refusal to carry something must win.
		if capability.ProhibitsGoodsClass(class) {
			decision.Refuse(capabilityfit.ReasonRequiredCapabilityUnavailable,
				"truck prohibits goods class "+class)
			continue
		}
		if capability.DeclaresGoodsClasses() {
			if !capability.SupportsGoodsClass(class) {
				decision.Refuse(capabilityfit.ReasonRequiredCapabilityUnavailable,
					"truck does not support goods class "+class)
			}
			continue
		}
		// No permit list was stated. Fall back to what this body carries by
		// construction rather than to a blanket "general only", which would
		// refuse a tanker the bulk liquid it exists to move.
		if !truckcatalog.BodyCarriesByDefault(facts.Family, class) {
			decision.Refuse(capabilityfit.ReasonRequiredCapabilityUnavailable,
				"a "+string(facts.Family)+" body does not carry goods class "+class+" without an explicit permit")
		}
	}

	// ── Insurance ─────────────────────────────────────────────────────────
	// Only enforced when it was recorded at all: insurance is projected from
	// the provider profile and is genuinely absent for older trucks, so
	// demanding it unconditionally would ground the existing fleet. A recorded
	// NO still refuses.
	if capability.InsuranceValid == capabilityfit.No {
		decision.Refuse(capabilityfit.ReasonProviderNotEligible, "truck insurance is not valid")
	}

	return decision
}

// resolveFact prefers what the truck itself recorded, and falls back to what
// the catalogue knows about the body type.
//
// The order matters. A flatbed's own record saying "enclosed" would be a data
// error, but a recorded fact is still evidence about this vehicle, whereas the
// catalogue is a statement about the type. In practice the two only differ for
// types the catalogue leaves Unknown -- trailer, other -- which is exactly
// where the fallback earns its keep.
func resolveFact(recorded, fromType capabilityfit.Tri) capabilityfit.Tri {
	if recorded.Known() {
		return recorded
	}
	return fromType
}

// anyOf reports Yes when any input is Yes, No when all are No, and Unknown
// when nothing is Yes but something is unrecorded. It is the three-valued OR,
// and getting it wrong in the obvious way (treating Unknown as No) would turn
// a data gap into a confident refusal.
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

func evaluateDimensions(decision *capabilityfit.Decision, capability TruckCapability, requirement Requirement) {
	if !requirement.HasDimensions() && !requirement.DimensionsRequired {
		return
	}
	if !capability.InternalDimsKnown {
		if requirement.DimensionsRequired || requirement.HasDimensions() {
			decision.Refuse(capabilityfit.ReasonCapabilityUnknown,
				"truck internal dimensions not recorded")
		}
		return
	}
	if requirement.DimensionsRequired && !requirement.HasDimensions() {
		decision.Refuse(capabilityfit.ReasonCapabilityUnknown,
			"product requires cargo dimensions and none were stated")
		return
	}
	if requirement.LengthMm > capability.InternalLengthMm {
		decision.Refuse(capabilityfit.ReasonCargoFitFailed,
			fmt.Sprintf("cargo length %dmm exceeds %dmm", requirement.LengthMm, capability.InternalLengthMm))
	}
	if requirement.WidthMm > capability.InternalWidthMm {
		decision.Refuse(capabilityfit.ReasonCargoFitFailed,
			fmt.Sprintf("cargo width %dmm exceeds %dmm", requirement.WidthMm, capability.InternalWidthMm))
	}
	if requirement.HeightMm > capability.InternalHeightMm {
		decision.Refuse(capabilityfit.ReasonCargoFitFailed,
			fmt.Sprintf("cargo height %dmm exceeds %dmm", requirement.HeightMm, capability.InternalHeightMm))
	}
	if requirement.VolumeMl > 0 {
		decision.RequireBound(requirement.VolumeMl, capability.UsableVolumeMl, capability.UsableVolumeMlKnown,
			capabilityfit.ReasonCargoFitFailed,
			fmt.Sprintf("cargo volume %dml exceeds %dml", requirement.VolumeMl, capability.UsableVolumeMl))
	}
}

func evaluateOpenings(
	decision *capabilityfit.Decision,
	capability TruckCapability,
	requirement Requirement,
	facts truckcatalog.DeclaredFacts,
) {
	if !requirement.HasDimensions() {
		return
	}
	// An open deck has no door to fit through: cargo is craned or forked on
	// from above or the side. Demanding an opening measurement from a flatbed
	// would refuse every flatbed for no physical reason.
	switch facts.Family {
	case truckcatalog.FamilyOpenDeck, truckcatalog.FamilyLightOpen,
		truckcatalog.FamilyTipper, truckcatalog.FamilySpecialized:
		return
	}
	if !capability.OpeningDimsKnown {
		// Only unknown-refuse when the load is big enough for the door to
		// plausibly matter. A small parcel through an unmeasured van door is
		// not the risk this check exists for, and refusing it would make
		// every unmeasured van useless for ordinary work.
		if requirement.LongestDimensionMm() >= openingScrutinyThresholdMm {
			decision.Refuse(capabilityfit.ReasonCapabilityUnknown,
				"truck opening dimensions not recorded")
		}
		return
	}
	// The load must pass through the opening in SOME orientation, so the
	// smaller two of its three edges are what have to clear the doorway.
	first, second := twoSmallestEdges(requirement)
	if first > capability.OpeningWidthMm || second > capability.OpeningHeightMm {
		// Try the load rotated ninety degrees before refusing.
		if second > capability.OpeningWidthMm || first > capability.OpeningHeightMm {
			decision.Refuse(capabilityfit.ReasonCargoFitFailed,
				fmt.Sprintf("cargo cross-section %dx%dmm does not pass a %dx%dmm opening",
					first, second, capability.OpeningWidthMm, capability.OpeningHeightMm))
		}
	}
}

// openingScrutinyThresholdMm is the load size above which an unmeasured
// opening becomes a refusal. One metre is comfortably larger than anything a
// courier-sized consignment involves and comfortably smaller than the
// furniture and machinery cases where a doorway genuinely decides the outcome.
const openingScrutinyThresholdMm = 1000

func twoSmallestEdges(requirement Requirement) (int64, int64) {
	edges := []int64{requirement.LengthMm, requirement.WidthMm, requirement.HeightMm}
	for i := 0; i < len(edges); i++ {
		for j := i + 1; j < len(edges); j++ {
			if edges[j] < edges[i] {
				edges[i], edges[j] = edges[j], edges[i]
			}
		}
	}
	return edges[0], edges[1]
}

func evaluateTemperature(
	decision *capabilityfit.Decision,
	capability TruckCapability,
	requirement Requirement,
	facts truckcatalog.DeclaredFacts,
) {
	if !requirement.NeedsRefrigeration() {
		return
	}
	refrigerated := resolveFact(capability.Refrigerated, facts.Refrigerated)
	decision.RequireCapability(true, refrigerated, "refrigeration required")
	if refrigerated != capabilityfit.Yes {
		// No point complaining about the band of a truck that has no cooling.
		return
	}
	holds, known := capability.HoldsTemperature(requirement.MinTempC, requirement.MaxTempC)
	if !known {
		decision.Refuse(capabilityfit.ReasonCapabilityUnknown,
			"truck temperature range not recorded")
		return
	}
	if !holds {
		decision.Refuse(capabilityfit.ReasonRequiredCapabilityUnavailable,
			fmt.Sprintf("required band %s not within truck range %s",
				formatBand(requirement.MinTempC, requirement.MaxTempC),
				formatBand(capability.TemperatureMinC, capability.TemperatureMaxC)))
	}
}

func formatBand(minC, maxC *int) string {
	low, high := "?", "?"
	if minC != nil {
		low = fmt.Sprintf("%dC", *minC)
	}
	if maxC != nil {
		high = fmt.Sprintf("%dC", *maxC)
	}
	return low + ".." + high
}

func requirementIsBulkLiquid(requirement Requirement) bool {
	for _, class := range requirement.NormalizedGoodsClasses() {
		if class == "LIQUID_BULK" || class == "BULK_LIQUID" || class == "FUEL" {
			return true
		}
	}
	return false
}

// EvaluateProvider runs hard fit for one provider across all of their active
// trucks.
//
// A provider is eligible if ANY of their trucks fits, because the provider
// brings one truck to the job and gets to choose which. The decision returned
// is their BEST attempt, so an operator sees why the closest thing to a fit
// still failed rather than an arbitrary truck's complaints.
//
// It lives here, rather than in the matcher, because recovery must select a
// replacement by exactly the same rule the original match used. Two copies of
// this loop would be two places for "and also check the thermal bag" to be
// added to only one of them -- and the one that silently kept the old rule
// would be the one choosing a truck for cargo already on the road.
func EvaluateProvider(
	trucks []TruckCapability,
	resolution Resolution,
	requirement Requirement,
) (TruckCapability, *capabilityfit.Decision) {
	if resolution != ResolutionFound || len(trucks) == 0 {
		// Which of the two this is matters: a store that did not answer is a
		// dependency problem, an empty list is a provider with no active truck.
		if resolution == ResolutionFound {
			resolution = ResolutionMissing
		}
		return TruckCapability{}, Evaluate(resolution, TruckCapability{}, requirement)
	}

	var best *capabilityfit.Decision
	var bestTruck TruckCapability
	for _, truck := range trucks {
		candidate := Evaluate(ResolutionFound, truck, requirement)
		if candidate.Eligible {
			return truck, candidate
		}
		if best == nil || len(candidate.Reasons) < len(best.Reasons) {
			best, bestTruck = candidate, truck
		}
	}
	return bestTruck, best
}
