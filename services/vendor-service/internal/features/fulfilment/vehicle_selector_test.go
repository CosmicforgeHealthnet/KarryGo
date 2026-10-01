package fulfilment

// Tests for the delivery requirement and vehicle selection engine.
//
// The engine decides which vehicle carries goods a customer paid for. Every
// test here pins a rule that, if it broke, would either put a load on a
// vehicle that cannot safely carry it or refuse a delivery the platform could
// have made. They are written as statements about the outcome rather than
// about the implementation, so a rewrite of the evaluator that preserves the
// behaviour keeps passing.

import (
	"errors"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

var fixedNow = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

// everyServiceEnabled is a deployment with every integration switched on. Tests that
// are about the LOAD use it, so an unexpected refusal is never a disabled
// integration in disguise.
func everyServiceEnabled() ServiceabilitySnapshot {
	return ServiceabilityFrom(map[Service]bool{
		ServiceDispatch: true,
		ServiceHauling:  true,
		ServiceTaxi:     true,
	}, fixedNow)
}

// loadOf builds a requirement for a plainly measured, ordinary load.
func loadOf(weightGrams, volumeMl int64, longestMm int, packages int64) DeliveryRequirement {
	return DeliveryRequirement{
		Estimated: PhysicalMeasurement{
			State:              MeasurementEstimated,
			Source:             SourceCatalogEstimate,
			PackageCount:       packages,
			TotalWeightGrams:   weightGrams,
			WeightEvidence:     EvidenceComplete,
			TotalVolumeMl:      volumeMl,
			VolumeEvidence:     EvidenceComplete,
			LongestDimensionMm: longestMm,
			DimensionEvidence:  EvidenceComplete,
			Stackable:          true,
		},
	}
}

func decide(t *testing.T, requirement DeliveryRequirement) VehicleDecision {
	t.Helper()

	return decideWith(t, requirement, everyServiceEnabled())
}

func decideWith(
	t *testing.T,
	requirement DeliveryRequirement,
	serviceability ServiceabilitySnapshot,
) VehicleDecision {
	t.Helper()

	decision, err := Evaluate(SelectionRequest{
		Requirement:    requirement,
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: serviceability,
		Now:            fixedNow,
	})
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}

	return decision
}

func candidateFor(
	t *testing.T, decision VehicleDecision, class VehicleClass,
) CandidateEvaluation {
	t.Helper()

	for _, candidate := range decision.Candidates {
		if candidate.Class == class {
			return candidate
		}
	}

	t.Fatalf("no candidate recorded for %s", class)

	return CandidateEvaluation{}
}

func hasReason(decision VehicleDecision, code string) bool {
	for _, reason := range decision.SupportingReasons {
		if reason == code {
			return true
		}
	}

	return false
}

// -----------------------------------------------------------------------------
// Vehicle priority and the escalation ladder
// -----------------------------------------------------------------------------

// A load a motorcycle can carry goes by motorcycle. Anything else is the
// platform charging a customer for a van it did not need.
func TestEngine_SmallLoadSelectsMotorcycle(t *testing.T) {
	decision := decide(t, loadOf(3_000, 5_000, 300, 2))

	if !decision.Selected() {
		t.Fatalf("small load refused: %s", decision.PrimaryReason)
	}

	if decision.SelectedClass != VehicleMotorcycle {
		t.Fatalf("selected %s, want MOTORCYCLE", decision.SelectedClass)
	}

	if decision.SelectedService != ServiceDispatch {
		t.Fatalf("motorcycle routed to %s, want DISPATCH",
			decision.SelectedService)
	}
}

// Weight past the motorcycle envelope escalates, and the refusal says why.
func TestEngine_WeightEscalatesPastMotorcycle(t *testing.T) {
	decision := decide(t, loadOf(40_000, 5_000, 300, 2))

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("a 40 kg load was assigned to a motorcycle")
	}

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonWeightExceeded {
		t.Fatalf("motorcycle refused for %q, want WEIGHT_EXCEEDED", got)
	}
}

// A light load with one long item still will not go in a courier box. Volume
// and weight both passing must not override the single-edge check.
func TestEngine_LongItemEscalatesRegardlessOfWeight(t *testing.T) {
	decision := decide(t, loadOf(2_000, 4_000, 1_800, 1))

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("a 1.8 m item was assigned to a motorcycle")
	}

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonDimensionExceeded {
		t.Fatalf("motorcycle refused for %q, want PACKAGE_DIMENSION_EXCEEDED", got)
	}
}

// Volume alone can exceed the envelope while the weight is trivial.
func TestEngine_VolumeEscalatesPastMotorcycle(t *testing.T) {
	decision := decide(t, loadOf(1_000, 300_000, 400, 4))

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("a 300 L load was assigned to a motorcycle")
	}

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonVolumeExceeded {
		t.Fatalf("motorcycle refused for %q, want VOLUME_EXCEEDED", got)
	}
}

// Package count is a suitability constraint of its own. Twenty small parcels
// are within every weight and volume limit and still are not one bike load.
func TestEngine_PackageCountEscalatesPastMotorcycle(t *testing.T) {
	decision := decide(t, loadOf(6_000, 20_000, 200, 40))

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("40 packages were assigned to a motorcycle")
	}

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonPackageCountExceeded {
		t.Fatalf("motorcycle refused for %q, want PACKAGE_COUNT_EXCEEDED", got)
	}
}

// The smallest class that fits wins. A load that outgrows a van must not be
// answered with the largest truck when a smaller one would do - but here the
// point is the reverse: a van-sized load must not be quoted as a truck.
func TestEngine_SelectsSmallestSuitableClass(t *testing.T) {
	decision := decide(t, loadOf(300_000, 2_000_000, 2_000, 60))

	if decision.SelectedClass != VehicleVan {
		t.Fatalf("selected %s, want VAN - the smallest class that fits",
			decision.SelectedClass)
	}

	if decision.SelectedService != ServiceHauling {
		t.Fatalf("van routed to %s, want HAULING", decision.SelectedService)
	}

	// The truck fits too, and the engine must say so rather than pretending it
	// was never considered.
	truck := candidateFor(t, decision, VehicleTruck)
	if !truck.PhysicallyFits {
		t.Fatal("truck should physically fit a van-sized load")
	}

	if truck.ReasonCode != ReasonCandidateFitsNotSmallest {
		t.Fatalf("truck recorded as %q, want LARGER_THAN_REQUIRED",
			truck.ReasonCode)
	}
}

// A load past the van envelope takes the truck, and nothing smaller.
func TestEngine_TruckSelectedWhenVanCannotCarry(t *testing.T) {
	decision := decide(t, loadOf(4_000_000, 20_000_000, 6_000, 800))

	if decision.SelectedClass != VehicleTruck {
		t.Fatalf("selected %s, want TRUCK", decision.SelectedClass)
	}

	if candidateFor(t, decision, VehicleVan).PhysicallyFits {
		t.Fatal("a 4 tonne load must not be recorded as fitting a van")
	}
}

// THE ANTI-FRAGMENTATION INVARIANT.
//
// A single load that outgrows a motorcycle is answered with ONE larger
// vehicle. The engine returns one class, never a count, so splitting a
// coherent load across several bikes to dodge a van is not merely discouraged
// - it is unrepresentable.
func TestEngine_NeverAnswersWithSeveralMotorcycles(t *testing.T) {
	// Three motorcycle-loads' worth of goods in one obligation.
	decision := decide(t, loadOf(70_000, 220_000, 600, 9))

	if !decision.Selected() {
		t.Fatalf("load refused: %s", decision.PrimaryReason)
	}

	if decision.SelectedClass != VehicleCar {
		t.Fatalf("selected %s, want CAR - one vehicle, not several bikes",
			decision.SelectedClass)
	}

	if candidateFor(t, decision, VehicleMotorcycle).PhysicallyFits {
		t.Fatal("motorcycle must not be recorded as fitting a 70 kg load")
	}
}

// -----------------------------------------------------------------------------
// The taxi gate
// -----------------------------------------------------------------------------

func TestEngine_CarRoutesToTaxiGoodsWhenEnabled(t *testing.T) {
	// Past every motorcycle limit, comfortably inside the car envelope.
	decision := decide(t, loadOf(120_000, 300_000, 1_400, 20))

	if decision.MinimumPhysicalClass != VehicleCar {
		t.Fatalf("minimum physical class is %s, want CAR",
			decision.MinimumPhysicalClass)
	}

	if decision.SelectedClass != VehicleCar || decision.SelectedService != ServiceTaxi {
		t.Fatalf("selected %s/%s, want CAR/TAXI", decision.SelectedClass, decision.SelectedService)
	}

}

func TestEngine_TaxiGoodsRuntimeGateFailsClosed(t *testing.T) {
	serviceability := ServiceabilitySnapshot{
		Integrations: map[Service]bool{
			ServiceDispatch: true,
			ServiceHauling:  true,
			ServiceTaxi:     false,
		},
		ServiceArea: map[Service]bool{ServiceTaxi: true},
		MultiPickup: map[Service]bool{ServiceTaxi: true},
		CapturedAt:  fixedNow,
	}

	decision := decideWith(t, loadOf(120_000, 300_000, 1_400, 20), serviceability)

	if decision.SelectedClass == VehicleCar || decision.SelectedService == ServiceTaxi {
		t.Fatal("disabled Taxi Goods integration made a car mission selectable")
	}
}

// -----------------------------------------------------------------------------
// Handling and temperature
// -----------------------------------------------------------------------------

// An insulated courier box is not a cold chain. Chilled goods must leave the
// motorcycle behind however light they are.
func TestEngine_ColdChainEscalatesPastMotorcycle(t *testing.T) {
	requirement := loadOf(1_000, 2_000, 200, 1)
	requirement.Handling.Capabilities = []HandlingCapability{CapabilityColdChain}

	decision := decide(t, requirement)

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("chilled goods were assigned to a motorcycle")
	}

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonHandlingUnsupported {
		t.Fatalf("motorcycle refused for %q, want HANDLING_UNSUPPORTED", got)
	}
}

func TestEngine_FrozenEscalatesPastMotorcycle(t *testing.T) {
	requirement := loadOf(1_000, 2_000, 200, 1)
	requirement.Handling.Capabilities = []HandlingCapability{CapabilityFrozenChain}

	if decide(t, requirement).SelectedClass == VehicleMotorcycle {
		t.Fatal("frozen goods were assigned to a motorcycle")
	}
}

// Hot food and spill risk are both within a courier box's competence. A
// platform that escalated them would price every food delivery as a van.
func TestEngine_HotAndSpillStayOnMotorcycle(t *testing.T) {
	requirement := loadOf(2_000, 4_000, 250, 2)
	requirement.Handling.Capabilities = []HandlingCapability{
		CapabilityHotHold, CapabilityLeakContainment, CapabilityUprightCarry,
	}

	decision := decide(t, requirement)

	if decision.SelectedClass != VehicleMotorcycle {
		t.Fatalf("hot upright spill-risk food escalated to %s",
			decision.SelectedClass)
	}
}

// Fragile goods are outside a motorcycle's competence and inside a van's.
func TestEngine_FragileEscalatesPastMotorcycle(t *testing.T) {
	requirement := loadOf(2_000, 4_000, 250, 2)
	requirement.Handling.Capabilities = []HandlingCapability{CapabilityFragileCarry}

	decision := decide(t, requirement)

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("fragile goods were assigned to a motorcycle")
	}

	if !decision.Selected() {
		t.Fatalf("fragile goods refused entirely: %s", decision.PrimaryReason)
	}
}

// THE TEMPERATURE CONFLICT INVARIANT.
//
// Hot and frozen goods in one compartment is not solved by a bigger vehicle.
// Silently combining them is what this refuses.
func TestEngine_HotAndFrozenIsRefusedNotEscalated(t *testing.T) {
	requirement := loadOf(2_000, 4_000, 250, 2)
	requirement.Handling.Capabilities = []HandlingCapability{
		CapabilityHotHold, CapabilityFrozenChain,
	}

	decision := decide(t, requirement)

	if decision.Selected() {
		t.Fatalf("hot and frozen goods were combined onto a %s",
			decision.SelectedClass)
	}

	if decision.PrimaryReason != ReasonTemperatureConflict {
		t.Fatalf("refused for %q, want TEMPERATURE_CONFLICT",
			decision.PrimaryReason)
	}

	// A van "handles" both capabilities in the capability table. The refusal
	// must come before any vehicle is considered, or the table would let the
	// conflict through.
	if len(decision.Candidates) != 0 {
		t.Fatal("a conflicted load was evaluated against vehicles at all")
	}
}

func TestEngine_ChilledAndFrozenIsAConflict(t *testing.T) {
	requirement := loadOf(2_000, 4_000, 250, 2)
	requirement.Handling.Capabilities = []HandlingCapability{
		CapabilityColdChain, CapabilityFrozenChain,
	}

	if decide(t, requirement).PrimaryReason != ReasonTemperatureConflict {
		t.Fatal("two different controlled bands in one compartment was allowed")
	}
}

// A hot meal and a cold drink in one insulated box is ordinary work, not a
// conflict. Over-refusing here would break every food delivery.
func TestEngine_HotAndInsulatedIsNotAConflict(t *testing.T) {
	requirement := loadOf(2_000, 4_000, 250, 2)
	requirement.Handling.Capabilities = []HandlingCapability{
		CapabilityHotHold, CapabilityInsulated,
	}

	decision := decide(t, requirement)

	if !decision.Selected() || decision.SelectedClass != VehicleMotorcycle {
		t.Fatalf("a hot meal with a cold drink was not a motorcycle load: %s / %s",
			decision.PrimaryReason, decision.SelectedClass)
	}
}

// Items the vendor declared must not travel together are not made compatible
// by a larger compartment.
func TestEngine_SeparationConflictIsRefused(t *testing.T) {
	requirement := loadOf(2_000, 4_000, 250, 2)
	requirement.Handling.SeparationGroups = []string{"RAW", "READY_TO_EAT"}

	decision := decide(t, requirement)

	if decision.Selected() {
		t.Fatal("goods declared inseparable were put on one vehicle")
	}

	if decision.PrimaryReason != ReasonSeparationConflict {
		t.Fatalf("refused for %q, want SEPARATION_CONFLICT",
			decision.PrimaryReason)
	}
}

// -----------------------------------------------------------------------------
// Value and special handling
// -----------------------------------------------------------------------------

// An open courier box is not a secure compartment for a high-value load.
func TestEngine_HighValueEscalatesPastMotorcycle(t *testing.T) {
	requirement := loadOf(1_500, 3_000, 200, 1)
	requirement.Value = ValueProfile{
		DeclaredValueMinor: 90_000_000,
		Currency:           "NGN",
		RiskClass:          RiskHighValue,
	}

	decision := decide(t, requirement)

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("a high-value load was assigned to a motorcycle")
	}

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonHighValueUnsupported {
		t.Fatalf("motorcycle refused for %q, want HIGH_VALUE_UNSUPPORTED", got)
	}
}

// Declared value is risk metadata. It bounds nothing financial, and the engine
// must not be able to express a fee at all - which it cannot, having no field
// for one. This pins the weaker but checkable claim: value never changes the
// class when the risk band does not.
func TestEngine_DeclaredValueWithoutHighRiskDoesNotEscalate(t *testing.T) {
	requirement := loadOf(1_500, 3_000, 200, 1)
	requirement.Value = ValueProfile{
		DeclaredValueMinor: 90_000_000,
		Currency:           "NGN",
		RiskClass:          RiskStandard,
	}

	if decide(t, requirement).SelectedClass != VehicleMotorcycle {
		t.Fatal("a declared value alone escalated the vehicle class")
	}
}

// KEEP_DRY goods cannot ride an open bike.
func TestEngine_SpecialHandlingFiltersClasses(t *testing.T) {
	requirement := loadOf(1_500, 3_000, 200, 1)
	requirement.Handling.SpecialHandling = []SpecialHandling{
		SpecialHandlingKeepDry,
	}

	decision := decide(t, requirement)

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("keep-dry goods were assigned to an open motorcycle box")
	}

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonSpecialHandlingUnsupported {
		t.Fatalf("motorcycle refused for %q, want SPECIAL_HANDLING_UNSUPPORTED",
			got)
	}
}

// An unrecognized classification must never select a vehicle. It is recorded
// as weak evidence so the decision is visibly taken without it.
func TestEngine_UnrecognizedHandlingIsRecordedNotIgnored(t *testing.T) {
	requirement := loadOf(1_500, 3_000, 200, 1)
	requirement.Handling.UnrecognizedHandling = []string{"NEEDS_UNICORN"}

	decision := decide(t, requirement)

	if !hasReason(decision, ReasonUnrecognizedHandling) {
		t.Fatal("an unrecognized handling classification was silently dropped")
	}
}

// -----------------------------------------------------------------------------
// Serviceability, separate from physical suitability
// -----------------------------------------------------------------------------

// A load a motorcycle can carry, in a deployment where dispatch is switched
// off, must escalate rather than fail - and the refusal must be an OPERATIONAL
// reason, not a claim that the load was too big.
func TestEngine_DisabledIntegrationIsNotAPhysicalRefusal(t *testing.T) {
	serviceability := ServiceabilityFrom(map[Service]bool{
		ServiceDispatch: false,
		ServiceHauling:  true,
	}, fixedNow)

	decision := decideWith(t, loadOf(3_000, 5_000, 300, 2), serviceability)

	motorcycle := candidateFor(t, decision, VehicleMotorcycle)

	if !motorcycle.PhysicallyFits {
		t.Fatal("a disabled integration was recorded as a physical misfit")
	}

	if motorcycle.ReasonCode != ReasonIntegrationDisabled {
		t.Fatalf("motorcycle refused for %q, want the service-unavailable code",
			motorcycle.ReasonCode)
	}

	if decision.MinimumPhysicalClass != VehicleMotorcycle {
		t.Fatalf("minimum physical class became %s; physical suitability must not depend on integrations",
			decision.MinimumPhysicalClass)
	}

	if decision.SelectedClass != VehicleVan {
		t.Fatalf("selected %s, want VAN", decision.SelectedClass)
	}
}

// A route the authority does not operate is not a smaller vehicle's problem.
func TestEngine_OutOfServiceAreaRefusesTheClass(t *testing.T) {
	serviceability := ServiceabilitySnapshot{
		Integrations: map[Service]bool{
			ServiceDispatch: true, ServiceHauling: true,
		},
		ServiceArea: map[Service]bool{ServiceDispatch: false},
		CapturedAt:  fixedNow,
	}

	decision := decideWith(t, loadOf(3_000, 5_000, 300, 2), serviceability)

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonOutOfServiceArea {
		t.Fatalf("motorcycle refused for %q, want OUT_OF_SERVICE_AREA", got)
	}
}

// With nothing enabled, the answer is an operational refusal naming no
// service - never a physical one, and never a mission to a service that is off.
func TestEngine_NothingEnabledFailsClosed(t *testing.T) {
	decision := decideWith(
		t, loadOf(3_000, 5_000, 300, 2),
		ServiceabilityFrom(map[Service]bool{}, fixedNow),
	)

	if decision.Selected() {
		t.Fatal("a mission was selected with no integration enabled")
	}

	if decision.PrimaryReason != ReasonNoExecutableService {
		t.Fatalf("refused for %q, want NO_SUITABLE_EXECUTABLE_SERVICE",
			decision.PrimaryReason)
	}

	if decision.SelectedService != "" {
		t.Fatalf("a refusal named service %q", decision.SelectedService)
	}

	// The load itself was fine. Saying otherwise would send a customer away
	// believing their order was too large.
	if !decision.MinimumPhysicalFound {
		t.Fatal("an operational refusal was reported as a physical one")
	}
}

// A load nothing can carry is refused as a load, not as an outage.
func TestEngine_ExceedsEveryClass(t *testing.T) {
	decision := decide(t, loadOf(500_000_000, 500_000_000, 90_000, 90_000))

	if decision.Selected() {
		t.Fatal("a load beyond every vehicle was assigned one")
	}

	if decision.PrimaryReason != ReasonNoSuitableVehicle {
		t.Fatalf("refused for %q, want NO_SUITABLE_VEHICLE",
			decision.PrimaryReason)
	}

	if decision.MinimumPhysicalFound {
		t.Fatal("a load nothing fits reported a minimum physical class")
	}
}

// -----------------------------------------------------------------------------
// Route, SLA and multi-pickup
// -----------------------------------------------------------------------------

// A promise that cannot be met by a class is that class's problem, not the
// load's.
func TestEngine_SLAUnreachableRefusesTheClass(t *testing.T) {
	requirement := loadOf(3_000, 5_000, 300, 2)

	promised := fixedNow.Add(20 * time.Minute)
	requirement.Timing = TimingProfile{
		PromisedBy:              &promised,
		EstimatedTransitMinutes: 90,
	}

	decision := decide(t, requirement)

	if decision.Selected() {
		t.Fatal("a delivery was planned that cannot arrive in time")
	}

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonSLANotMet {
		t.Fatalf("motorcycle refused for %q, want SLA_NOT_MET", got)
	}
}

// A promise that can be met changes nothing.
func TestEngine_SLAReachableSelectsNormally(t *testing.T) {
	requirement := loadOf(3_000, 5_000, 300, 2)

	promised := fixedNow.Add(3 * time.Hour)
	requirement.Timing = TimingProfile{
		PromisedBy:              &promised,
		EstimatedTransitMinutes: 40,
	}

	if decide(t, requirement).SelectedClass != VehicleMotorcycle {
		t.Fatal("a comfortably achievable promise escalated the class")
	}
}

// Transit cannot start before the vendor is ready, and the SLA check must say
// so rather than measuring from now.
func TestEngine_SLACountsFromTheReadyWindow(t *testing.T) {
	requirement := loadOf(3_000, 5_000, 300, 2)

	ready := fixedNow.Add(2 * time.Hour)
	promised := fixedNow.Add(150 * time.Minute)

	requirement.Timing = TimingProfile{
		ReadyFrom:               &ready,
		PromisedBy:              &promised,
		EstimatedTransitMinutes: 45,
	}

	decision := decide(t, requirement)

	if decision.Selected() {
		t.Fatal("the promise was judged achievable by ignoring the ready window")
	}
}

// A route longer than a class can serve is refused for distance, not weight.
func TestEngine_RouteDistanceRefusesTheClass(t *testing.T) {
	requirement := loadOf(3_000, 5_000, 300, 2)
	requirement.Route = RouteProfile{DistanceMeters: 200_000, PickupCount: 1}

	decision := decide(t, requirement)

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonRouteDistanceExceeded {
		t.Fatalf("motorcycle refused for %q, want ROUTE_DISTANCE_EXCEEDED", got)
	}

	if decision.SelectedClass != VehicleVan {
		t.Fatalf("a 200 km route selected %s, want VAN", decision.SelectedClass)
	}
}

// THE MULTI-PICKUP INVARIANT.
//
// No logistics runtime models a multi-stop collection today. A plan needing
// one must not be handed to a service that cannot drive it, however suitable
// the vehicle is.
func TestEngine_MultiPickupFailsClosedAgainstTodaysRuntimes(t *testing.T) {
	requirement := loadOf(3_000, 5_000, 300, 2)
	requirement.Route = RouteProfile{PickupCount: 3}

	decision := decide(t, requirement)

	if decision.Selected() {
		t.Fatalf("a three-pickup mission was planned onto %s, which cannot drive it",
			decision.SelectedClass)
	}

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonMultiPickupUnsupported {
		t.Fatalf("motorcycle refused for %q, want MULTI_PICKUP_UNSUPPORTED", got)
	}
}

// The capability is modelled, not hardcoded off: a runtime that gains
// multi-pickup is switched on by configuration, without touching the engine.
func TestEngine_MultiPickupBecomesSelectableWhenTheRuntimeSupportsIt(t *testing.T) {
	serviceability := ServiceabilitySnapshot{
		Integrations: map[Service]bool{
			ServiceDispatch: true, ServiceHauling: true,
		},
		MultiPickup: map[Service]bool{ServiceDispatch: true},
		CapturedAt:  fixedNow,
	}

	requirement := loadOf(3_000, 5_000, 300, 2)
	requirement.Route = RouteProfile{PickupCount: 3}

	decision := decideWith(t, requirement, serviceability)

	if decision.SelectedClass != VehicleMotorcycle {
		t.Fatalf("selected %s; a supported multi-pickup runtime should be usable",
			decision.SelectedClass)
	}
}

// -----------------------------------------------------------------------------
// Vendor hints
// -----------------------------------------------------------------------------

// A vendor hint raises the floor and never lowers it.
func TestEngine_VendorHintRaisesTheClass(t *testing.T) {
	requirement := loadOf(1_000, 2_000, 200, 1)
	requirement.VehicleHints = []string{"VAN"}

	decision := decide(t, requirement)

	if decision.SelectedClass != VehicleVan {
		t.Fatalf("selected %s, want VAN - the vendor said its load needs one",
			decision.SelectedClass)
	}

	if !hasReason(decision, ReasonHintFloorApplied) {
		t.Fatal("the decision does not record that a vendor hint raised it")
	}

	if got := candidateFor(t, decision, VehicleMotorcycle).ReasonCode; got != ReasonBelowVendorHintFloor {
		t.Fatalf("motorcycle refused for %q, want BELOW_VENDOR_HINT_FLOOR", got)
	}
}

// A hint cannot shrink a load below what it physically needs. Safety limits
// are the server's, not the vendor's.
func TestEngine_VendorHintCannotDownsizeAnOversizedLoad(t *testing.T) {
	requirement := loadOf(400_000, 2_000_000, 2_000, 60)
	requirement.VehicleHints = []string{"MOTORCYCLE"}

	decision := decide(t, requirement)

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("a vendor hint put a 400 kg load on a motorcycle")
	}

	if decision.SelectedClass != VehicleVan {
		t.Fatalf("selected %s, want VAN", decision.SelectedClass)
	}
}

// -----------------------------------------------------------------------------
// Estimated versus confirmed
// -----------------------------------------------------------------------------

// Confirmed packing outranks a catalogue estimate, and the estimate survives
// so the escalation can be explained.
func TestEngine_ConfirmedPackingEscalatesAndKeepsTheEstimate(t *testing.T) {
	requirement := loadOf(7_000, 20_000, 400, 2)

	// Packed heavier than the catalogue estimated: 32 kg is past the 25 kg
	// motorcycle envelope, so the class must move.
	confirmed, handling, err := ConfirmFromPacking([]PackedParcel{
		{WeightGrams: 16_000, LengthMm: 400, WidthMm: 300, HeightMm: 300, Stackable: true},
		{WeightGrams: 16_000, LengthMm: 400, WidthMm: 300, HeightMm: 300, Stackable: true},
	})
	if err != nil {
		t.Fatalf("packing: %v", err)
	}

	escalated := requirement.WithConfirmedPacking(confirmed, handling)

	before := decide(t, requirement)
	after := decide(t, escalated)

	if before.SelectedClass != VehicleMotorcycle {
		t.Fatalf("estimate selected %s, want MOTORCYCLE", before.SelectedClass)
	}

	if after.SelectedClass == VehicleMotorcycle {
		t.Fatal("a 32 kg confirmed load stayed on a motorcycle")
	}

	// The estimate must still be readable. An escalation nobody can compare to
	// its original is an escalation nobody can explain.
	if escalated.Estimated.TotalWeightGrams != 7_000 {
		t.Fatalf("estimate was overwritten: %d",
			escalated.Estimated.TotalWeightGrams)
	}

	if escalated.Authoritative().TotalWeightGrams != 32_000 {
		t.Fatalf("authoritative weight is %d, want the confirmed 32000",
			escalated.Authoritative().TotalWeightGrams)
	}

	if after.MeasurementState != MeasurementConfirmed {
		t.Fatalf("decision claims %s evidence, want CONFIRMED",
			after.MeasurementState)
	}

	if !hasReason(after, ReasonConfirmedMeasurement) {
		t.Fatal("the decision does not record that it used packed evidence")
	}
}

// A confirmed load smaller than its estimate may plan smaller. Refusing to
// would charge the customer for a van that was never needed.
func TestEngine_ConfirmedPackingMayReduceTheClass(t *testing.T) {
	requirement := loadOf(200_000, 400_000, 1_500, 30)

	confirmed, handling, err := ConfirmFromPacking([]PackedParcel{
		{WeightGrams: 2_000, LengthMm: 200, WidthMm: 200, HeightMm: 200, Stackable: true},
	})
	if err != nil {
		t.Fatalf("packing: %v", err)
	}

	after := decide(t, requirement.WithConfirmedPacking(confirmed, handling))

	if after.SelectedClass != VehicleMotorcycle {
		t.Fatalf("a 2 kg confirmed load selected %s, want MOTORCYCLE",
			after.SelectedClass)
	}
}

// Packing that forgot a handling flag must not be able to unset one the
// catalogue established. A merge that could lose FROZEN would put frozen goods
// on a bike.
func TestEngine_ConfirmedPackingCannotDropAHandlingRequirement(t *testing.T) {
	requirement := loadOf(3_000, 6_000, 300, 2)
	requirement.Handling.Capabilities = []HandlingCapability{CapabilityFrozenChain}

	confirmed, handling, err := ConfirmFromPacking([]PackedParcel{
		{WeightGrams: 2_000, LengthMm: 200, WidthMm: 200, HeightMm: 200,
			Stackable: true, TemperatureControl: "NONE"},
	})
	if err != nil {
		t.Fatalf("packing: %v", err)
	}

	merged := requirement.WithConfirmedPacking(confirmed, handling)

	if !merged.Handling.Has(CapabilityFrozenChain) {
		t.Fatal("packing silently dropped the frozen-chain requirement")
	}

	if decide(t, merged).SelectedClass == VehicleMotorcycle {
		t.Fatal("frozen goods reached a motorcycle after packing")
	}
}

// -----------------------------------------------------------------------------
// Stackability
// -----------------------------------------------------------------------------

// A load that cannot be stacked cannot use the full cube of a compartment.
// Pretending it can is how a van arrives that the goods will not go into.
func TestEngine_NonStackableLoadDeratesUsableVolume(t *testing.T) {
	requirement := loadOf(2_000, 70_000, 300, 4)
	requirement.Estimated.Stackable = false

	decision := decide(t, requirement)

	if decision.EffectiveVolumeMl <= 70_000 {
		t.Fatalf("effective volume %d was not derated above the raw 70000",
			decision.EffectiveVolumeMl)
	}

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("a non-stackable 70 L load stayed within the 80 L bike envelope")
	}

	if !hasReason(decision, ReasonNonStackableDerate) {
		t.Fatal("the derate was applied without being recorded")
	}
}

// -----------------------------------------------------------------------------
// Missing metadata policy
// -----------------------------------------------------------------------------

func evaluateWithPolicy(
	t *testing.T,
	requirement DeliveryRequirement,
	missing MissingMetadataPolicy,
) VehicleDecision {
	t.Helper()

	policy := DefaultCapabilityPolicy()
	policy.MissingMetadata = missing

	decision, err := Evaluate(SelectionRequest{
		Requirement:    requirement,
		Policy:         policy,
		Serviceability: everyServiceEnabled(),
		Now:            fixedNow,
	})
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}

	return decision
}

func unmeasuredLoad() DeliveryRequirement {
	return DeliveryRequirement{
		Estimated: PhysicalMeasurement{
			State:             MeasurementEstimated,
			Source:            SourceCatalogEstimate,
			PackageCount:      2,
			WeightEvidence:    EvidenceAbsent,
			VolumeEvidence:    EvidenceAbsent,
			DimensionEvidence: EvidenceAbsent,
			Stackable:         true,
		},
	}
}

// The platform's stated policy: plan on the lower bound, and record that the
// evidence was weak rather than pretending it was not.
func TestEngine_MissingMetadataProceedsAndRecordsWeakEvidence(t *testing.T) {
	decision := evaluateWithPolicy(t, unmeasuredLoad(), MissingMetadataProceed)

	if !decision.Selected() {
		t.Fatalf("an unmeasured load was refused: %s", decision.PrimaryReason)
	}

	if !hasReason(decision, ReasonWeakEvidence) {
		t.Fatal("a decision on absent measurements did not record it")
	}
}

// A deployment that would rather escalate than guess gets an escalation, not a
// refusal.
func TestEngine_MissingMetadataCanEscalateInstead(t *testing.T) {
	decision := evaluateWithPolicy(
		t, unmeasuredLoad(), MissingMetadataConservativeEscalation,
	)

	if !decision.Selected() {
		t.Fatalf("conservative escalation refused outright: %s",
			decision.PrimaryReason)
	}

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("an unmeasured load was put on the smallest vehicle")
	}
}

// A deployment carrying regulated goods can refuse to decide at all.
func TestEngine_MissingMetadataCanRefuseToDecide(t *testing.T) {
	for _, testCase := range []struct {
		policy MissingMetadataPolicy
		reason string
	}{
		{MissingMetadataRequire, ReasonMetadataIncomplete},
		{MissingMetadataManualReview, ReasonManualReviewRequired},
	} {
		decision := evaluateWithPolicy(t, unmeasuredLoad(), testCase.policy)

		if decision.Selected() {
			t.Fatalf("%s selected a vehicle anyway", testCase.policy)
		}

		if decision.PrimaryReason != testCase.reason {
			t.Fatalf("%s refused for %q, want %q",
				testCase.policy, decision.PrimaryReason, testCase.reason)
		}
	}
}

// -----------------------------------------------------------------------------
// Boundaries, determinism and hostile input
// -----------------------------------------------------------------------------

// Exactly at the limit fits; one gram over does not. Off-by-one at a safety
// threshold is a load on the wrong vehicle.
func TestEngine_WeightBoundaryIsExact(t *testing.T) {
	limit, ok := DefaultCapabilityPolicy().Lookup(VehicleMotorcycle)
	if !ok {
		t.Fatal("motorcycle capability missing from the default policy")
	}

	for _, testCase := range []struct {
		name   string
		weight int64
		want   VehicleClass
	}{
		{"one under", limit.MaxWeightGrams - 1, VehicleMotorcycle},
		{"exactly at", limit.MaxWeightGrams, VehicleMotorcycle},
		{"one over", limit.MaxWeightGrams + 1, VehicleCar},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			decision := decide(t, loadOf(testCase.weight, 1_000, 200, 1))

			if decision.SelectedClass != testCase.want {
				t.Fatalf("%d g selected %s, want %s",
					testCase.weight, decision.SelectedClass, testCase.want)
			}
		})
	}
}

func TestEngine_DimensionBoundaryIsExact(t *testing.T) {
	limit, _ := DefaultCapabilityPolicy().Lookup(VehicleMotorcycle)

	if got := decide(t, loadOf(1_000, 1_000, limit.MaxLongestDimensionMm, 1)).SelectedClass; got != VehicleMotorcycle {
		t.Fatalf("a package exactly at the dimension limit selected %s", got)
	}

	if got := decide(t, loadOf(1_000, 1_000, limit.MaxLongestDimensionMm+1, 1)).SelectedClass; got == VehicleMotorcycle {
		t.Fatal("a package one millimetre over the limit stayed on a motorcycle")
	}
}

func TestEngine_PackageCountBoundaryIsExact(t *testing.T) {
	limit, _ := DefaultCapabilityPolicy().Lookup(VehicleMotorcycle)

	if got := decide(t, loadOf(1_000, 1_000, 200, limit.MaxPackageCount)).SelectedClass; got != VehicleMotorcycle {
		t.Fatalf("exactly the package limit selected %s", got)
	}

	if got := decide(t, loadOf(1_000, 1_000, 200, limit.MaxPackageCount+1)).SelectedClass; got == VehicleMotorcycle {
		t.Fatal("one package over the limit stayed on a motorcycle")
	}
}

// THE DETERMINISM INVARIANT.
//
// The same inputs must always produce the same decision, including the order
// of the candidates and the supporting reasons. A snapshot that varies between
// two evaluations of the same load cannot be used to detect that a replan
// changed nothing.
func TestEngine_IsDeterministic(t *testing.T) {
	requirement := loadOf(120_000, 300_000, 1_400, 20)
	requirement.Handling.Capabilities = []HandlingCapability{
		CapabilityFragileCarry, CapabilityUprightCarry, CapabilityInsulated,
	}
	requirement.VehicleHints = []string{"CAR", "MOTORCYCLE"}

	request := SelectionRequest{
		Requirement:    requirement,
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: everyServiceEnabled(),
		Now:            fixedNow,
	}

	first, err := EvaluateAndSnapshot(request)
	if err != nil {
		t.Fatalf("first evaluation: %v", err)
	}

	for attempt := 0; attempt < 25; attempt++ {
		next, nextErr := EvaluateAndSnapshot(request)
		if nextErr != nil {
			t.Fatalf("evaluation %d: %v", attempt, nextErr)
		}

		if next.Fingerprint != first.Fingerprint {
			t.Fatalf("evaluation %d produced a different fingerprint", attempt)
		}

		if next.Decision.SelectedClass != first.Decision.SelectedClass {
			t.Fatalf("evaluation %d selected a different class", attempt)
		}

		if len(next.Decision.Candidates) != len(first.Decision.Candidates) {
			t.Fatalf("evaluation %d evaluated a different number of classes",
				attempt)
		}

		for index := range next.Decision.Candidates {
			if next.Decision.Candidates[index] != first.Decision.Candidates[index] {
				t.Fatalf("evaluation %d reordered candidate %d", attempt, index)
			}
		}
	}
}

// Two different decisions must not share a fingerprint, or a replan would
// conclude nothing changed.
func TestEngine_FingerprintDistinguishesDecisions(t *testing.T) {
	light, err := EvaluateAndSnapshot(SelectionRequest{
		Requirement:    loadOf(3_000, 5_000, 300, 2),
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: everyServiceEnabled(),
		Now:            fixedNow,
	})
	if err != nil {
		t.Fatalf("light: %v", err)
	}

	heavy, err := EvaluateAndSnapshot(SelectionRequest{
		Requirement:    loadOf(300_000, 500_000, 2_000, 40),
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: everyServiceEnabled(),
		Now:            fixedNow,
	})
	if err != nil {
		t.Fatalf("heavy: %v", err)
	}

	if !light.Changed(heavy) {
		t.Fatal("a motorcycle decision and a van decision share a fingerprint")
	}

	reason, replan := heavy.ReplanReason(light)
	if !replan {
		t.Fatal("a class change did not justify a replan")
	}

	if reason != "VEHICLE_OVERFLOW" {
		t.Fatalf("replan reason %q, want VEHICLE_OVERFLOW", reason)
	}
}

// Re-evaluating an unchanged load must not manufacture a replan.
func TestEngine_UnchangedDecisionDoesNotJustifyAReplan(t *testing.T) {
	request := SelectionRequest{
		Requirement:    loadOf(3_000, 5_000, 300, 2),
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: everyServiceEnabled(),
		Now:            fixedNow,
	}

	first, _ := EvaluateAndSnapshot(request)

	request.Now = fixedNow.Add(4 * time.Hour)
	second, _ := EvaluateAndSnapshot(request)

	if second.Changed(first) {
		t.Fatal("the same decision at a later time looks like a change")
	}

	if _, replan := second.ReplanReason(first); replan {
		t.Fatal("an unchanged decision produced a replan reason")
	}
}

// THE OVERFLOW INVARIANT.
//
// A wrapped weight is a negative weight, and a negative weight fits on
// anything. Implausible measurements are refused, never wrapped.
func TestEngine_ImplausibleMeasurementsAreRefused(t *testing.T) {
	requirement := loadOf(1_000, 1_000, 200, 1)
	requirement.Estimated.TotalWeightGrams = MaxPlausibleWeightGrams + 1

	if _, err := Evaluate(SelectionRequest{
		Requirement:    requirement,
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: everyServiceEnabled(),
	}); !errors.Is(err, ErrRequirementInvalid) {
		t.Fatalf("an implausible weight was accepted: %v", err)
	}
}

func TestEngine_PackingArithmeticCannotOverflow(t *testing.T) {
	_, _, err := ConfirmFromPacking([]PackedParcel{
		{
			WeightGrams: 1,
			LengthMm:    1 << 40,
			WidthMm:     1 << 40,
			HeightMm:    1 << 40,
		},
	})

	if err == nil {
		t.Fatal("a bounding box that overflows int64 was accepted")
	}
}

// Physically meaningless input is a caller fault, refused before any vehicle
// is considered.
func TestEngine_InvalidInputIsRefused(t *testing.T) {
	cases := map[string]func(*DeliveryRequirement){
		"negative weight": func(r *DeliveryRequirement) {
			r.Estimated.TotalWeightGrams = -1
		},
		"negative volume": func(r *DeliveryRequirement) {
			r.Estimated.TotalVolumeMl = -1
		},
		"negative dimension": func(r *DeliveryRequirement) {
			r.Estimated.LongestDimensionMm = -1
		},
		"negative package count": func(r *DeliveryRequirement) {
			r.Estimated.PackageCount = -1
		},
		"negative distance": func(r *DeliveryRequirement) {
			r.Route.DistanceMeters = -1
		},
		"negative pickup count": func(r *DeliveryRequirement) {
			r.Route.PickupCount = -1
		},
		"unknown risk class": func(r *DeliveryRequirement) {
			r.Value.RiskClass = "PRICELESS"
		},
		"unknown handling capability": func(r *DeliveryRequirement) {
			r.Handling.Capabilities = []HandlingCapability{"TELEPORT"}
		},
		"unknown special handling": func(r *DeliveryRequirement) {
			r.Handling.SpecialHandling = []SpecialHandling{"SENTIENT"}
		},
		"value without currency": func(r *DeliveryRequirement) {
			r.Value.DeclaredValueMinor = 5_000
		},
		"one package heavier than the load": func(r *DeliveryRequirement) {
			r.Estimated.TotalWeightGrams = 1_000
			r.Estimated.MaxSinglePackageWeightGrams = 5_000
		},
		"negative transit estimate": func(r *DeliveryRequirement) {
			r.Timing.EstimatedTransitMinutes = -1
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			requirement := loadOf(1_000, 1_000, 200, 1)
			mutate(&requirement)

			if _, err := Evaluate(SelectionRequest{
				Requirement:    requirement,
				Policy:         DefaultCapabilityPolicy(),
				Serviceability: everyServiceEnabled(),
			}); !errors.Is(err, ErrRequirementInvalid) {
				t.Fatalf("accepted %s: %v", name, err)
			}
		})
	}
}

// A ready window that closes before it opens, and a promise due before the
// goods exist, are both invalid statements rather than tight schedules.
func TestEngine_ImpossibleTimingIsRefused(t *testing.T) {
	requirement := loadOf(1_000, 1_000, 200, 1)

	from := fixedNow.Add(2 * time.Hour)
	until := fixedNow.Add(time.Hour)
	requirement.Timing = TimingProfile{ReadyFrom: &from, ReadyUntil: &until}

	if err := requirement.Validate(); !errors.Is(err, ErrRequirementInvalid) {
		t.Fatal("an inverted ready window was accepted")
	}

	promised := fixedNow
	requirement.Timing = TimingProfile{ReadyFrom: &from, PromisedBy: &promised}

	if err := requirement.Validate(); !errors.Is(err, ErrRequirementInvalid) {
		t.Fatal("a delivery promised before collection was accepted")
	}
}

// -----------------------------------------------------------------------------
// Snapshot integrity
// -----------------------------------------------------------------------------

// Every reason on a stored decision must be countable. A free-text reason is
// not evidence.
func TestSnapshot_OnlyStoresControlledReasonCodes(t *testing.T) {
	requirements := []DeliveryRequirement{
		loadOf(3_000, 5_000, 300, 2),
		loadOf(120_000, 300_000, 1_400, 20),
		loadOf(500_000_000, 500_000_000, 90_000, 90_000),
	}

	conflicted := loadOf(2_000, 4_000, 250, 2)
	conflicted.Handling.Capabilities = []HandlingCapability{
		CapabilityHotHold, CapabilityFrozenChain,
	}
	requirements = append(requirements, conflicted)

	for _, requirement := range requirements {
		snapshot, err := EvaluateAndSnapshot(SelectionRequest{
			Requirement:    requirement,
			Policy:         DefaultCapabilityPolicy(),
			Serviceability: everyServiceEnabled(),
			Now:            fixedNow,
		})
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}

		if !KnownReasonCode(snapshot.Decision.PrimaryReason) {
			t.Fatalf("uncontrolled primary reason %q",
				snapshot.Decision.PrimaryReason)
		}

		for _, reason := range snapshot.Decision.SupportingReasons {
			if !KnownReasonCode(reason) {
				t.Fatalf("uncontrolled supporting reason %q", reason)
			}
		}

		for _, candidate := range snapshot.Decision.Candidates {
			if !KnownReasonCode(candidate.ReasonCode) {
				t.Fatalf("uncontrolled candidate reason %q",
					candidate.ReasonCode)
			}
		}
	}
}

// A stored decision must replay from stored evidence alone, with no live
// service reachable and no dependence on the policy currently deployed.
func TestSnapshot_ReplaysFromStoredEvidence(t *testing.T) {
	requirement := loadOf(120_000, 300_000, 1_400, 20)

	original, err := EvaluateAndSnapshot(SelectionRequest{
		Requirement:    requirement,
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: everyServiceEnabled(),
		Now:            fixedNow,
	})
	if err != nil {
		t.Fatalf("original: %v", err)
	}

	payload, err := original.MarshalEvidence()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	restored, err := UnmarshalSnapshot(payload)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Replay uses the EMBEDDED policy, not today's. A snapshot that could only
	// be replayed against current configuration explains nothing the day the
	// configuration changes.
	replayed, err := Evaluate(SelectionRequest{
		Requirement:    restored.Requirement,
		Policy:         restored.Policy,
		Serviceability: restored.Serviceability,
		Now:            restored.Decision.EvaluatedAt,
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}

	if replayed.SelectedClass != original.Decision.SelectedClass {
		t.Fatalf("replay selected %s, original selected %s",
			replayed.SelectedClass, original.Decision.SelectedClass)
	}

	if Snapshot(SelectionRequest{
		Requirement:    restored.Requirement,
		Policy:         restored.Policy,
		Serviceability: restored.Serviceability,
	}, replayed).Fingerprint != original.Fingerprint {
		t.Fatal("a replayed decision does not fingerprint identically")
	}
}

// A decision taken under one policy version must keep naming that version even
// after the thresholds change.
func TestSnapshot_PreservesTheHistoricalPolicy(t *testing.T) {
	historical := DefaultCapabilityPolicy()
	historical.Version = "vehicle-capability/legacy-test"

	// Under the historical rules a motorcycle carried 60 kg.
	for index := range historical.Classes {
		if historical.Classes[index].Class == VehicleMotorcycle {
			historical.Classes[index].MaxWeightGrams = 60_000
		}
	}

	snapshot, err := EvaluateAndSnapshot(SelectionRequest{
		Requirement:    loadOf(40_000, 5_000, 300, 2),
		Policy:         historical,
		Serviceability: everyServiceEnabled(),
		Now:            fixedNow,
	})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	if snapshot.Decision.SelectedClass != VehicleMotorcycle {
		t.Fatalf("under the historical policy a 40 kg load selected %s",
			snapshot.Decision.SelectedClass)
	}

	if snapshot.Decision.PolicyVersion != "vehicle-capability/legacy-test" {
		t.Fatalf("policy version %q was not preserved",
			snapshot.Decision.PolicyVersion)
	}

	// Today's rules would answer differently, which is exactly why the old
	// answer has to carry its own rules with it.
	today := decide(t, loadOf(40_000, 5_000, 300, 2))
	if today.SelectedClass == VehicleMotorcycle {
		t.Fatal("the current policy still allows 40 kg on a motorcycle; the test no longer proves anything")
	}

	if capability, ok := snapshot.Policy.Lookup(VehicleMotorcycle); !ok ||
		capability.MaxWeightGrams != 60_000 {
		t.Fatal("the snapshot did not embed the thresholds it decided under")
	}
}

// A snapshot naming a class with no service, or a service with no class, is a
// mission nobody owns.
func TestSnapshot_RejectsAHalfWrittenDecision(t *testing.T) {
	snapshot, err := EvaluateAndSnapshot(SelectionRequest{
		Requirement:    loadOf(3_000, 5_000, 300, 2),
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: everyServiceEnabled(),
		Now:            fixedNow,
	})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	broken := snapshot
	broken.Decision.SelectedService = ""

	if err = broken.Validate(); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("a decision naming a class but no service was accepted")
	}

	broken = snapshot
	broken.Decision.Executable = false

	if err = broken.Validate(); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatal("a refusal still naming a selected class was accepted")
	}
}

// Attaching a snapshot to a plan must not change what it says. The identifiers
// are bookkeeping; the decision is evidence.
func TestSnapshot_AttachDoesNotChangeTheDecision(t *testing.T) {
	snapshot, err := EvaluateAndSnapshot(SelectionRequest{
		Requirement:    loadOf(3_000, 5_000, 300, 2),
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: everyServiceEnabled(),
		Now:            fixedNow,
	})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	attached := snapshot.Attach("cfg_1", "cfpv_1", "slot-a")

	if attached.Fingerprint != snapshot.Fingerprint {
		t.Fatal("binding a snapshot to a plan changed its fingerprint")
	}

	if attached.SlotRef != "slot-a" || attached.PlanID != "cfpv_1" {
		t.Fatal("attach did not record the plan binding")
	}
}

// -----------------------------------------------------------------------------
// Provider supply can never reach the engine
// -----------------------------------------------------------------------------

// THE NO-DOWNGRADE-ON-SUPPLY INVARIANT.
//
// Provider scarcity must never cause a smaller, unsafe vehicle to be chosen.
// The structural guarantee is that the engine has nowhere to put a provider
// count: this test pins the shape, because a future field would be the first
// step toward "no vans available, try a bike".
func TestEngine_HasNoProviderSupplyInput(t *testing.T) {
	// A van-sized load with hauling switched off must be refused, NOT demoted
	// to the motorcycle that is still available.
	serviceability := ServiceabilityFrom(map[Service]bool{
		ServiceDispatch: true,
		ServiceHauling:  false,
	}, fixedNow)

	decision := decideWith(t, loadOf(300_000, 2_000_000, 2_000, 60), serviceability)

	if decision.Selected() {
		t.Fatalf("a van-sized load was assigned to %s because hauling was unavailable",
			decision.SelectedClass)
	}

	if decision.PrimaryReason != ReasonNoExecutableService {
		t.Fatalf("refused for %q, want NO_SUITABLE_EXECUTABLE_SERVICE",
			decision.PrimaryReason)
	}

	// The physical requirement is unchanged by the outage. Saying otherwise
	// would be the platform revising physics to match its supply.
	if decision.MinimumPhysicalClass != VehicleVan {
		t.Fatalf("minimum physical class became %s during an outage",
			decision.MinimumPhysicalClass)
	}
}

// Catering and Manufacturer loads reach the same selection engine as every
// other vendor's. The engine never sees a vendor category: it sees weight,
// volume, dimension and handling, which is why a caterer's tray stack and a
// factory's pallet get the vehicle the physics require rather than the one
// their business type suggests.
func TestCateringBulkLoadEscalatesBeyondAMotorcycle(t *testing.T) {
	// A modest catering run: trays for fifty covers.
	decision := decide(t, loadOf(45_000, 120_000, 900, 12))

	if decision.SelectedClass == VehicleMotorcycle {
		t.Fatal("a catering tray load was assigned to a motorcycle")
	}
	if !decision.Selected() {
		t.Fatalf("catering load refused entirely: %s", decision.PrimaryReason)
	}
	if decision.SelectedService == "" {
		t.Fatal("no authority selected for a catering load")
	}
}

func TestManufacturerPalletLoadSelectsHaulage(t *testing.T) {
	// Palletised factory output: beyond any van.
	decision := decide(t, loadOf(850_000, 2_400_000, 1_200, 4))

	if decision.SelectedService != ServiceHauling {
		t.Fatalf("pallet shipment selected %s/%s, want the HAULING authority",
			decision.SelectedClass, decision.SelectedService)
	}
}

// A manufacturer is not condemned to a truck: a small shipment that genuinely
// fits a smaller authority gets one, because the requirement decides.
func TestManufacturerSmallShipmentUsesASmallerAuthority(t *testing.T) {
	decision := decide(t, loadOf(3_000, 8_000, 250, 1))

	if decision.SelectedService == ServiceHauling {
		t.Fatal("a 3 kg shipment was sent to Haulage because of who made it")
	}
	if !decision.Selected() {
		t.Fatalf("small manufacturer shipment refused: %s", decision.PrimaryReason)
	}
}
