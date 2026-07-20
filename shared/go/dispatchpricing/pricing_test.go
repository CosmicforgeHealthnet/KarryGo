package dispatchpricing

import (
	"math"
	"testing"
)

// These values are the contract shared by the api-gateway partner delivery API
// and the dispatch-delivery-service customer booking API. Changing them changes
// quoted fares on BOTH surfaces.
func TestFareKobo(t *testing.T) {
	cases := []struct {
		name string
		km   float64
		size string
		tier string
		want int64
	}{
		{"min fare short trip", 0.5, "small", "standard", 100000},
		{"5km small standard", 5, "small", "standard", 170000},
		{"5km medium standard", 5, "medium", "standard", 200000},
		{"5km large standard", 5, "large", "standard", 240000},
		{"5km small express", 5, "small", "express", 229500},
		{"unknown size defaults small", 5, "envelope", "standard", 170000},
		{"unknown tier defaults standard", 5, "small", "priority", 170000},
		{"tier case-insensitive", 5, "small", "EXPRESS", 229500},
		{"size case-insensitive with spaces", 5, " Medium ", "standard", 200000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FareKobo(tc.km, tc.size, tc.tier); got != tc.want {
				t.Fatalf("FareKobo(%v,%q,%q) = %d, want %d", tc.km, tc.size, tc.tier, got, tc.want)
			}
		})
	}
}

func TestFareRoundingAndFloor(t *testing.T) {
	// Every fare must be a multiple of FareRoundKobo and >= MinFareKobo.
	for _, km := range []float64{0, 0.3, 1.11, 2.77, 9.13, 25.5} {
		for _, size := range []string{"small", "medium", "large"} {
			for _, tier := range []string{"standard", "express"} {
				fare := FareKobo(km, size, tier)
				if fare%FareRoundKobo != 0 {
					t.Fatalf("fare %d not rounded to %d kobo (km=%v size=%s tier=%s)", fare, FareRoundKobo, km, size, tier)
				}
				if fare < MinFareKobo {
					t.Fatalf("fare %d below min %d (km=%v size=%s tier=%s)", fare, MinFareKobo, km, size, tier)
				}
			}
		}
	}
}

func TestBreakdown(t *testing.T) {
	b := Breakdown(5, "medium", "express")
	if b.BaseFareKobo != BaseFareKobo {
		t.Fatalf("base = %d", b.BaseFareKobo)
	}
	if b.DistanceFareKobo != 90000 {
		t.Fatalf("distance = %d, want 90000", b.DistanceFareKobo)
	}
	if b.SizeSurchargeKobo != MediumAddonKobo {
		t.Fatalf("size = %d", b.SizeSurchargeKobo)
	}
	// (80000 + 90000 + 30000) * 0.35 = 70000
	if b.ExpressSurchargeKobo != 70000 {
		t.Fatalf("express = %d, want 70000", b.ExpressSurchargeKobo)
	}
	if standard := Breakdown(5, "small", "standard"); standard.ExpressSurchargeKobo != 0 || standard.SizeSurchargeKobo != 0 {
		t.Fatalf("standard/small must carry no surcharges: %+v", standard)
	}
}

func TestETA(t *testing.T) {
	if got := ETALabel(1); got != "15 mins" { // floor
		t.Fatalf("ETALabel(1) = %q", got)
	}
	if got := ETALabel(9.1); got != "41 mins" { // ceil(9.1/18*60)+10 = 31+10
		t.Fatalf("ETALabel(9.1) = %q", got)
	}
}

func TestRoundKM(t *testing.T) {
	if got := RoundKM(9.14999); got != 9.1 {
		t.Fatalf("RoundKM = %v", got)
	}
}

func TestHaversineKm(t *testing.T) {
	// Surulere -> Ikeja is roughly 9km.
	km := HaversineKm(6.5244, 3.3792, 6.6018, 3.3515)
	if math.Abs(km-9.1) > 0.3 {
		t.Fatalf("HaversineKm = %v, want ~9.1", km)
	}
	if HaversineKm(6.5, 3.3, 6.5, 3.3) != 0 {
		t.Fatal("identical points must be 0")
	}
}
