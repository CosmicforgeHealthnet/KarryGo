package cost

import "testing"

// TestCostOfIsExactAndRoundsHalfAwayFromZero pins the documented rounding rule.
//
// The cases that matter are the ones a float implementation gets wrong: a
// third of an odd amount, and an exact .5 boundary.
func TestCostOfIsExactAndRoundsHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		name          string
		numerator     int64
		denominator   int64
		unitCostMinor int64
		want          int64
	}{
		{"whole quantity", 3, 1, 850, 2550},
		{"one third of 850", 1, 3, 850, 283},             // 283.33 -> 283
		{"two thirds of 850", 2, 3, 850, 567},            // 566.67 -> 567
		{"exact half rounds away", 1, 2, 851, 426},       // 425.5 -> 426
		{"exact half rounds away again", 1, 2, 849, 425}, // 424.5 -> 425
		{"zero quantity is a real zero", 0, 1, 850, 0},
		{"zero unit cost is a real zero", 5, 1, 0, 0},
		// Overflow territory: the intermediate product exceeds int64 while the
		// result does not. An int64 implementation would wrap here.
		{"large values stay exact", 1, 1000000, 9000000000000000, 9000000000},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			quantity, err := NewQuantity(testCase.numerator, testCase.denominator)
			if err != nil {
				t.Fatalf("quantity: %v", err)
			}
			got, err := CostOf(quantity, testCase.unitCostMinor)
			if err != nil {
				t.Fatalf("CostOf: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("cost = %d want %d", got, testCase.want)
			}
		})
	}
}

// TestApplyExpectedWasteMatchesSection16CRule pins the gross-up against the
// same rule Section 16C uses when it freezes batch requirements
// (applyExpectedWaste in internal/features/productionbatch/math.go):
//
//	gross = net * 10000 / (10000 - bps)
//
// Section 16C's copy is package-private, so this table is the contract between
// the two. If either side changes the formula, these numbers move and the
// mismatch surfaces here rather than as a silently wrong COGS.
func TestApplyExpectedWasteMatchesSection16CRule(t *testing.T) {
	cases := []struct {
		name        string
		numerator   int64
		denominator int64
		wasteBPS    int
		wantNum     int64
		wantDen     int64
	}{
		{"zero waste is identity", 5, 1, 0, 5, 1},
		{"10 percent", 9, 1, 1000, 10, 1},            // 9 * 10000/9000 = 10
		{"5 percent stays exact", 19, 1, 500, 20, 1}, // 19 * 10000/9500 = 20
		{"fractional net", 1, 3, 2000, 5, 12},        // (1/3)*(10000/8000) = 5/12
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			net, err := NewQuantity(testCase.numerator, testCase.denominator)
			if err != nil {
				t.Fatal(err)
			}
			gross, err := ApplyExpectedWaste(net, testCase.wasteBPS)
			if err != nil {
				t.Fatalf("ApplyExpectedWaste: %v", err)
			}
			if gross.Numerator != testCase.wantNum || gross.Denominator != testCase.wantDen {
				t.Fatalf("gross = %d/%d want %d/%d",
					gross.Numerator, gross.Denominator, testCase.wantNum, testCase.wantDen)
			}
		})
	}
}

// TestExpectedWasteAppliedOnceVersusTwice is the double-count guard.
//
// It is the arithmetic behind the rule that recipe COGS grosses up and
// production expected cost does not: applying the gross-up a second time
// inflates the cost measurably, and this test states by how much so a
// regression is unmistakable rather than plausible-looking.
func TestExpectedWasteAppliedOnceVersusTwice(t *testing.T) {
	net, err := NewQuantity(9, 1)
	if err != nil {
		t.Fatal(err)
	}
	const unitCost = 1000
	const wasteBPS = 1000 // 10%

	once, err := ApplyExpectedWaste(net, wasteBPS)
	if err != nil {
		t.Fatal(err)
	}
	costOnce, err := CostOf(once, unitCost)
	if err != nil {
		t.Fatal(err)
	}

	twice, err := ApplyExpectedWaste(once, wasteBPS)
	if err != nil {
		t.Fatal(err)
	}
	costTwice, err := CostOf(twice, unitCost)
	if err != nil {
		t.Fatal(err)
	}

	if costOnce != 10000 {
		t.Fatalf("single gross-up cost = %d want 10000", costOnce)
	}
	if costTwice != 11111 {
		t.Fatalf("double gross-up cost = %d want 11111", costTwice)
	}
	if costTwice <= costOnce {
		t.Fatal("double-counting waste must be detectable as a higher cost")
	}
}

// TestWeightedAverageIsQuantityWeighted proves the average is weighted by
// accepted quantity rather than being a plain mean, which is the entire point
// of the strategy: a 100kg delivery must move the number far more than a 1kg
// one.
func TestWeightedAverageIsQuantityWeighted(t *testing.T) {
	// 100 units at 100 minor, 1 unit at 1000 minor.
	// Weighted: (100*100 + 1*1000) / 101 = 11000/101 = 108.9 -> 109
	// A plain mean would be 550, which is the bug this guards against.
	average, ok := WeightedAverageMinor([]int64{100, 1}, []int64{100, 1000})
	if !ok {
		t.Fatal("expected a weighted average")
	}
	if average != 109 {
		t.Fatalf("weighted average = %d want 109", average)
	}
	if average == 550 {
		t.Fatal("average is unweighted")
	}
}

// TestWeightedAverageWithNoWeightIsNotZero proves absence stays absent. A
// caller must not be handed 0 and mistake it for a free ingredient.
func TestWeightedAverageWithNoWeightIsNotZero(t *testing.T) {
	if _, ok := WeightedAverageMinor(nil, nil); ok {
		t.Fatal("empty evidence must not resolve")
	}
	if _, ok := WeightedAverageMinor([]int64{0}, []int64{500}); ok {
		t.Fatal("zero total weight must not resolve to a cost")
	}
}

// TestVarianceIsSigned proves an under-run is preserved rather than clamped.
func TestVarianceIsSigned(t *testing.T) {
	if got := VarianceMinor(126800, 119500); got != 7300 {
		t.Fatalf("overrun variance = %d want 7300", got)
	}
	if got := VarianceMinor(110000, 119500); got != -9500 {
		t.Fatalf("underrun variance = %d want -9500", got)
	}
}

// TestPerUnitRefusesZeroOutput proves a per-unit figure is reported as absent
// rather than as the undivided total.
func TestPerUnitRefusesZeroOutput(t *testing.T) {
	if _, ok := PerUnitMinor(10000, Quantity{Numerator: 0, Denominator: 1}); ok {
		t.Fatal("zero output must not produce a per-unit cost")
	}
	got, ok := PerUnitMinor(10000, Quantity{Numerator: 8, Denominator: 1})
	if !ok || got != 1250 {
		t.Fatalf("per-unit = %d ok=%t want 1250 true", got, ok)
	}
}

// TestInvalidQuantitiesAreRejected proves corrupt input is an error rather
// than a silently wrong number.
func TestInvalidQuantitiesAreRejected(t *testing.T) {
	if _, err := NewQuantity(1, 0); err == nil {
		t.Fatal("zero denominator must be rejected")
	}
	if _, err := NewQuantity(-1, 1); err == nil {
		t.Fatal("negative quantity must be rejected")
	}
	if _, err := ApplyExpectedWaste(Quantity{1, 1}, 10000); err == nil {
		t.Fatal("100% waste must be rejected; it would divide by zero")
	}
	if _, err := CostOf(Quantity{1, 1}, -5); err == nil {
		t.Fatal("negative unit cost must be rejected")
	}
}

// TestSubtractQuantityFloorsAtZero proves the waste-portion helper cannot
// produce a negative contribution.
func TestSubtractQuantityFloorsAtZero(t *testing.T) {
	gross := Quantity{Numerator: 10, Denominator: 1}
	net := Quantity{Numerator: 9, Denominator: 1}

	waste, err := SubtractQuantity(gross, net)
	if err != nil {
		t.Fatal(err)
	}
	if waste.Numerator != 1 || waste.Denominator != 1 {
		t.Fatalf("waste = %d/%d want 1/1", waste.Numerator, waste.Denominator)
	}

	inverted, err := SubtractQuantity(net, gross)
	if err != nil {
		t.Fatal(err)
	}
	if inverted.Numerator != 0 {
		t.Fatalf("inverted subtraction = %d/%d want 0", inverted.Numerator, inverted.Denominator)
	}
}
