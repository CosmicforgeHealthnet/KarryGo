package cargofit

// Section 25 acceptance scenario 10: cargo that is well within the truck's
// payload but longer than its body is refused.

import (
	"testing"

	"cosmicforge/logistics/shared/go/capabilityfit"
)

func TestScenario10CargoLongerThanBodyIsRefusedAlthoughWeightFits(t *testing.T) {
	truck := surveyedBoxTruck() // 3 t payload, 4000 x 2000 x 2200 mm body
	cargo := Requirement{WeightGrams: 400_000, LengthMm: 4_600, WidthMm: 1_000, HeightMm: 1_000}

	if cargo.WeightGrams >= truck.MaxPayloadGrams {
		t.Fatalf("scenario needs a load well inside payload: %d g vs %d g", cargo.WeightGrams, truck.MaxPayloadGrams)
	}
	fits := cargo
	fits.LengthMm = 3_000
	assertEligible(t, Evaluate(ResolutionFound, truck, fits))
	assertRefused(t, Evaluate(ResolutionFound, truck, cargo), capabilityfit.ReasonCargoFitFailed)
}
