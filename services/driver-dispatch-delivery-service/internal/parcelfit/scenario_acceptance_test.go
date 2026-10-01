package parcelfit

// Section 25 acceptance scenario 5: a large cake that must stay upright is
// refused by a motorcycle even though it weighs almost nothing.

import (
	"testing"

	"cosmicforge/logistics/shared/go/capabilityfit"
)

func TestScenario05LargeUprightCakeRejectsMotorcycleDespiteLowWeight(t *testing.T) {
	bike := surveyedBike() // 20 kg payload, 500 x 400 x 400 mm box
	cake := Requirement{WeightGrams: 2_500, LengthMm: 350, WidthMm: 350, HeightMm: 450, UprightRequired: true}

	if cake.WeightGrams >= bike.MaxPayloadGrams {
		t.Fatalf("scenario needs a load well inside payload: %d g vs %d g", cake.WeightGrams, bike.MaxPayloadGrams)
	}
	// Laid on its side it would fit; upright it is taller than the box.
	lying := cake
	lying.UprightRequired = false
	assertEligible(t, Evaluate(ResolutionFound, bike, lying))
	assertRefused(t, Evaluate(ResolutionFound, bike, cake), capabilityfit.ReasonCargoFitFailed)
}
