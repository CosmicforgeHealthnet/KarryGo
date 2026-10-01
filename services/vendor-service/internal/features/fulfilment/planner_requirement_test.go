package fulfilment

// Planner integration for the delivery-requirement engine.
//
// The engine is exercised on its own in vehicle_selector_test.go. These tests
// pin the WIRING: that the engine runs before the delivery mode is chosen,
// that its answer reaches the vendor's own policy evaluation, and that the
// evidence it produces ends up on the plan.
//
// Two of the overflow triggers below were previously unreachable. The delivery
// policy evaluator had always accepted a RequiredVehicleClass and an SLA pair,
// and the planner had never populated either - so VEHICLE_OVERFLOW and
// SLA_OVERFLOW could not fire however a vendor configured its policy. That is
// what these tests exist to keep fixed.

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
)

// hybridPolicy is a vendor that would deliver its own orders and is willing to
// fall back to the platform.
func hybridPolicy() delivery.Policy {
	return delivery.Policy{
		Mode:              delivery.ModeHybrid,
		FeeRule:           delivery.FeeFlat,
		Enabled:           true,
		Currency:          "NGN",
		BaseFeeMinor:      1000,
		OperatingTimezone: "Africa/Lagos",
		RiderAvailability: delivery.RiderAvailable,
		Version:           7,
	}
}

// Every plan now carries the evidence behind its vehicle choice. A plan that
// records a van without recording why is a delivery nobody can explain.
func TestPlanner_AttachesRequirementEvidenceToEveryPlannedGroup(t *testing.T) {
	harness := newHarness()

	plan, _, err := harness.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusPlanned || len(plan.Groups) != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}

	snapshot := plan.Groups[0].RequirementSnapshot
	if snapshot == nil {
		t.Fatal("a planned group carries no requirement evidence")
	}

	if snapshot.Decision.SelectedClass != plan.Groups[0].VehicleClass {
		t.Fatalf("evidence says %s, plan says %s",
			snapshot.Decision.SelectedClass, plan.Groups[0].VehicleClass)
	}

	if snapshot.Fingerprint == "" || snapshot.Policy.Version == "" {
		t.Fatal("evidence is not replayable: missing fingerprint or policy")
	}

	if !KnownReasonCode(snapshot.Decision.PrimaryReason) {
		t.Fatalf("uncontrolled reason %q reached a plan",
			snapshot.Decision.PrimaryReason)
	}
}

// A group refused before any vehicle was considered carries no evidence,
// because no vehicle question was ever asked. Fabricating a decision there
// would be evidence of something that did not happen.
func TestPlanner_RefusedBranchCarriesNoRequirementEvidence(t *testing.T) {
	harness := newHarness()
	harness.branches.branches = map[string]Branch{}

	plan, _, err := harness.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusUnserviceable {
		t.Fatalf("an unknown branch was planned: %+v", plan)
	}

	if plan.Groups[0].RequirementSnapshot != nil {
		t.Fatal("a group refused before vehicle selection carries a decision")
	}
}

// THE VEHICLE_OVERFLOW WIRING.
//
// A vendor whose own fleet is motorcycles only cannot carry a van-sized load.
// The overflow to the platform must fire with the vehicle trigger, which is
// only possible because the requirement engine now runs first and hands the
// vendor policy the class the load actually needs.
func TestPlanner_VendorVehicleOverflowFallsBackToPlatform(t *testing.T) {
	harness := newHarness()
	harness.vendorOwn.enabled = true

	policy := hybridPolicy()
	policy.SupportedVehicleClasses = []string{"MOTORCYCLE"}
	harness.vendorOwn.policies["bizA|brA"] = policy

	// A load past every motorcycle limit, so the requirement engine answers
	// VAN and the vendor's motorcycle-only fleet cannot serve it.
	harness.logistics.profiles = map[string]LineProfile{
		"l1": {Known: true, WeightGrams: ptr64(300_000)},
	}

	plan, _, err := harness.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusPlanned || len(plan.Missions) != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}

	mission := plan.Missions[0]

	if mission.Authority != AuthorityPlatform {
		t.Fatalf("mission authority %s, want PLATFORM", mission.Authority)
	}

	if mission.FallbackTrigger != string(delivery.TriggerVehicle) {
		t.Fatalf("fallback trigger %q, want VEHICLE_OVERFLOW - the requirement engine's answer never reached the vendor policy",
			mission.FallbackTrigger)
	}

	if mission.VehicleClass != VehicleVan {
		t.Fatalf("platform fallback selected %s, want VAN", mission.VehicleClass)
	}
}

// A vendor whose fleet DOES cover the required class keeps the delivery. The
// overflow must not fire merely because the field is now populated.
func TestPlanner_VendorKeepsDeliveryWhenItsFleetCoversTheClass(t *testing.T) {
	harness := newHarness()
	harness.vendorOwn.enabled = true

	policy := hybridPolicy()
	policy.SupportedVehicleClasses = []string{"MOTORCYCLE", "VAN"}
	harness.vendorOwn.policies["bizA|brA"] = policy

	plan, _, err := harness.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusPlanned || len(plan.Missions) != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}

	if plan.Missions[0].Authority != AuthorityVendorOwn {
		t.Fatalf("mission authority %s, want VENDOR_OWN_DELIVERY - the vendor can carry this",
			plan.Missions[0].Authority)
	}
}

// THE SLA_OVERFLOW WIRING.
//
// A vendor that cannot reach the customer by the promised time overflows to
// the platform. This required both the promise and the vendor's own estimate
// to reach the policy evaluation; neither did before.
func TestPlanner_VendorSLAOverflowFallsBackToPlatform(t *testing.T) {
	harness := newHarness()
	harness.vendorOwn.enabled = true
	harness.vendorOwn.policies["bizA|brA"] = hybridPolicy()

	now := time.Now().UTC()
	promised := now.Add(20 * time.Minute)

	group := groupA()
	group.Timing = TimingProfile{
		PromisedBy: &promised,
		// The platform can make the trip inside the promise.
		EstimatedTransitMinutes: 12,
	}
	// The vendor's own delivery would take an hour and a half. That is the
	// whole SLA overflow condition: the vendor cannot keep a promise the
	// platform still can.
	group.VendorEstimateMinutes = 90

	plan, _, err := harness.planner(t).Build(
		context.Background(), request(group),
	)
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusPlanned || len(plan.Missions) != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}

	if plan.Missions[0].FallbackTrigger != string(delivery.TriggerSLA) {
		t.Fatalf("fallback trigger %q, want SLA_OVERFLOW - the promise never reached the vendor policy",
			plan.Missions[0].FallbackTrigger)
	}
}

// A stated promise the vendor can comfortably meet changes nothing.
func TestPlanner_AchievablePromiseDoesNotTriggerOverflow(t *testing.T) {
	harness := newHarness()
	harness.vendorOwn.enabled = true
	harness.vendorOwn.policies["bizA|brA"] = hybridPolicy()

	now := time.Now().UTC()
	promised := now.Add(4 * time.Hour)

	group := groupA()
	group.Timing = TimingProfile{
		PromisedBy:              &promised,
		EstimatedTransitMinutes: 30,
	}
	group.VendorEstimateMinutes = 40

	plan, _, err := harness.planner(t).Build(
		context.Background(), request(group),
	)
	if err != nil {
		t.Fatal(err)
	}

	if plan.Status != StatusPlanned || len(plan.Missions) != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}

	if plan.Missions[0].Authority != AuthorityVendorOwn {
		t.Fatalf("an achievable promise pushed the delivery to %s",
			plan.Missions[0].Authority)
	}
}

// promisedMaxMinutes must distinguish "no promise stated" from "already late".
// Collapsing them to zero would silently skip the SLA check for exactly the
// orders that have already missed it.
func TestPromisedMaxMinutes_DistinguishesNoPromiseFromOverdue(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	if got := promisedMaxMinutes(TimingProfile{}, now); got != 0 {
		t.Fatalf("no promise produced %d, want 0", got)
	}

	overdue := now.Add(-time.Hour)
	if got := promisedMaxMinutes(
		TimingProfile{PromisedBy: &overdue}, now,
	); got != 1 {
		t.Fatalf("an overdue promise produced %d, want 1 - zero would read as no promise",
			got)
	}

	future := now.Add(45 * time.Minute)
	if got := promisedMaxMinutes(
		TimingProfile{PromisedBy: &future}, now,
	); got < 45 || got > 46 {
		t.Fatalf("a 45 minute promise produced %d", got)
	}
}

// The vendor policy is told the MINIMUM PHYSICAL class, not the executable
// one. A vendor with its own car must not be told its fleet is inadequate
// because the platform cannot run a taxi.
func TestRequiredVehicleClass_ReportsThePhysicalMinimum(t *testing.T) {
	if got := requiredVehicleClass(nil); got != "" {
		t.Fatalf("a missing snapshot produced %q, want empty", got)
	}

	snapshot, err := EvaluateAndSnapshot(SelectionRequest{
		// Past every motorcycle limit, inside the car envelope.
		Requirement:    loadOf(120_000, 300_000, 1_400, 20),
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: everyServiceEnabled(),
		Now:            fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := requiredVehicleClass(&snapshot); got != string(VehicleCar) {
		t.Fatalf("vendor policy told %q, want CAR - the class the load actually needs",
			got)
	}

	if snapshot.Decision.SelectedClass != VehicleCar || snapshot.Decision.SelectedService != ServiceTaxi {
		t.Fatalf("executable answer = %s/%s, want CAR/TAXI", snapshot.Decision.SelectedClass, snapshot.Decision.SelectedService)
	}
}
