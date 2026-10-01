package main

import (
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

var evidenceNow = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

func evidenceFor(
	t *testing.T, plans []orders.MissionPlan,
) []struct {
	Slot       string
	Class      string
	Service    string
	Executable bool
} {
	t.Helper()

	decisions := requirementEvidenceFor(
		plans, fulfilment.CapabilityPolicy{}, evidenceNow,
	)

	out := make([]struct {
		Slot       string
		Class      string
		Service    string
		Executable bool
	}, 0, len(decisions))

	for _, decision := range decisions {
		if err := decision.Validate(); err != nil {
			t.Fatalf("produced evidence that cannot be stored: %v", err)
		}

		out = append(out, struct {
			Slot       string
			Class      string
			Service    string
			Executable bool
		}{
			Slot:       decision.SlotRef,
			Class:      decision.SelectedVehicleClass,
			Service:    decision.SelectedService,
			Executable: decision.Executable,
		})
	}

	return out
}

// Ordinary case: the evidence agrees with the plan and names the slot it
// explains.
func TestRequirementEvidence_ExplainsTheSlotItBelongsTo(t *testing.T) {
	evidence := evidenceFor(t, []orders.MissionPlan{
		{
			PlanMissionRef: "pm-1",
			Service:        orders.LogisticsDispatch,
			VehicleClass:   "MOTORCYCLE",
			WeightGrams:    3_000,
			PackageCount:   2,
			PickupBusiness: "biz_1",
			PickupBranch:   "br_1",
		},
	})

	if len(evidence) != 1 {
		t.Fatalf("produced %d decisions for one mission", len(evidence))
	}

	if evidence[0].Slot != "pm-1" {
		t.Fatalf("evidence filed against slot %q, want pm-1", evidence[0].Slot)
	}

	if evidence[0].Class != "MOTORCYCLE" {
		t.Fatalf("evidence says %q, plan says MOTORCYCLE", evidence[0].Class)
	}
}

// THE NO-UNSAFE-DOWNGRADE INVARIANT AT ACTIVATION.
//
// By activation the order carries weight and package count but not the
// handling metadata behind the checkout decision. Re-deriving a class from
// measurements alone could therefore answer MOTORCYCLE for a load correctly
// sent by van because it was frozen.
//
// The committed class is a FLOOR. Evidence may never describe a smaller
// vehicle than the plan actually uses.
func TestRequirementEvidence_NeverDescribesASmallerVehicleThanThePlan(t *testing.T) {
	// A light load the measurements alone would put on a motorcycle, which
	// checkout committed to a van - the frozen-goods case.
	evidence := evidenceFor(t, []orders.MissionPlan{
		{
			PlanMissionRef: "pm-frozen",
			Service:        orders.LogisticsHauling,
			VehicleClass:   "VAN",
			WeightGrams:    2_000,
			PackageCount:   1,
			PickupBusiness: "biz_1",
			PickupBranch:   "br_1",
		},
	})

	if len(evidence) != 1 {
		t.Fatalf("produced %d decisions", len(evidence))
	}

	if evidence[0].Class != "VAN" {
		t.Fatalf("evidence downgraded the plan to %q; the committed class is a floor",
			evidence[0].Class)
	}

	if evidence[0].Service != "HAULING" {
		t.Fatalf("evidence routed to %q, want HAULING", evidence[0].Service)
	}
}

// Measurements that outgrew the committed class must still be able to
// escalate: a floor is a minimum, not a ceiling.
func TestRequirementEvidence_MayEscalateAboveTheCommittedClass(t *testing.T) {
	// Committed to a motorcycle, but the recorded weight is a van load. The
	// evidence must not claim a motorcycle carried it.
	evidence := evidenceFor(t, []orders.MissionPlan{
		{
			PlanMissionRef: "pm-heavy",
			Service:        orders.LogisticsDispatch,
			VehicleClass:   "MOTORCYCLE",
			WeightGrams:    400_000,
			PackageCount:   40,
			PickupBusiness: "biz_1",
			PickupBranch:   "br_1",
		},
	})

	// The engine escalates to VAN, which contradicts the plan's MOTORCYCLE, so
	// no evidence is filed at all rather than evidence that disagrees with the
	// delivery it describes.
	if len(evidence) != 0 {
		t.Fatalf("filed %d contradictory decisions; expected none", len(evidence))
	}
}

// Evidence is an explanation, not a precondition. An order with no missions
// produces none, and nothing fails.
func TestRequirementEvidence_EmptyPlanProducesNothing(t *testing.T) {
	if got := requirementEvidenceFor(
		nil, fulfilment.CapabilityPolicy{}, evidenceNow,
	); len(got) != 0 {
		t.Fatalf("produced %d decisions for an empty plan", len(got))
	}
}

// Every mission gets its own decision, keyed to its own slot.
func TestRequirementEvidence_OneDecisionPerMission(t *testing.T) {
	evidence := evidenceFor(t, []orders.MissionPlan{
		{
			PlanMissionRef: "pm-1", Service: orders.LogisticsDispatch,
			VehicleClass: "MOTORCYCLE", WeightGrams: 3_000, PackageCount: 2,
		},
		{
			PlanMissionRef: "pm-2", Service: orders.LogisticsHauling,
			VehicleClass: "VAN", WeightGrams: 300_000, PackageCount: 40,
		},
	})

	if len(evidence) != 2 {
		t.Fatalf("produced %d decisions for two missions", len(evidence))
	}

	slots := map[string]bool{}
	for _, decision := range evidence {
		if slots[decision.Slot] {
			t.Fatalf("two decisions filed against slot %q", decision.Slot)
		}

		slots[decision.Slot] = true
	}
}

// A car appears as the executed class only when Taxi Goods executes it. A plan
// claiming another authority drove a car gets no stored evidence, because the
// evidence would disagree with the plan it explains.
func TestRequirementEvidence_NeverFilesAnExecutedCar(t *testing.T) {
	taxi := evidenceFor(t, []orders.MissionPlan{{
		PlanMissionRef: "pm-taxi", Service: orders.LogisticsTaxi,
		VehicleClass: "CAR", WeightGrams: 40_000, PackageCount: 4,
	}})
	if len(taxi) != 1 || taxi[0].Class != "CAR" {
		t.Fatalf("Taxi Goods car evidence = %+v", taxi)
	}

	evidence := evidenceFor(t, []orders.MissionPlan{
		{
			PlanMissionRef: "pm-car",
			Service:        orders.LogisticsHauling,
			VehicleClass:   "CAR",
			WeightGrams:    120_000,
			PackageCount:   20,
		},
	})

	for _, decision := range evidence {
		if decision.Class == "CAR" {
			t.Fatal("stored evidence claims a car executed a delivery")
		}
	}
}
