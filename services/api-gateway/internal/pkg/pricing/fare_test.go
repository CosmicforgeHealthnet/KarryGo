package pricing

import "testing"

var testCfg = Config{
	BaseKobo:          50000,
	StandardPerKmKobo: 15000,
	ExpressPerKmKobo:  25000,
	LargeMultiplier:   1.5,
}

func TestStandardFareIsBasePlusDistanceRate(t *testing.T) {
	// 10 km standard: 50000 + 10*15000 = 200000
	got := CalculateFare(testCfg, 10, "standard", "small")
	if got != 200000 {
		t.Fatalf("standard fare = %d, want 200000", got)
	}
}

func TestExpressIsHigherThanStandard(t *testing.T) {
	std := CalculateFare(testCfg, 10, "standard", "small")
	exp := CalculateFare(testCfg, 10, "express", "small")
	if exp <= std {
		t.Fatalf("express %d must be > standard %d", exp, std)
	}
	// 50000 + 10*25000 = 300000
	if exp != 300000 {
		t.Fatalf("express fare = %d, want 300000", exp)
	}
}

func TestLargeAppliesMultiplier(t *testing.T) {
	small := CalculateFare(testCfg, 10, "standard", "small")
	large := CalculateFare(testCfg, 10, "standard", "large")
	// 200000 * 1.5 = 300000
	if large != int64(float64(small)*testCfg.LargeMultiplier) {
		t.Fatalf("large fare = %d, want %d", large, int64(float64(small)*testCfg.LargeMultiplier))
	}
}

func TestFareNeverBelowBase(t *testing.T) {
	// zero distance -> base fare
	if got := CalculateFare(testCfg, 0, "standard", "small"); got != testCfg.BaseKobo {
		t.Fatalf("zero-distance fare = %d, want base %d", got, testCfg.BaseKobo)
	}
	// negative distance is clamped up to base
	if got := CalculateFare(testCfg, -5, "standard", "small"); got < testCfg.BaseKobo {
		t.Fatalf("fare %d below base %d", got, testCfg.BaseKobo)
	}
}
