package fulfilment

import (
	"reflect"
	"testing"
)

func i64(v int64) *int64 { return &v }
func iptr(v int) *int    { return &v }

func line(id string, quantity int64) LineLogistics {
	return LineLogistics{
		CheckoutLineID: id,
		Domain:         "SHOP",
		BusinessID:     "biz_1",
		BranchID:       "brn_1",
		ItemRef:        "item_" + id,
		Quantity:       quantity,
	}
}

func TestAggregate_SumsMeasuredLines(t *testing.T) {
	lines := []LineLogistics{line("l1", 2), line("l2", 3)}

	profiles := map[string]LineProfile{
		"l1": {
			Known:       true,
			WeightGrams: i64(500),
			VolumeMl:    i64(400),
			LengthMm:    iptr(200),
			HeightMm:    iptr(310),
		},
		"l2": {
			Known:       true,
			WeightGrams: i64(1000),
			VolumeMl:    i64(900),
			WidthMm:     iptr(120),
		},
	}

	requirement := Aggregate("biz_1", "brn_1", lines, profiles)

	// Package count is units carried, not lines.
	if requirement.PackageCount != 5 {
		t.Fatalf("package count = %d, want 5", requirement.PackageCount)
	}

	if requirement.TotalWeightGrams != 500*2+1000*3 {
		t.Fatalf("weight = %d, want 4000", requirement.TotalWeightGrams)
	}

	if requirement.TotalVolumeMl != 400*2+900*3 {
		t.Fatalf("volume = %d, want 3500", requirement.TotalVolumeMl)
	}

	// Longest dimension is a max over single units, never a sum — stacking
	// cannot make a long item fit.
	if requirement.LongestDimensionMm != 310 {
		t.Fatalf("longest dimension = %d, want 310",
			requirement.LongestDimensionMm)
	}

	if requirement.WeightEvidence != EvidenceComplete ||
		requirement.VolumeEvidence != EvidenceComplete ||
		requirement.DimensionEvidence != EvidenceComplete {
		t.Fatalf("evidence should be complete, got %+v", requirement)
	}

	if requirement.Urgency != UrgencyStandard {
		t.Fatalf("urgency = %q, want STANDARD", requirement.Urgency)
	}
}

// Missing evidence must be reported, never invented. This is the test that
// stops an unstated weight from silently justifying a motorcycle.
func TestAggregate_MissingWeightIsPartialNotZero(t *testing.T) {
	lines := []LineLogistics{line("l1", 1), line("l2", 1)}

	profiles := map[string]LineProfile{
		"l1": {Known: true, WeightGrams: i64(800)},
		"l2": {Known: true},
	}

	requirement := Aggregate("biz_1", "brn_1", lines, profiles)

	if requirement.WeightEvidence != EvidencePartial {
		t.Fatalf("weight evidence = %q, want PARTIAL",
			requirement.WeightEvidence)
	}

	if requirement.TotalWeightGrams != 800 {
		t.Fatalf("weight = %d, want the lower bound 800",
			requirement.TotalWeightGrams)
	}
}

func TestAggregate_UnprofiledLineStillCounted(t *testing.T) {
	lines := []LineLogistics{line("l1", 1), line("missing", 4)}

	profiles := map[string]LineProfile{
		"l1": {Known: true, WeightGrams: i64(100)},
	}

	requirement := Aggregate("biz_1", "brn_1", lines, profiles)

	if requirement.PackageCount != 5 {
		t.Fatalf("package count = %d, want 5 — an unprofiled line is still carried",
			requirement.PackageCount)
	}

	if requirement.ProfiledLines != 1 || requirement.TotalLines != 2 {
		t.Fatalf("profiled/total = %d/%d, want 1/2",
			requirement.ProfiledLines, requirement.TotalLines)
	}

	if requirement.WeightEvidence != EvidencePartial {
		t.Fatalf("weight evidence = %q, want PARTIAL",
			requirement.WeightEvidence)
	}
}

func TestAggregate_NoMeasurementsAtAllIsAbsent(t *testing.T) {
	requirement := Aggregate(
		"biz_1", "brn_1",
		[]LineLogistics{line("l1", 1)},
		map[string]LineProfile{"l1": {Known: true}},
	)

	if requirement.WeightEvidence != EvidenceAbsent ||
		requirement.VolumeEvidence != EvidenceAbsent ||
		requirement.DimensionEvidence != EvidenceAbsent {
		t.Fatalf("expected absent evidence, got %+v", requirement)
	}
}

// The Food size class is fallback evidence only: it contributes a weight but
// must never let the group claim COMPLETE measurement.
func TestAggregate_SizeClassFallbackStaysPartial(t *testing.T) {
	requirement := Aggregate(
		"biz_1", "brn_1",
		[]LineLogistics{line("l1", 2)},
		map[string]LineProfile{
			"l1": {Known: true, PackageSizeClass: "LARGE"},
		},
	)

	if requirement.TotalWeightGrams != 24000 {
		t.Fatalf("weight = %d, want the LARGE fallback 2x12000",
			requirement.TotalWeightGrams)
	}

	if requirement.WeightEvidence == EvidenceComplete {
		t.Fatal("a size-class fallback must never be reported as a measured weight")
	}
}

func TestAggregate_TemperatureVocabulariesNormalize(t *testing.T) {
	cases := map[string]HandlingCapability{
		"HOT":     CapabilityHotHold,
		"COLD":    CapabilityInsulated,
		"FRESH":   CapabilityInsulated,
		"CHILLED": CapabilityColdChain,
		"FROZEN":  CapabilityFrozenChain,
	}

	for temperature, want := range cases {
		requirement := Aggregate(
			"biz_1", "brn_1",
			[]LineLogistics{line("l1", 1)},
			map[string]LineProfile{
				"l1": {Known: true, TemperatureClass: temperature},
			},
		)

		if !requirement.HasCapability(want) {
			t.Fatalf("%s should require %s, got %v",
				temperature, want, requirement.Capabilities)
		}
	}

	// AMBIENT asks for nothing.
	ambient := Aggregate(
		"biz_1", "brn_1",
		[]LineLogistics{line("l1", 1)},
		map[string]LineProfile{
			"l1": {Known: true, TemperatureClass: "AMBIENT"},
		},
	)

	if len(ambient.Capabilities) != 0 {
		t.Fatalf("ambient should require nothing, got %v", ambient.Capabilities)
	}
}

// An insulated box is not a cold chain. This distinction is what keeps a cold
// drink on a bike and chilled meat off one.
func TestAggregate_InsulatedIsNotACoolingChain(t *testing.T) {
	cold := Aggregate("biz_1", "brn_1",
		[]LineLogistics{line("l1", 1)},
		map[string]LineProfile{"l1": {Known: true, TemperatureClass: "COLD"}})

	if cold.RequiresCoolingChain() {
		t.Fatal("a COLD food line must not demand an active cooling chain")
	}

	chilled := Aggregate("biz_1", "brn_1",
		[]LineLogistics{line("l1", 1)},
		map[string]LineProfile{"l1": {Known: true, TemperatureClass: "CHILLED"}})

	if !chilled.RequiresCoolingChain() {
		t.Fatal("a CHILLED shop line must demand an active cooling chain")
	}
}

func TestAggregate_HotFoodIsImmediate(t *testing.T) {
	requirement := Aggregate("biz_1", "brn_1",
		[]LineLogistics{line("l1", 1)},
		map[string]LineProfile{"l1": {Known: true, TemperatureClass: "HOT"}})

	if requirement.Urgency != UrgencyImmediate {
		t.Fatalf("urgency = %q, want IMMEDIATE for hot food",
			requirement.Urgency)
	}
}

func TestAggregate_HandlingFlagsBecomeCapabilities(t *testing.T) {
	requirement := Aggregate("biz_1", "brn_1",
		[]LineLogistics{line("l1", 1)},
		map[string]LineProfile{"l1": {
			Known:           true,
			Fragile:         true,
			SpillRisk:       true,
			UprightRequired: true,
		}})

	for _, want := range []HandlingCapability{
		CapabilityFragileCarry,
		CapabilityLeakContainment,
		CapabilityUprightCarry,
	} {
		if !requirement.HasCapability(want) {
			t.Fatalf("missing %s in %v", want, requirement.Capabilities)
		}
	}
}

func TestAggregate_SeparationConflictIsReported(t *testing.T) {
	requirement := Aggregate("biz_1", "brn_1",
		[]LineLogistics{line("l1", 1), line("l2", 1)},
		map[string]LineProfile{
			"l1": {Known: true, SeparationGroup: "raw-protein"},
			"l2": {Known: true, SeparationGroup: "ready-to-eat"},
		})

	if !requirement.SeparationConflict() {
		t.Fatalf("expected a separation conflict, got %v",
			requirement.SeparationGroups)
	}

	// Same group twice is not a conflict.
	same := Aggregate("biz_1", "brn_1",
		[]LineLogistics{line("l1", 1), line("l2", 1)},
		map[string]LineProfile{
			"l1": {Known: true, SeparationGroup: "raw-protein"},
			"l2": {Known: true, SeparationGroup: "RAW-PROTEIN"},
		})

	if same.SeparationConflict() {
		t.Fatalf("case-different same group must not conflict, got %v",
			same.SeparationGroups)
	}
}

// These values are persisted on a plan and compared across replans, so map
// iteration order must never leak into them.
func TestAggregate_OutputIsDeterministic(t *testing.T) {
	build := func() Requirement {
		return Aggregate("biz_1", "brn_1",
			[]LineLogistics{line("l1", 1), line("l2", 1), line("l3", 1)},
			map[string]LineProfile{
				"l1": {Known: true, TemperatureClass: "FROZEN", Fragile: true,
					SeparationGroup: "z", ConsolidationClass: "FROZEN",
					VehicleHints: []string{"VAN", "TRUCK"}},
				"l2": {Known: true, SpillRisk: true, SeparationGroup: "a",
					ConsolidationClass: "LIQUID", VehicleHints: []string{"TRUCK"}},
				"l3": {Known: true, UprightRequired: true, SeparationGroup: "m",
					ConsolidationClass: "GENERAL"},
			})
	}

	first := build()

	for i := 0; i < 25; i++ {
		if !reflect.DeepEqual(first, build()) {
			t.Fatal("aggregation is not deterministic across runs")
		}
	}
}
