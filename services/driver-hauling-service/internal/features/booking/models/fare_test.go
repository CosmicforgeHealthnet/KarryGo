package bookingmodels

import "testing"

// Characterization tests for the v1 Haulage fare formula.
//
// These tests pin what CalculateFare returns TODAY. They are the migration
// source for the Central Pricing Engine's first Haulage policy version, which
// must reproduce these amounts exactly so that cutover changes no customer's
// price. They are deliberately assertions about current behaviour, not a
// statement that these are the correct permanent commercial rates -- the
// permanent rates are an open product decision.
//
// If you change the formula, these tests SHOULD fail. Update them only
// together with a new, approved pricing policy version.

func TestCalculateFare_PinnedAmounts(t *testing.T) {
	cases := []struct {
		name     string
		km       float64
		weightKg int
		helpers  int
		want     int64
	}{
		// Base only: there is no minimum fare separate from the base.
		{"zero distance is the base fare", 0, 0, 0, 500_000},
		{"tiny trip adds rounded per-km only", 0.001, 0, 0, 500_025},
		{"10km no weight no helpers", 10, 0, 0, 750_000},
		{"25.5km", 25.5, 0, 0, 1_137_500},

		// Helpers are a flat 2,000 naira each and are not affected by weight.
		{"one helper", 10, 0, 1, 950_000},
		{"three helpers", 10, 0, 3, 1_350_000},

		// Weight surcharge is 10% of (base + distance), and it applies only
		// ABOVE 500kg -- exactly 500kg is NOT surcharged.
		{"exactly 500kg is not surcharged", 10, 500, 0, 750_000},
		{"501kg is surcharged", 10, 501, 0, 825_000},
		{"1000kg", 10, 1000, 0, 825_000},

		// The surcharge is computed before helpers are added, so helpers are
		// never surcharged.
		{"heavy load with two helpers", 10, 900, 2, 1_225_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CalculateFare(tc.km, tc.weightKg, tc.helpers)
			if got.FareEstimateKobo != tc.want {
				t.Fatalf("CalculateFare(%v, %d, %d) = %d kobo, want %d",
					tc.km, tc.weightKg, tc.helpers, got.FareEstimateKobo, tc.want)
			}
		})
	}
}

// Unlike Taxi and Dispatch, Haulage does NOT round the total to the nearest
// 50 kobo. This is a real difference between the services and the central
// engine's Haulage policy must reproduce it.
func TestCalculateFare_TotalIsNotRoundedTo50Kobo(t *testing.T) {
	// 1.0001km * 25,000 = 25,002.5 kobo, which rounds half-away-from-zero to
	// 25,003 -- an amount that is not a multiple of 50.
	got := CalculateFare(1.0001, 0, 0)

	if got.BreakdownKobo.PerKmFareKobo != 25_003 {
		t.Fatalf("per-km component = %d kobo, want 25003 (half away from zero)",
			got.BreakdownKobo.PerKmFareKobo)
	}
	if got.FareEstimateKobo != 525_003 {
		t.Fatalf("total = %d kobo, want 525003", got.FareEstimateKobo)
	}
	if got.FareEstimateKobo%50 == 0 {
		t.Fatalf("total %d is a multiple of 50; Haulage is expected NOT to round to 50 kobo",
			got.FareEstimateKobo)
	}
}

// The weight surcharge uses integer division, so it truncates rather than
// rounding.
func TestCalculateFare_WeightSurchargeTruncates(t *testing.T) {
	// base 500,000 + per-km 25,003 = 525,003; 10% is 52,500.3 which truncates
	// to 52,500.
	got := CalculateFare(1.0001, 600, 0)

	if got.BreakdownKobo.WeightSurcharge != 52_500 {
		t.Fatalf("weight surcharge = %d kobo, want 52500 (truncated, not rounded)",
			got.BreakdownKobo.WeightSurcharge)
	}
	if got.FareEstimateKobo != 577_503 {
		t.Fatalf("total = %d kobo, want 577503", got.FareEstimateKobo)
	}
}

// The breakdown shown to the customer must always add up to the charged total,
// and the distance must be echoed back unchanged. The central engine enforces
// the same "components sum to the total" rule.
func TestCalculateFare_BreakdownSumsToTotalAndEchoesDistance(t *testing.T) {
	for _, km := range []float64{0, 0.7, 3.333, 12.3456, 25.5, 480} {
		for _, weightKg := range []int{0, 500, 501, 2000} {
			for _, helpers := range []int{0, 1, 4} {
				got := CalculateFare(km, weightKg, helpers)

				sum := got.BreakdownKobo.BaseFareKobo +
					got.BreakdownKobo.PerKmFareKobo +
					got.BreakdownKobo.WeightSurcharge +
					got.BreakdownKobo.HelperFeeKobo
				if sum != got.FareEstimateKobo {
					t.Fatalf("km=%v weight=%d helpers=%d: breakdown sums to %d but total is %d",
						km, weightKg, helpers, sum, got.FareEstimateKobo)
				}
				if got.DistanceKm != km {
					t.Fatalf("distance echoed as %v, want %v", got.DistanceKm, km)
				}
				if got.BreakdownKobo.BaseFareKobo != 500_000 {
					t.Fatalf("base component = %d kobo, want 500000", got.BreakdownKobo.BaseFareKobo)
				}
			}
		}
	}
}
