package fulfilment

import (
	"errors"
	"testing"
)

func allServices() map[Service]bool {
	return map[Service]bool{
		ServiceDispatch: true,
		ServiceHauling:  true,
	}
}

func selectFor(t *testing.T, requirement Requirement) Selection {
	t.Helper()

	selection, err := SelectVehicle(
		requirement, DefaultCapabilities(), allServices(),
	)
	if err != nil {
		t.Fatalf("expected a selection, got %v (%s)", err, selection.ReasonCode)
	}

	return selection
}

func TestSelectVehicle_SmallLoadGoesByMotorcycle(t *testing.T) {
	selection := selectFor(t, Requirement{
		PackageCount:     2,
		TotalWeightGrams: 3000,
		TotalVolumeMl:    5000,
	})

	if selection.Class != VehicleMotorcycle {
		t.Fatalf("class = %q, want MOTORCYCLE", selection.Class)
	}

	if selection.Service != ServiceDispatch {
		t.Fatalf("service = %q, want DISPATCH", selection.Service)
	}
}

func TestSelectVehicle_BulkyLoadEscalates(t *testing.T) {
	van := selectFor(t, Requirement{TotalWeightGrams: 120000})

	if van.Class != VehicleVan || van.Service != ServiceHauling {
		t.Fatalf("120kg should be a hauling van, got %q/%q",
			van.Class, van.Service)
	}

	truck := selectFor(t, Requirement{TotalWeightGrams: 4000000})

	if truck.Class != VehicleTruck || truck.Service != ServiceHauling {
		t.Fatalf("4t should be a hauling truck, got %q/%q",
			truck.Class, truck.Service)
	}
}

// Van and truck both belong to driver-hauling-service. Vendor Service must
// never route a van anywhere else.
func TestServiceFor_VanAndTruckBothHauling(t *testing.T) {
	for _, class := range []VehicleClass{VehicleVan, VehicleTruck} {
		service, err := ServiceFor(class)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", class, err)
		}

		if service != ServiceHauling {
			t.Fatalf("%s mapped to %q, want HAULING", class, service)
		}
	}
}

// Section 17C must work without taxi. The car class is structurally excluded:
// it cannot be selected, and it cannot be mapped to a service.
func TestTaxiIsNeverReachable(t *testing.T) {
	if _, err := ServiceFor(VehicleCar); !errors.Is(err, ErrNoSuitableVehicle) {
		t.Fatal("VehicleCar must not map to any service while taxi is not production-ready")
	}

	if _, ok := classRank(VehicleCar); ok {
		t.Fatal("VehicleCar must not be selectable")
	}

	// Even if a deployment somehow offered a car capability, selection must
	// skip it rather than choose it.
	capabilities := append(DefaultCapabilities(), Capability{
		Class:                 VehicleCar,
		MaxWeightGrams:        100000,
		MaxVolumeMl:           500000,
		MaxLongestDimensionMm: 1500,
		AllowsLargeLoad:       true,
		Handles: []HandlingCapability{
			CapabilityHotHold, CapabilityInsulated,
			CapabilityColdChain, CapabilityFrozenChain,
			CapabilityUprightCarry, CapabilityFragileCarry,
			CapabilityLeakContainment,
		},
	})

	selection, err := SelectVehicle(
		Requirement{TotalWeightGrams: 60000},
		capabilities,
		map[Service]bool{ServiceDispatch: true, ServiceHauling: true},
	)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}

	if selection.Class == VehicleCar {
		t.Fatal("selection chose the car class; taxi must never be selected")
	}
}

// A cold chain cannot be satisfied by a motorcycle's insulated box.
func TestSelectVehicle_ColdChainEscalatesPastMotorcycle(t *testing.T) {
	selection := selectFor(t, Requirement{
		TotalWeightGrams: 1000,
		Capabilities:     []HandlingCapability{CapabilityColdChain},
	})

	if selection.Class != VehicleVan {
		t.Fatalf("class = %q, want VAN for a cold chain", selection.Class)
	}

	// But an insulated-only need stays on a bike.
	insulated := selectFor(t, Requirement{
		TotalWeightGrams: 1000,
		Capabilities:     []HandlingCapability{CapabilityInsulated},
	})

	if insulated.Class != VehicleMotorcycle {
		t.Fatalf("class = %q, want MOTORCYCLE for insulated-only",
			insulated.Class)
	}
}

func TestSelectVehicle_FragileAndLargeLoadEscalate(t *testing.T) {
	fragile := selectFor(t, Requirement{
		TotalWeightGrams: 500,
		Capabilities:     []HandlingCapability{CapabilityFragileCarry},
	})

	if fragile.Class != VehicleVan {
		t.Fatalf("fragile class = %q, want VAN", fragile.Class)
	}

	large := selectFor(t, Requirement{TotalWeightGrams: 500, LargeLoad: true})

	if large.Class != VehicleVan {
		t.Fatalf("large-load class = %q, want VAN", large.Class)
	}
}

func TestSelectVehicle_LongItemEscalatesRegardlessOfWeight(t *testing.T) {
	selection := selectFor(t, Requirement{
		TotalWeightGrams:   400,
		LongestDimensionMm: 1800,
	})

	if selection.Class != VehicleVan {
		t.Fatalf("class = %q, want VAN for a 1.8m item", selection.Class)
	}
}

// A vendor hint is read as "at least this class". One item hinted VAN must
// not be sent by motorcycle because another item hinted MOTORCYCLE.
func TestSelectVehicle_HintsFloorTheClass(t *testing.T) {
	selection := selectFor(t, Requirement{
		TotalWeightGrams: 500,
		VehicleHints:     []string{"MOTORCYCLE", "VAN"},
	})

	if selection.Class != VehicleVan {
		t.Fatalf("class = %q, want VAN from the hint floor", selection.Class)
	}

	if !selection.HintFloorApplied {
		t.Fatal("hint-driven escalation must be recorded, not silent")
	}

	// A motorcycle-only hint on a small load changes nothing.
	plain := selectFor(t, Requirement{
		TotalWeightGrams: 500,
		VehicleHints:     []string{"MOTORCYCLE"},
	})

	if plain.Class != VehicleMotorcycle || plain.HintFloorApplied {
		t.Fatalf("unexpected escalation: %+v", plain)
	}
}

// A CAR hint floors at VAN: honouring the vendor's stated need without
// selecting the class that has no production service behind it.
func TestSelectVehicle_CarHintFloorsAtVan(t *testing.T) {
	selection := selectFor(t, Requirement{
		TotalWeightGrams: 500,
		VehicleHints:     []string{"CAR"},
	})

	if selection.Class != VehicleVan {
		t.Fatalf("class = %q, want VAN for a CAR hint", selection.Class)
	}

	if selection.Service != ServiceHauling {
		t.Fatalf("service = %q, want HAULING", selection.Service)
	}
}

func TestSelectVehicle_UnknownHintIsIgnored(t *testing.T) {
	selection := selectFor(t, Requirement{
		TotalWeightGrams: 500,
		VehicleHints:     []string{"HELICOPTER"},
	})

	if selection.Class != VehicleMotorcycle {
		t.Fatalf("class = %q, want MOTORCYCLE — an unknown hint is not a floor",
			selection.Class)
	}
}

func TestSelectVehicle_ExceedsEveryCapacity(t *testing.T) {
	selection, err := SelectVehicle(
		Requirement{TotalWeightGrams: 900000000},
		DefaultCapabilities(),
		allServices(),
	)

	if !errors.Is(err, ErrNoSuitableVehicle) {
		t.Fatalf("expected refusal, got %v", err)
	}

	if selection.ReasonCode != ReasonExceedsAllCapacity {
		t.Fatalf("reason = %q, want %q",
			selection.ReasonCode, ReasonExceedsAllCapacity)
	}
}

func TestSelectVehicle_NoVehicleHandlesTheRequirement(t *testing.T) {
	selection, err := SelectVehicle(
		Requirement{
			TotalWeightGrams: 1000,
			Capabilities:     []HandlingCapability{CapabilityFrozenChain},
		},
		// A deployment whose only class is a motorcycle cannot freeze.
		DefaultCapabilities()[:1],
		allServices(),
	)

	if !errors.Is(err, ErrNoSuitableVehicle) {
		t.Fatalf("expected refusal, got %v", err)
	}

	if selection.ReasonCode != ReasonNoCapableVehicle {
		t.Fatalf("reason = %q, want %q",
			selection.ReasonCode, ReasonNoCapableVehicle)
	}
}

// A deployment with an integration switched off must not select a class it
// cannot execute, and must say so distinctly.
func TestSelectVehicle_RespectsDisabledIntegrations(t *testing.T) {
	// Dispatch off: a small load falls through to hauling rather than
	// selecting a motorcycle nothing can execute.
	selection := Requirement{TotalWeightGrams: 500}

	withoutDispatch, err := SelectVehicle(
		selection, DefaultCapabilities(),
		map[Service]bool{ServiceHauling: true},
	)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}

	if withoutDispatch.Service != ServiceHauling {
		t.Fatalf("service = %q, want HAULING when dispatch is off",
			withoutDispatch.Service)
	}

	// Nothing enabled at all refuses rather than defaulting to everything.
	none, err := SelectVehicle(
		selection, DefaultCapabilities(), map[Service]bool{},
	)

	if !errors.Is(err, ErrNoSuitableVehicle) {
		t.Fatalf("expected refusal with no enabled service, got %v", err)
	}

	if none.ReasonCode != ReasonServiceNotAvailable {
		t.Fatalf("reason = %q, want %q",
			none.ReasonCode, ReasonServiceNotAvailable)
	}
}

func TestSelectVehicle_HintsExcludeEveryAvailableClass(t *testing.T) {
	// Hinted TRUCK, but only dispatch is enabled.
	selection, err := SelectVehicle(
		Requirement{TotalWeightGrams: 500, VehicleHints: []string{"TRUCK"}},
		DefaultCapabilities(),
		map[Service]bool{ServiceDispatch: true},
	)

	if !errors.Is(err, ErrNoSuitableVehicle) {
		t.Fatalf("expected refusal, got %v", err)
	}

	if selection.ReasonCode != ReasonHintsExcludeAll {
		t.Fatalf("reason = %q, want %q",
			selection.ReasonCode, ReasonHintsExcludeAll)
	}
}

// A refusal must never name a class or a service: that is what stops an
// unserviceable group being handed to a logistics authority anyway.
func TestSelectVehicle_RefusalNamesNoService(t *testing.T) {
	selection, _ := SelectVehicle(
		Requirement{TotalWeightGrams: 900000000},
		DefaultCapabilities(),
		allServices(),
	)

	if selection.Class != "" || selection.Service != "" {
		t.Fatalf("refusal carried an execution target: %+v", selection)
	}
}
