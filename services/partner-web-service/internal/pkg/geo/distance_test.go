package geo

import (
	"math"
	"testing"
)

func TestHaversineKmKnownDistance(t *testing.T) {
	// Lagos: ~6.5244,3.3792 -> 6.6018,3.3515 is roughly 8-10 km.
	d := HaversineKm(6.5244, 3.3792, 6.6018, 3.3515)
	if d < 8 || d > 11 {
		t.Fatalf("distance = %.3f km, want ~8-11 km", d)
	}
}

func TestHaversineKmZeroForSamePoint(t *testing.T) {
	if d := HaversineKm(6.5, 3.3, 6.5, 3.3); math.Abs(d) > 1e-9 {
		t.Fatalf("same-point distance = %v, want 0", d)
	}
}

func TestHaversineKmSymmetric(t *testing.T) {
	a := HaversineKm(6.5244, 3.3792, 6.6018, 3.3515)
	b := HaversineKm(6.6018, 3.3515, 6.5244, 3.3792)
	if math.Abs(a-b) > 1e-9 {
		t.Fatalf("not symmetric: %v vs %v", a, b)
	}
}
