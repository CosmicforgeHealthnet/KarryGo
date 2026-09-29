package fulfilment

import "testing"

// Karryam V4 §14.6: every platform-delivered line states a packed weight per
// sell unit; quantity multiplies it; the smallest SAFE executable class wins;
// scarcity never downgrades.

func weighedLine(id string, quantity int64) LineLogistics {
	return LineLogistics{CheckoutLineID: id, Quantity: quantity}
}

func weightProfile(grams int64) LineProfile {
	return LineProfile{Known: true, WeightGrams: &grams}
}

func planDecision(t *testing.T, requirement Requirement, services map[Service]bool) VehicleDecision {
	t.Helper()
	decision, err := Evaluate(SelectionRequest{
		Requirement:    RequirementFrom(requirement),
		Policy:         DefaultCapabilityPolicy(),
		Serviceability: ServiceabilityFrom(services, fixedNow),
		Now:            fixedNow,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return decision
}

func allPlatformServices() map[Service]bool {
	return map[Service]bool{ServiceDispatch: true, ServiceTaxi: true, ServiceHauling: true}
}

func TestDefaultPolicyRequiresPackedWeight(t *testing.T) {
	policy := DefaultCapabilityPolicy()
	if policy.MissingMetadata != MissingMetadataRequirePackedWeight || policy.Version != "vehicle-capability/2" {
		t.Fatalf("default policy = %s/%s", policy.MissingMetadata, policy.Version)
	}
	if (CapabilityPolicy{MissingMetadata: "BOGUS"}).Normalized().MissingMetadata != MissingMetadataRequirePackedWeight {
		t.Fatal("an unknown policy did not fail closed")
	}
}

func TestMissingPackedWeightNeverPlansOnTheSmallestVehicle(t *testing.T) {
	light := int64(300)
	for name, requirement := range map[string]Requirement{
		// One line weighed, one not: a lower bound small enough for a bike.
		"partial": Aggregate("biz", "br",
			[]LineLogistics{weighedLine("a", 1), weighedLine("b", 1)},
			map[string]LineProfile{"a": {Known: true, WeightGrams: &light}, "b": {Known: true}}),
		// The Food package-size fallback is not a packed weight.
		"size-class fallback": Aggregate("biz", "br",
			[]LineLogistics{weighedLine("a", 1)},
			map[string]LineProfile{"a": {Known: true, PackageSizeClass: "SMALL"}}),
		// No logistics profile at all.
		"absent": Aggregate("biz", "br",
			[]LineLogistics{weighedLine("a", 2)}, map[string]LineProfile{}),
	} {
		decision := planDecision(t, requirement, allPlatformServices())
		if decision.Selected() || decision.PrimaryReason != ReasonMetadataIncomplete {
			t.Fatalf("%s: selected=%v class=%s reason=%s", name, decision.Selected(), decision.SelectedClass, decision.PrimaryReason)
		}
		if !hasReason(decision, ReasonWeakEvidence) {
			t.Fatalf("%s: refusal does not record weak evidence", name)
		}
	}
}

func TestWeightOnlyProfilePlans(t *testing.T) {
	// Volume and dimensions are vendor-declared where material; their absence
	// alone does not refuse a weighed load.
	requirement := Aggregate("biz", "br", []LineLogistics{weighedLine("a", 2)},
		map[string]LineProfile{"a": weightProfile(500)})
	decision := planDecision(t, requirement, allPlatformServices())
	if !decision.Selected() || decision.SelectedClass != VehicleMotorcycle {
		t.Fatalf("weighed load: selected=%v class=%s reason=%s", decision.Selected(), decision.SelectedClass, decision.PrimaryReason)
	}
}

func TestQuantityEscalatesTheSameProduct(t *testing.T) {
	for _, testCase := range []struct {
		quantity int64
		want     VehicleClass
	}{
		{quantity: 1, want: VehicleMotorcycle},  // 2 kg
		{quantity: 12, want: VehicleMotorcycle}, // 24 kg, within the 25 kg bike
		{quantity: 20, want: VehicleCar},        // 40 kg
		{quantity: 200, want: VehicleVan},       // 400 kg
	} {
		requirement := Aggregate("biz", "br", []LineLogistics{weighedLine("a", testCase.quantity)},
			map[string]LineProfile{"a": weightProfile(2000)})
		decision := planDecision(t, requirement, allPlatformServices())
		if !decision.Selected() || decision.SelectedClass != testCase.want {
			t.Fatalf("quantity %d: class=%s want %s (reason %s)", testCase.quantity, decision.SelectedClass, testCase.want, decision.PrimaryReason)
		}
	}
}

func TestUnavailableCorrectClassEscalatesNeverDowngrades(t *testing.T) {
	// 40 kg needs a car. With Taxi Goods unavailable the only safe answer is a
	// larger class; the available motorcycle is never chosen.
	requirement := Aggregate("biz", "br", []LineLogistics{weighedLine("a", 20)},
		map[string]LineProfile{"a": weightProfile(2000)})
	decision := planDecision(t, requirement, map[Service]bool{ServiceDispatch: true, ServiceHauling: true})
	if !decision.Selected() || decision.SelectedClass == VehicleMotorcycle {
		t.Fatalf("car unavailable: class=%s reason=%s", decision.SelectedClass, decision.PrimaryReason)
	}
	if decision.SelectedClass != VehicleVan {
		t.Fatalf("expected escalation to the van, got %s", decision.SelectedClass)
	}
	// With only the motorcycle available the load is refused, not squeezed.
	decision = planDecision(t, requirement, map[Service]bool{ServiceDispatch: true})
	if decision.Selected() {
		t.Fatalf("unsafe downgrade to %s", decision.SelectedClass)
	}
}
