package fulfilment

// The vehicle selection engine.
//
// A pure, deterministic evaluator. The same requirement, the same capability
// policy and the same serviceability snapshot always produce the same
// decision, with the same reasons, in the same order. There is no inference,
// no model, no scoring and no randomness: every answer is a walk down an
// ordered ladder of configured thresholds, and every rung records why it was
// or was not taken.
//
// It has NO side effects. It creates no mission, calls no authority, writes
// no row, sends no notification and touches no money. It returns a decision;
// what to do with that decision belongs to fulfilment planning.
//
// The engine answers two different questions and keeps them separate:
//
//   - What is the MINIMUM PHYSICAL CLASS this load needs? A fact about the
//     load. A car can be the answer.
//
//   - What is the SELECTED EXECUTABLE SERVICE? What the platform can actually
//     perform. When taxi goods mode is unavailable, a load whose minimum
//     physical class is a car is executed by the next class that both fits
//     and can be executed - never by something smaller.
//
// Collapsing those two would lose the distinction between "your order needs a
// bigger vehicle" and "we cannot currently run the vehicle your order needs",
// which are different answers to a customer and different problems to an
// operator.
//
// THE ANTI-FRAGMENTATION RULE. The engine returns ONE class for ONE load. It
// never answers "three motorcycles" for a load that exceeds one motorcycle,
// because splitting a coherent load across small vehicles to avoid a van
// multiplies custody handovers, riders, failure points and support cases for
// no benefit. Legitimate splits - separate branches, incompatible handling,
// different ready windows - are decided upstream by fulfilment planning,
// which presents each resulting load here on its own.

import (
	"sort"
	"time"
)

// CandidateEvaluation is one vehicle class as the engine saw it.
//
// Every configured class produces exactly one of these, whether or not it was
// chosen. Recording the classes that failed is what turns "why a van" from a
// guess into an answer.
type CandidateEvaluation struct {
	Class VehicleClass `json:"vehicle_class"`
	Rank  int          `json:"rank"`

	Service Service `json:"service"`

	// PhysicallyFits is about boxes and vehicles only.
	PhysicallyFits bool `json:"physically_fits"`

	// Executable is whether the owning authority could actually perform it.
	Executable bool `json:"executable"`

	// ReasonCode is the single controlled code for this class's outcome.
	ReasonCode string `json:"reason_code"`
}

// VehicleDecision is one complete, explainable selection.
type VehicleDecision struct {
	// MinimumPhysicalClass is the smallest configured class that can
	// physically carry the load, whether or not anything can execute it.
	MinimumPhysicalClass VehicleClass `json:"minimum_physical_class,omitempty"`

	// MinimumPhysicalFound is false when nothing in the catalogue fits.
	MinimumPhysicalFound bool `json:"minimum_physical_found"`

	// SelectedClass and SelectedService are the executable answer. They are
	// empty when Executable is false.
	SelectedClass   VehicleClass `json:"selected_vehicle_class,omitempty"`
	SelectedService Service      `json:"selected_service,omitempty"`

	Executable bool `json:"executable"`

	// PrimaryReason is the one code that answers "why this outcome".
	PrimaryReason string `json:"primary_reason"`

	// SupportingReasons are the additional codes an operator needs to
	// understand the decision. A van chosen because a car cannot be executed
	// looks identical to a van chosen because the load is heavy, unless the
	// difference is recorded.
	SupportingReasons []string `json:"supporting_reasons,omitempty"`

	Candidates []CandidateEvaluation `json:"candidates"`

	PolicyVersion string `json:"policy_version"`

	// EffectiveVolumeMl is the volume actually compared against each class,
	// after any non-stackable derate. It is reported so the comparison can be
	// reproduced from the snapshot alone.
	EffectiveVolumeMl int64 `json:"effective_volume_ml"`

	// MeasurementState and MeasurementSource say which evidence the decision
	// was taken against.
	MeasurementState  MeasurementState  `json:"measurement_state"`
	MeasurementSource MeasurementSource `json:"measurement_source,omitempty"`

	EvaluatedAt time.Time `json:"evaluated_at"`
}

// Selected reports whether the decision produced an executable service.
func (d VehicleDecision) Selected() bool {
	return d.Executable && d.SelectedClass != "" && d.SelectedService != ""
}

// Escalated reports whether the executed class is larger than the minimum
// physical requirement.
func (d VehicleDecision) Escalated() bool {
	return d.Selected() && d.MinimumPhysicalFound &&
		d.SelectedClass != d.MinimumPhysicalClass
}

// hasReason reports whether a supporting reason was recorded.
func (d VehicleDecision) hasReason(code string) bool {
	for _, reason := range d.SupportingReasons {
		if reason == code {
			return true
		}
	}

	return false
}

// Selection projects the decision onto the narrower shape the mission
// pipeline consumes.
//
// The projection is lossy by design: consolidation and quoting only need to
// know which class and which authority. Everything they discard - the
// candidates, the policy version, the minimum physical class - stays on the
// snapshot, which is where an explanation is read from. The reason travels
// with the projection so a refused group still carries a code a customer can
// be shown.
func (d VehicleDecision) Selection() Selection {
	selection := Selection{
		ReasonCode:       d.PrimaryReason,
		HintFloorApplied: d.hasReason(ReasonHintFloorApplied),
	}

	if d.Selected() {
		selection.Class = d.SelectedClass
		selection.Service = d.SelectedService
	}

	return selection
}

// SelectionRequest is everything the evaluator needs, and nothing more.
//
// Serviceability is passed IN rather than looked up, which is what makes the
// evaluator pure and therefore replayable: a stored snapshot plus a stored
// policy reproduces a stored decision exactly, without any live service being
// reachable.
type SelectionRequest struct {
	Requirement DeliveryRequirement

	Policy CapabilityPolicy

	Serviceability ServiceabilitySnapshot

	// Now is the clock the SLA check is made against. Passed explicitly so a
	// replay produces the original answer rather than today's.
	Now time.Time
}

// Evaluate produces the vehicle decision for one load.
//
// It returns an error only for an input that is not a physically meaningful
// statement. An unserviceable load is a normal decision with Executable false
// and a reason the customer can be shown - not an error.
func Evaluate(request SelectionRequest) (VehicleDecision, error) {
	requirement := request.Requirement

	if err := requirement.Validate(); err != nil {
		return VehicleDecision{}, err
	}

	policy := request.Policy.Normalized()
	measurement := requirement.Authoritative()
	now := request.Now

	if now.IsZero() {
		now = time.Now()
	}

	decision := VehicleDecision{
		PolicyVersion:     policy.Version,
		MeasurementState:  measurement.State,
		MeasurementSource: measurement.Source,
		EvaluatedAt:       now.UTC(),
	}

	if decision.MeasurementState == "" {
		decision.MeasurementState = MeasurementEstimated
	}

	// Conflicts are refused before any vehicle is considered. A bigger van
	// does not make hot and frozen goods compatible, and a load the vendor
	// said must be separated is not made safe by a larger compartment.
	if requirement.Handling.TemperatureConflict() {
		decision.PrimaryReason = ReasonTemperatureConflict

		return decision, nil
	}

	if requirement.Handling.SeparationConflict() {
		decision.PrimaryReason = ReasonSeparationConflict

		return decision, nil
	}

	// Incomplete evidence is answered by the configured policy, in the open,
	// before any threshold comparison. Two of the four answers refuse to
	// decide at all.
	weak := measurement.WeaklyEvidenced() || !measurement.Measured()
	unrecognized := len(requirement.Handling.UnrecognizedHandling) > 0
	// Packed weight is the one measurement every platform-delivered line
	// must state; anything short of COMPLETE is a lower bound.
	weightIncomplete := measurement.WeightEvidence != EvidenceComplete ||
		measurement.TotalWeightGrams <= 0

	if refusal, refuse := metadataRefusal(
		policy.MissingMetadata, weak, unrecognized, weightIncomplete,
	); refuse {
		decision.PrimaryReason = refusal

		if weak || weightIncomplete {
			decision.SupportingReasons = append(
				decision.SupportingReasons, ReasonWeakEvidence,
			)
		}

		if unrecognized {
			decision.SupportingReasons = append(
				decision.SupportingReasons, ReasonUnrecognizedHandling,
			)
		}

		return decision, nil
	}

	effectiveVolume, err := effectiveVolumeMl(
		measurement, policy.NonStackableVolumeFactorBP,
	)
	if err != nil {
		return VehicleDecision{}, err
	}

	decision.EffectiveVolumeMl = effectiveVolume

	floor, hinted := hintFloor(requirement.VehicleHints)

	// One pass over the ordered ladder. Everything the decision needs is
	// collected here so no class is evaluated twice and the recorded
	// candidate order matches the order the engine actually walked.
	var (
		firstFit     *CandidateEvaluation
		selected     *CandidateEvaluation
		sawFit       bool
		blockedByCar bool
	)

	escalationFloor := policyEscalationFloor(policy, weak)

	if floor < escalationFloor {
		floor = escalationFloor
	}

	for _, capability := range policy.Classes {
		candidate := CandidateEvaluation{
			Class:   capability.Class,
			Rank:    capability.Rank,
			Service: capability.Service,
		}

		if reason, fits := physicalFit(
			capability, requirement, measurement, effectiveVolume,
		); !fits {
			candidate.ReasonCode = reason
			decision.Candidates = append(decision.Candidates, candidate)

			continue
		}

		candidate.PhysicallyFits = true
		sawFit = true

		if firstFit == nil {
			snapshot := candidate
			firstFit = &snapshot
		}

		if reason, executable := executability(
			capability, requirement, request.Serviceability, now,
		); !executable {
			candidate.ReasonCode = reason

			if capability.Class == VehicleCar &&
				reason == ReasonGoodsModeDisabled {
				blockedByCar = true
			}

			decision.Candidates = append(decision.Candidates, candidate)

			continue
		}

		candidate.Executable = true

		if capability.Rank < floor {
			// The vendor's own hints, or the configured escalation policy,
			// put a floor under this load. A class below it fits and could be
			// executed, and is still refused - which is exactly why the
			// reason is recorded rather than the class silently skipped.
			candidate.ReasonCode = ReasonBelowVendorHintFloor
			decision.Candidates = append(decision.Candidates, candidate)

			continue
		}

		if selected == nil {
			candidate.ReasonCode = ReasonCandidateSelected
			chosen := candidate
			selected = &chosen
		} else {
			candidate.ReasonCode = ReasonCandidateFitsNotSmallest
		}

		decision.Candidates = append(decision.Candidates, candidate)
	}

	if firstFit != nil {
		decision.MinimumPhysicalClass = firstFit.Class
		decision.MinimumPhysicalFound = true
	}

	switch {
	case selected != nil:
		decision.SelectedClass = selected.Class
		decision.SelectedService = selected.Service
		decision.Executable = true
		decision.PrimaryReason = ReasonCandidateSelected

	case !sawFit:
		// Nothing in the catalogue can carry this load at all.
		decision.PrimaryReason = ReasonNoSuitableVehicle

	default:
		// Something fits, but nothing that fits can be executed.
		decision.PrimaryReason = ReasonNoExecutableService
	}

	decision.SupportingReasons = append(
		decision.SupportingReasons,
		supportingReasons(decision, requirement, measurement, weak,
			unrecognized, hinted, floor, blockedByCar)...,
	)

	return decision, nil
}

// metadataRefusal applies the configured missing-metadata policy.
//
// PROCEED and CONSERVATIVE_ESCALATION both still produce a decision; only
// REQUIRE_METADATA and MANUAL_REVIEW refuse to decide. Escalation is applied
// later as a class floor rather than here, because it changes which vehicle
// is chosen rather than whether one is.
func metadataRefusal(
	policy MissingMetadataPolicy,
	weak, unrecognized, weightIncomplete bool,
) (string, bool) {
	if policy == MissingMetadataRequirePackedWeight &&
		(weightIncomplete || unrecognized) {
		return ReasonMetadataIncomplete, true
	}

	if !weak && !unrecognized {
		return "", false
	}

	switch policy {
	case MissingMetadataRequire:
		return ReasonMetadataIncomplete, true

	case MissingMetadataManualReview:
		return ReasonManualReviewRequired, true

	default:
		return "", false
	}
}

// policyEscalationFloor is the minimum rank the configured policy imposes
// when evidence is weak.
//
// Rank 2 rather than "the next class up" is deliberate: the escalation exists
// to keep an unmeasured load off the smallest vehicle, and stating that as a
// fixed floor keeps the rule reproducible when the ladder changes shape.
func policyEscalationFloor(policy CapabilityPolicy, weak bool) int {
	if !weak || policy.MissingMetadata != MissingMetadataConservativeEscalation {
		return 0
	}

	return 2
}

// effectiveVolumeMl applies the non-stackable derate.
//
// The multiplication is checked because a derate that overflowed would wrap
// to a small volume and put an oversized load on a bike - the precise failure
// the derate exists to prevent.
func effectiveVolumeMl(
	measurement PhysicalMeasurement,
	factorBP int,
) (int64, error) {
	if measurement.TotalVolumeMl <= 0 || measurement.Stackable {
		return measurement.TotalVolumeMl, nil
	}

	if factorBP <= 0 {
		factorBP = DefaultNonStackableVolumeFactorBP
	}

	scaled, ok := mulChecked(measurement.TotalVolumeMl, int64(factorBP))
	if !ok {
		return 0, ErrRequirementOverflow
	}

	return scaled / 10000, nil
}

// physicalFit reports whether a class can carry the load, and the first
// reason it cannot.
//
// The order of the checks is the order an operator would ask them in, and it
// is fixed: two identical loads must always fail for the same stated reason,
// or the reason code is not evidence of anything.
//
// A zero limit means unbounded. See VehicleCapability.
func physicalFit(
	capability VehicleCapability,
	requirement DeliveryRequirement,
	measurement PhysicalMeasurement,
	effectiveVolume int64,
) (string, bool) {
	if capability.MaxWeightGrams > 0 &&
		measurement.TotalWeightGrams > capability.MaxWeightGrams {
		return ReasonWeightExceeded, false
	}

	if capability.MaxVolumeMl > 0 && effectiveVolume > capability.MaxVolumeMl {
		return ReasonVolumeExceeded, false
	}

	// The longest single edge, never the sum. A light load with one long item
	// still will not go into a courier box, which is why a low total weight
	// never implies motorcycle suitability.
	if capability.MaxLongestDimensionMm > 0 &&
		measurement.LongestDimensionMm > capability.MaxLongestDimensionMm {
		return ReasonDimensionExceeded, false
	}

	if capability.MaxPackageCount > 0 &&
		measurement.PackageCount > capability.MaxPackageCount {
		return ReasonPackageCountExceeded, false
	}

	if capability.MaxSinglePackageWeightGrams > 0 &&
		measurement.MaxSinglePackageWeightGrams >
			capability.MaxSinglePackageWeightGrams {
		return ReasonSinglePackageTooHeavy, false
	}

	if measurement.LargeLoad && !capability.AllowsLargeLoad {
		return ReasonLargeLoadUnsupported, false
	}

	for _, needed := range requirement.Handling.Capabilities {
		if !capability.handlesCapability(needed) {
			return ReasonHandlingUnsupported, false
		}
	}

	for _, needed := range requirement.Handling.SpecialHandling {
		if !capability.handlesSpecial(needed) {
			return ReasonSpecialHandlingUnsupported, false
		}
	}

	if requirement.Value.HighValue() && !capability.AllowsHighValue {
		return ReasonHighValueUnsupported, false
	}

	return "", true
}

// executability reports whether the owning authority could actually perform
// this mission, and the first reason it could not.
//
// Every check here is about the SERVICE, not the load. That separation is
// what lets the decision say "a car would have been right, but we cannot run
// one" instead of pretending the load needed a van.
func executability(
	capability VehicleCapability,
	requirement DeliveryRequirement,
	serviceability ServiceabilitySnapshot,
	now time.Time,
) (string, bool) {
	// The structural gate. A class whose owning service has no production
	// goods mode is refused here and can never be reached by any amount of
	// upstream change or configuration of the other flags.
	if !capability.GoodsModeActive {
		return ReasonGoodsModeDisabled, false
	}

	if !serviceability.IntegrationEnabled(capability.Service) {
		return ReasonIntegrationDisabled, false
	}

	if !serviceability.RouteServed(capability.Service) {
		return ReasonOutOfServiceArea, false
	}

	// A mission needing more than one collection point cannot be handed to a
	// runtime that cannot drive one. Modelling the capability rather than
	// hardcoding false is what lets a future multi-pickup runtime be switched
	// on without touching this logic.
	if requirement.Route.PickupCount > 1 &&
		!capability.SupportsMulti &&
		!serviceability.MultiPickupSupported(capability.Service) {
		return ReasonMultiPickupUnsupported, false
	}

	if capability.MaxRouteDistanceMeters > 0 &&
		requirement.Route.DistanceMeters > capability.MaxRouteDistanceMeters {
		return ReasonRouteDistanceExceeded, false
	}

	if ok, known := requirement.Timing.SLASatisfiable(now); known && !ok {
		return ReasonSLANotMet, false
	}

	return "", true
}

// supportingReasons assembles the evidence codes that accompany the decision.
//
// They are sorted so two identical decisions serialize identically: these end
// up on an immutable snapshot that is compared across replans, and an
// unstable ordering would make two identical decisions look different.
func supportingReasons(
	decision VehicleDecision,
	requirement DeliveryRequirement,
	measurement PhysicalMeasurement,
	weak, unrecognized, hinted bool,
	floor int,
	blockedByCar bool,
) []string {
	var reasons []string

	add := func(code string) {
		if !decision.hasReason(code) {
			reasons = append(reasons, code)
		}
	}

	if blockedByCar {
		// The single most misleading outcome this engine can produce is a van
		// that looks like a heavy load but is really a disabled integration.
		add(ReasonCarPhysicallySuitable)
	}

	if decision.Escalated() {
		add(ReasonEscalatedAboveMinimum)
	}

	if hinted && floor > 1 {
		add(ReasonHintFloorApplied)
	}

	if weak {
		add(ReasonWeakEvidence)
	}

	if unrecognized {
		add(ReasonUnrecognizedHandling)
	}

	if requirement.PackingConfirmed() {
		add(ReasonConfirmedMeasurement)
	}

	if measurement.TotalVolumeMl > 0 && !measurement.Stackable {
		add(ReasonNonStackableDerate)
	}

	sort.Strings(reasons)

	return reasons
}
