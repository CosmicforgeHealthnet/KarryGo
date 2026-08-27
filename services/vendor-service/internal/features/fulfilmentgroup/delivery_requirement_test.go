package fulfilmentgroup

import (
	"errors"
	"testing"
)

// validDecision is a well-formed executed decision. Each test breaks exactly
// one thing about it, so a failure names the rule that broke rather than
// leaving several candidates.
func validDecision() RequirementDecision {
	return RequirementDecision{
		SlotRef:              "slot-a",
		BusinessID:           "biz_1",
		BranchID:             "br_1",
		MinimumPhysicalClass: "MOTORCYCLE",
		SelectedVehicleClass: "MOTORCYCLE",
		SelectedService:      "DISPATCH",
		Executable:           true,
		PrimaryReason:        "VEHICLE_SELECTED",
		MeasurementState:     "ESTIMATED",
		TotalWeightGrams:     3_000,
		PackageCount:         2,
		PickupCount:          1,
		PolicyVersion:        "vehicle-capability/1",
		Fingerprint:          "abc123",
	}
}

func TestRequirementDecision_AcceptsAWellFormedDecision(t *testing.T) {
	if err := validDecision().Validate(); err != nil {
		t.Fatalf("a well-formed decision was rejected: %v", err)
	}
}

// A stored class with no service is a mission nobody owns; a refusal still
// naming a class claims a delivery that did not happen.
func TestRequirementDecision_RejectsAHalfWrittenDecision(t *testing.T) {
	cases := map[string]func(*RequirementDecision){
		"class without service": func(d *RequirementDecision) {
			d.SelectedService = ""
		},
		"service without class": func(d *RequirementDecision) {
			d.SelectedVehicleClass = ""
		},
		"refusal still naming a class": func(d *RequirementDecision) {
			d.Executable = false
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			decision := validDecision()
			mutate(&decision)

			if err := decision.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("accepted %s: %v", name, err)
			}
		})
	}
}

// THE TAXI GATE, restated where a plan is written.
//
// driver-taxi-service has no goods-delivery mode. A stored row claiming a car
// executed a delivery would be a delivery nobody performed, and it would be
// believed by everyone who read it afterwards.
func TestRequirementDecision_RefusesAnExecutedCar(t *testing.T) {
	decision := validDecision()
	decision.SelectedVehicleClass = "CAR"

	if err := decision.Validate(); !errors.Is(err, ErrConflict) {
		t.Fatalf("a car was accepted as an executed class: %v", err)
	}
}

// A car as the PHYSICAL requirement is legitimate and must stay storable:
// losing it would hide the difference between an oversized load and a
// disabled integration.
func TestRequirementDecision_AllowsACarAsThePhysicalMinimum(t *testing.T) {
	decision := validDecision()
	decision.MinimumPhysicalClass = "CAR"
	decision.SelectedVehicleClass = "VAN"
	decision.SelectedService = "HAULING"

	if err := decision.Validate(); err != nil {
		t.Fatalf("a car minimum with a van execution was rejected: %v", err)
	}

	if !decision.Escalated() {
		t.Fatal("a van executing a car-class requirement is an escalation")
	}
}

func TestRequirementDecision_RejectsImpossibleMeasurements(t *testing.T) {
	cases := map[string]func(*RequirementDecision){
		"negative weight":     func(d *RequirementDecision) { d.TotalWeightGrams = -1 },
		"negative volume":     func(d *RequirementDecision) { d.TotalVolumeMl = -1 },
		"negative dimension":  func(d *RequirementDecision) { d.LongestDimensionMm = -1 },
		"negative packages":   func(d *RequirementDecision) { d.PackageCount = -1 },
		"negative distance":   func(d *RequirementDecision) { d.RouteDistanceMeters = -1 },
		"negative pickups":    func(d *RequirementDecision) { d.PickupCount = -1 },
		"missing reason":      func(d *RequirementDecision) { d.PrimaryReason = "" },
		"missing fingerprint": func(d *RequirementDecision) { d.Fingerprint = "" },
		"missing policy":      func(d *RequirementDecision) { d.PolicyVersion = "" },
		"value without currency": func(d *RequirementDecision) {
			d.DeclaredValueMinor = 1_000
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			decision := validDecision()
			mutate(&decision)

			if err := decision.Validate(); err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Evidence is validated with the plan it justifies
// -----------------------------------------------------------------------------

func planWithDecisions(decisions ...RequirementDecision) PlanInput {
	return PlanInput{
		IdempotencyKey: "key-1",
		Reason:         ReasonInitialPlan,
		DeliveryMode:   ModePlatform,
		Missions: []PlanMission{
			{
				SlotRef:   "slot-a",
				Sequence:  1,
				Owner:     OwnerPlatform,
				Authority: AuthorityDispatch,
				Stops: []Stop{
					{Sequence: 1, Type: StopPickup, UnitID: "unit-1"},
					{Sequence: 2, Type: StopDropoff},
				},
			},
		},
		RequirementDecisions: decisions,
	}
}

func TestPlanInput_AcceptsEvidenceForItsOwnSlots(t *testing.T) {
	if err := planWithDecisions(validDecision()).Validate(); err != nil {
		t.Fatalf("evidence for an existing slot was rejected: %v", err)
	}
}

// Evidence for a slot the plan does not contain would explain a mission that
// does not exist while the one that does stays unexplained.
func TestPlanInput_RejectsEvidenceForAnAbsentSlot(t *testing.T) {
	decision := validDecision()
	decision.SlotRef = "slot-nowhere"

	if err := planWithDecisions(decision).Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("evidence for a nonexistent slot was accepted: %v", err)
	}
}

// Two decisions for one slot is not evidence, it is a contradiction about the
// same mission.
func TestPlanInput_RejectsTwoDecisionsForOneSlot(t *testing.T) {
	first := validDecision()

	second := validDecision()
	second.SelectedVehicleClass = "VAN"
	second.SelectedService = "HAULING"

	if err := planWithDecisions(first, second).Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("two decisions for one slot were accepted: %v", err)
	}
}

// A malformed decision must fail the plan before it commits. Rejecting it
// afterwards would leave a plan version nobody can explain.
func TestPlanInput_RejectsMalformedEvidence(t *testing.T) {
	decision := validDecision()
	decision.Fingerprint = ""

	if err := planWithDecisions(decision).Validate(); err == nil {
		t.Fatal("a plan carrying malformed evidence was accepted")
	}
}

// A plan with no evidence stays valid. Evidence is an explanation, and a
// deployment that has not started producing it must not lose the ability to
// plan deliveries.
func TestPlanInput_EvidenceIsOptional(t *testing.T) {
	if err := planWithDecisions().Validate(); err != nil {
		t.Fatalf("a plan without evidence was rejected: %v", err)
	}
}
