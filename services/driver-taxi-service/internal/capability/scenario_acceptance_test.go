package capability

// Section 25 acceptance scenario 3: removing one product's certification stops
// new work for that product only. The same vehicle, otherwise eligible, keeps
// taking Standard work.

import "testing"

func TestScenario03ComfortSuspensionLeavesStandardWorkRunning(t *testing.T) {
	comfort := Product{Code: "TAXI_COMFORT", Version: 1, Purpose: PurposePassenger, Active: true, MaxPassengers: 4}
	vehicle := passengerCandidate(ptrInt(4), "TAXI_STANDARD", "TAXI_COMFORT")

	// Certified for both: both products are available.
	for _, product := range []Product{standard, comfort} {
		requirement := PassengerRequirement{ProductCode: product.Code, PassengerCount: 2}
		if got := EvaluatePassenger(vehicle, requirement, product, now); got.Reason != ReasonEligible {
			t.Fatalf("before suspension %s = %s, want ELIGIBLE", product.Code, got.Reason)
		}
	}

	// Verification suspends Comfort only.
	row := vehicle.Eligibility["TAXI_COMFORT"]
	row.Status = EligibilitySuspended
	vehicle.Eligibility["TAXI_COMFORT"] = row

	if got := EvaluatePassenger(vehicle, PassengerRequirement{ProductCode: "TAXI_COMFORT", PassengerCount: 2}, comfort, now); got.Eligible || got.Reason != ReasonProductNotEligible {
		t.Fatalf("new Comfort work after suspension = %+v, want PRODUCT_NOT_ELIGIBLE", got)
	}
	if got := EvaluatePassenger(vehicle, PassengerRequirement{ProductCode: "TAXI_STANDARD", PassengerCount: 2}, standard, now); !got.Eligible {
		t.Fatalf("Standard work stopped by a Comfort-only suspension: %+v", got)
	}
}
