package fulfilment_readiness

// Catering and Manufacturer readiness, now that migration 000121 lets a
// business hold either capability.
//
// The point of these tests is that the two categories use the ordinary
// readiness path -- same states, same predictor, same authority -- and that
// each resolves to its OWN model. Falling back to a restaurant's or a
// wholesaler's stages would produce a plausible-looking prediction of the
// wrong work.

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/vendors"
)

func TestCateringMapsToCateringReadinessProfile(t *testing.T) {
	resolved, err := ParseVendorType("CATERING")
	if err != nil {
		t.Fatalf("CATERING is not a vendor type: %v", err)
	}
	if resolved != VendorCatering {
		t.Fatalf("CATERING resolved to %q", resolved)
	}
	if !resolved.ConfigurableOnPlatform() {
		t.Fatal("CATERING is not configurable")
	}
	want := []Substate{SubProductionBatches, SubStaging, SubLoadReady}
	got := SubstatesFor(VendorCatering)
	if len(got) != len(want) {
		t.Fatalf("catering substates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("catering substate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// Its own model, not a neighbour's.
	for _, other := range []VendorType{VendorRestaurant, VendorWholesale, VendorCloudKitchen} {
		if sameSubstates(got, SubstatesFor(other)) {
			t.Fatalf("catering silently fell back to the %s model", other)
		}
	}
	if rules := DefaultRules(VendorCatering); rules.DefaultPrep != 3*time.Hour {
		t.Fatalf("catering default prep = %s, want its own 3h, not a borrowed default", rules.DefaultPrep)
	}
}

func TestManufacturerMapsToManufacturerReadinessProfile(t *testing.T) {
	resolved, err := ParseVendorType("MANUFACTURER")
	if err != nil {
		t.Fatalf("MANUFACTURER is not a vendor type: %v", err)
	}
	if resolved != VendorManufacturer {
		t.Fatalf("MANUFACTURER resolved to %q", resolved)
	}
	if !resolved.ConfigurableOnPlatform() {
		t.Fatal("MANUFACTURER is not configurable")
	}
	want := []Substate{SubProductionComplete, SubWarehouseRelease, SubLoadingBayReady}
	got := SubstatesFor(VendorManufacturer)
	if len(got) != len(want) {
		t.Fatalf("manufacturer substates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("manufacturer substate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	for _, other := range []VendorType{VendorRestaurant, VendorWholesale, VendorCatering} {
		if sameSubstates(got, SubstatesFor(other)) {
			t.Fatalf("manufacturer silently fell back to the %s model", other)
		}
	}
	if rules := DefaultRules(VendorManufacturer); rules.DefaultPrep != 4*time.Hour {
		t.Fatalf("manufacturer default prep = %s, want its own 4h", rules.DefaultPrep)
	}
}

func sameSubstates(a, b []Substate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The readiness vocabulary and the capability vocabulary are one list. A
// readiness type the platform cannot configure describes an order nobody can
// place; a capability with no readiness model has no way to say it is ready.
func TestReadinessVocabularyMatchesTheCapabilityVocabulary(t *testing.T) {
	platform := PlatformVendorTypes()
	if len(platform) != len(vendors.SupportedCapabilityTypes) {
		t.Fatalf("readiness types = %d, capability types = %d", len(platform), len(vendors.SupportedCapabilityTypes))
	}
	for i, capability := range vendors.SupportedCapabilityTypes {
		if string(platform[i]) != string(capability) {
			t.Fatalf("position %d: readiness %q, capability %q", i, platform[i], capability)
		}
		resolved, err := ParseVendorType(string(capability))
		if err != nil {
			t.Fatalf("capability %q has no readiness model: %v", capability, err)
		}
		if len(SubstatesFor(resolved)) == 0 {
			t.Fatalf("capability %q resolves to a model with no stages", capability)
		}
	}
}

// walk drives one order through its category's own stages, on the real
// service, and returns the predicted-ready windows seen along the way.
func walk(t *testing.T, vendorType VendorType, stages []Substate) []time.Duration {
	t.Helper()
	pool := readinessTestPool(t)
	now := time.Now().UTC()
	service := newReadinessService(pool, now)
	vendorOrderID, parentOrderID := seedOrderGraph(t, pool)
	ctx := context.Background()

	current, err := service.Start(ctx, StartRequest{
		VendorOrderID: vendorOrderID, ParentOrderID: parentOrderID,
		BusinessID: "biz-1", BranchID: "br-1",
		VendorType: vendorType, Actor: staffActor(),
	})
	if err != nil {
		t.Fatalf("start %s: %v", vendorType, err)
	}
	if current.VendorType != vendorType {
		t.Fatalf("started as %q, want %q", current.VendorType, vendorType)
	}
	if current.PredictedReadyStart == nil || current.PredictedReadyEnd == nil {
		t.Fatalf("%s start recorded no predicted window", vendorType)
	}

	remaining := []time.Duration{current.PredictedReadyEnd.Sub(now)}
	for _, stage := range stages {
		current, err = service.Advance(ctx, AdvanceRequest{
			VendorOrderID: vendorOrderID, ExpectedVersion: current.Version,
			Move:  Move{To: StatePreparing, Substate: stage},
			Actor: staffActor(),
		})
		if err != nil {
			t.Fatalf("%s -> %s: %v", vendorType, stage, err)
		}
		if current.Substate != stage {
			t.Fatalf("substate = %q, want %q", current.Substate, stage)
		}
		if current.PredictedReadyEnd == nil {
			t.Fatalf("%s at %s recorded no prediction", vendorType, stage)
		}
		remaining = append(remaining, current.PredictedReadyEnd.Sub(now))
	}

	// The last stage of production is nearly done, so the order moves to
	// READY_SOON and then READY through the ordinary authority.
	current, err = service.Advance(ctx, AdvanceRequest{
		VendorOrderID: vendorOrderID, ExpectedVersion: current.Version,
		Move: Move{To: StateReadySoon, Substate: stages[len(stages)-1]}, Actor: staffActor(),
	})
	if err != nil {
		t.Fatalf("%s -> READY_SOON: %v", vendorType, err)
	}
	current, err = service.Advance(ctx, AdvanceRequest{
		VendorOrderID: vendorOrderID, ExpectedVersion: current.Version,
		Move: Move{To: StateReady}, Actor: staffActor(),
	})
	if err != nil {
		t.Fatalf("%s -> READY: %v", vendorType, err)
	}
	if current.State != StateReady || current.FirstActualReadyAt == nil {
		t.Fatalf("%s did not reach READY with a recorded time: %+v", vendorType, current)
	}
	return remaining
}

func TestCateringReadinessProgression(t *testing.T) {
	walk(t, VendorCatering, SubstatesFor(VendorCatering))
}

func TestManufacturerReadinessProgression(t *testing.T) {
	walk(t, VendorManufacturer, SubstatesFor(VendorManufacturer))
}

// Production signals move the prediction. Each stage completed is work that
// no longer remains, so the predicted window closes as the order progresses.
func TestCateringPredictionRecalculatesFromProductionSignals(t *testing.T) {
	assertPredictionCloses(t, VendorCatering, walk(t, VendorCatering, SubstatesFor(VendorCatering)))
}

func TestManufacturerPredictionRecalculatesFromProductionSignals(t *testing.T) {
	assertPredictionCloses(t, VendorManufacturer, walk(t, VendorManufacturer, SubstatesFor(VendorManufacturer)))
}

func assertPredictionCloses(t *testing.T, vendorType VendorType, remaining []time.Duration) {
	t.Helper()
	if len(remaining) < 3 {
		t.Fatalf("%s produced %d predictions", vendorType, len(remaining))
	}
	for i := 1; i < len(remaining); i++ {
		if remaining[i] > remaining[i-1] {
			t.Fatalf("%s prediction grew from %s to %s after a stage completed",
				vendorType, remaining[i-1], remaining[i])
		}
	}
	if remaining[len(remaining)-1] >= remaining[0] {
		t.Fatalf("%s prediction never moved: first %s, last %s",
			vendorType, remaining[0], remaining[len(remaining)-1])
	}
}

// Prediction is pure arithmetic over the branch's own stages: an Analytics
// profile sharpens it but is never required.
func TestProductionPredictionNeedsNoAnalyticsProfile(t *testing.T) {
	now := time.Now().UTC()
	for _, vendorType := range []VendorType{VendorCatering, VendorManufacturer} {
		stages := SubstatesFor(vendorType)
		first := Predict(PredictionInput{Now: now, VendorType: vendorType, State: StatePreparing,
			Substate: stages[0], PrepStartedAt: &now})
		last := Predict(PredictionInput{Now: now, VendorType: vendorType, State: StatePreparing,
			Substate: stages[len(stages)-1], PrepStartedAt: &now})
		if first.End.IsZero() || last.End.IsZero() {
			t.Fatalf("%s predicted nothing without an Analytics profile", vendorType)
		}
		if !last.End.Before(first.End) {
			t.Fatalf("%s: later stage is not closer to ready (%s vs %s)",
				vendorType, last.End, first.End)
		}
	}
}
