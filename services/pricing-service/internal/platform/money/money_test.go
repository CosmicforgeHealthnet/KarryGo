package money

import (
	"errors"
	"math"
	"testing"
)

func TestRoundingModes(t *testing.T) {
	// Each case is num/den, so the exact result is stated in the name.
	cases := []struct {
		name string
		num  int64
		den  int64
		want map[RoundingMode]int64
	}{
		{"exactly 0.5", 1, 2, map[RoundingMode]int64{
			Truncate: 0, Floor: 0, HalfAwayFromZero: 1, HalfUp: 1,
		}},
		{"exactly -0.5", -1, 2, map[RoundingMode]int64{
			Truncate: 0, Floor: -1, HalfAwayFromZero: -1, HalfUp: 0,
		}},
		{"0.4", 2, 5, map[RoundingMode]int64{
			Truncate: 0, Floor: 0, HalfAwayFromZero: 0, HalfUp: 0,
		}},
		{"0.6", 3, 5, map[RoundingMode]int64{
			Truncate: 0, Floor: 0, HalfAwayFromZero: 1, HalfUp: 1,
		}},
		{"-0.6", -3, 5, map[RoundingMode]int64{
			Truncate: 0, Floor: -1, HalfAwayFromZero: -1, HalfUp: -1,
		}},
		{"exact integer needs no rounding", 10, 5, map[RoundingMode]int64{
			Truncate: 2, Floor: 2, HalfAwayFromZero: 2, HalfUp: 2,
		}},
	}

	for _, tc := range cases {
		for mode, want := range tc.want {
			t.Run(tc.name+"/"+string(mode), func(t *testing.T) {
				got, err := PerUnit(tc.num, 1, tc.den, mode)
				if err != nil {
					t.Fatalf("PerUnit: %v", err)
				}
				if got != want {
					t.Fatalf("%s of %d/%d = %d, want %d", mode, tc.num, tc.den, got, want)
				}
			})
		}
	}
}

func TestUnknownRoundingModeIsRejected(t *testing.T) {
	if _, err := PerUnit(100, 1, 3, RoundingMode("NEAREST_GUESS")); err == nil {
		t.Fatal("an unknown rounding mode must be an error, not a silent default")
	}
	if RoundingMode("").Valid() {
		t.Fatal("the empty rounding mode must not be valid")
	}
}

func TestMultiplyIsExactBeforeRounding(t *testing.T) {
	// The legacy dispatch express surcharge multiplies the whole subtotal by
	// 1.35. Done exactly, 260,000 x 135/100 is 351,000 with no remainder.
	got, err := Multiply(260_000, Rational{Num: 135, Den: 100}, HalfAwayFromZero)
	if err != nil {
		t.Fatalf("Multiply: %v", err)
	}
	if got != 351_000 {
		t.Fatalf("express surcharge = %d, want 351000", got)
	}
}

func TestPerUnitPricesDistanceFromWholeMetres(t *testing.T) {
	// 180 naira per kilometre over 5,000 metres, with no float kilometre in
	// sight.
	got, err := PerUnit(18_000, 5_000, 1_000, HalfAwayFromZero)
	if err != nil {
		t.Fatalf("PerUnit: %v", err)
	}
	if got != 90_000 {
		t.Fatalf("distance component = %d, want 90000", got)
	}
}

func TestBasisPointsFloorsLikeTheWallet(t *testing.T) {
	// The wallet computes the platform fee as amount*bps/10000 with integer
	// division, i.e. floored. 170,003 x 15% is 25,500.45.
	got, err := BasisPoints(170_003, 1_500, Floor)
	if err != nil {
		t.Fatalf("BasisPoints: %v", err)
	}
	if got != 25_500 {
		t.Fatalf("platform fee = %d, want 25500 (floored)", got)
	}
}

func TestRoundToStep(t *testing.T) {
	cases := []struct {
		amount int64
		step   int64
		mode   RoundingMode
		want   int64
	}{
		{170_037, 50, HalfAwayFromZero, 170_050},
		{170_024, 50, HalfAwayFromZero, 170_000},
		{170_025, 50, HalfAwayFromZero, 170_050}, // exactly half a step
		{170_025, 50, Truncate, 170_000},
		{525_003, 0, HalfAwayFromZero, 525_003}, // step 0 means no rounding
		{525_003, 1, HalfAwayFromZero, 525_003},
	}
	for _, tc := range cases {
		got, err := RoundToStep(tc.amount, tc.step, tc.mode)
		if err != nil {
			t.Fatalf("RoundToStep: %v", err)
		}
		if got != tc.want {
			t.Fatalf("RoundToStep(%d, %d, %s) = %d, want %d", tc.amount, tc.step, tc.mode, got, tc.want)
		}
	}
}

func TestSumAndOverflowAreRefusedNotWrapped(t *testing.T) {
	if _, err := Sum(math.MaxInt64, 1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Sum overflow error = %v, want ErrOverflow", err)
	}
	if _, err := Multiply(math.MaxInt64, Rational{Num: 2, Den: 1}, Truncate); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Multiply overflow error = %v, want ErrOverflow", err)
	}
	total, err := Sum(500_000, 250_000, 75_000)
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if total != 825_000 {
		t.Fatalf("Sum = %d, want 825000", total)
	}
}

func TestZeroDenominatorAndUnitAreErrors(t *testing.T) {
	if _, err := Multiply(100, Rational{Num: 1, Den: 0}, Truncate); err == nil {
		t.Fatal("a zero denominator must be an error")
	}
	if _, err := PerUnit(100, 1, 0, Truncate); err == nil {
		t.Fatal("a zero unit must be an error")
	}
}

// The tests below are the reason this package exists: they show that these
// primitives reproduce the THREE legacy formulas exactly, which is what makes a
// price-neutral Version 1 possible. The expected values are taken from the
// legacy code and its characterization tests, and are deliberately written out
// here rather than imported -- pricing-service must not depend on the legacy
// fare packages it is replacing.
func TestReproducesLegacyDispatchFares(t *testing.T) {
	// shared/go/dispatchpricing: base 80000 + 18000/km + size addon, express
	// multiplies the whole subtotal by 1.35, round to nearest 50, then a
	// 100,000 minimum applied AFTER rounding.
	const (
		baseKobo    = 80_000
		perKmKobo   = 18_000
		mediumAddon = 30_000
		largeAddon  = 70_000
		minFare     = 100_000
	)
	express := Rational{Num: 135, Den: 100}

	fare := func(metres int64, addon int64, isExpress bool) int64 {
		t.Helper()
		distance, err := PerUnit(perKmKobo, metres, 1_000, HalfAwayFromZero)
		if err != nil {
			t.Fatalf("distance: %v", err)
		}
		subtotal, err := Sum(baseKobo, distance, addon)
		if err != nil {
			t.Fatalf("subtotal: %v", err)
		}
		if isExpress {
			if subtotal, err = Multiply(subtotal, express, HalfAwayFromZero); err != nil {
				t.Fatalf("express: %v", err)
			}
		}
		rounded, err := RoundToStep(subtotal, 50, HalfAwayFromZero)
		if err != nil {
			t.Fatalf("round: %v", err)
		}
		return AtLeast(rounded, minFare)
	}

	cases := []struct {
		name string
		got  int64
		want int64
	}{
		{"5km small standard", fare(5_000, 0, false), 170_000},
		{"5km medium standard", fare(5_000, mediumAddon, false), 200_000},
		{"5km large standard", fare(5_000, largeAddon, false), 240_000},
		{"5km small express", fare(5_000, 0, true), 229_500},
		{"500m trip hits the minimum", fare(500, 0, false), 100_000},
		{"10km small standard", fare(10_000, 0, false), 260_000},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %d kobo, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestReproducesLegacyHaulageFares(t *testing.T) {
	// driver-hauling-service CalculateFare: base 500000 + 25000/km, a 10%
	// surcharge on (base+distance) above 500kg using integer division, 200000
	// per helper, and NO total rounding or minimum.
	const (
		baseKobo  = 500_000
		perKmKobo = 25_000
		helperFee = 200_000
	)

	fare := func(metres int64, weightGrams int64, helpers int64) int64 {
		t.Helper()
		distance, err := PerUnit(perKmKobo, metres, 1_000, HalfAwayFromZero)
		if err != nil {
			t.Fatalf("distance: %v", err)
		}
		subtotal, err := Sum(baseKobo, distance)
		if err != nil {
			t.Fatalf("subtotal: %v", err)
		}
		var surcharge int64
		if weightGrams > 500_000 { // strictly above 500kg
			if surcharge, err = BasisPoints(subtotal, 1_000, Truncate); err != nil {
				t.Fatalf("surcharge: %v", err)
			}
		}
		total, err := Sum(subtotal, surcharge, helpers*helperFee)
		if err != nil {
			t.Fatalf("total: %v", err)
		}
		return total // no rounding, no minimum
	}

	cases := []struct {
		name string
		got  int64
		want int64
	}{
		{"10km empty", fare(10_000, 0, 0), 750_000},
		{"10km exactly 500kg is not surcharged", fare(10_000, 500_000, 0), 750_000},
		{"10km 501kg is surcharged", fare(10_000, 501_000, 0), 825_000},
		{"10km with three helpers", fare(10_000, 0, 3), 1_350_000},
		{"25.5km", fare(25_500, 0, 0), 1_137_500},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %d kobo, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestReproducesLegacyTaxiFares(t *testing.T) {
	// driver-taxi-service config_provider: base + per_km*km + per_min*minutes,
	// rounded to the nearest 50 kobo, then raised to the class minimum. These
	// rates are the dev defaults; production values are captured separately and
	// become policy data.
	const (
		baseKobo   = 30_000
		perKmKobo  = 15_000
		perMinKobo = 2_000
		minKobo    = 100_000
	)

	fare := func(metres int64, seconds int64) int64 {
		t.Helper()
		distance, err := PerUnit(perKmKobo, metres, 1_000, Truncate)
		if err != nil {
			t.Fatalf("distance: %v", err)
		}
		// The legacy provider charges whole minutes, flooring the remainder.
		minutes, err := PerUnit(seconds, 1, 60, Floor)
		if err != nil {
			t.Fatalf("minutes: %v", err)
		}
		subtotal, err := Sum(baseKobo, distance, minutes*perMinKobo)
		if err != nil {
			t.Fatalf("subtotal: %v", err)
		}
		rounded, err := RoundToStep(subtotal, 50, HalfUp)
		if err != nil {
			t.Fatalf("round: %v", err)
		}
		return AtLeast(rounded, minKobo)
	}

	// 8km, 20m30s: 30000 + 120000 + 20*2000 = 190000.
	if got := fare(8_000, 1_230); got != 190_000 {
		t.Errorf("8km/20m30s = %d kobo, want 190000 (30s does not buy a minute)", got)
	}
	// A very short trip is raised to the minimum.
	if got := fare(300, 60); got != 100_000 {
		t.Errorf("300m/1m = %d kobo, want the 100000 minimum", got)
	}
}
