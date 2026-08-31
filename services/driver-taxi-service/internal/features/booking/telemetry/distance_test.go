package bookingtelemetry

import (
	"math"
	"testing"

	"cosmicforge/logistics/shared/go/geo"
)

func TestActualDistanceKm_FewerThanTwoPointsIsNotDerivable(t *testing.T) {
	if _, ok := ActualDistanceKm(nil); ok {
		t.Error("ok = true for a nil trail, want false")
	}
	if _, ok := ActualDistanceKm([]Point{{Lat: 6.5, Lng: 3.4}}); ok {
		t.Error("ok = true for a single point, want false")
	}
}

func TestActualDistanceKm_SumsConsecutiveHaversineDistances(t *testing.T) {
	trail := []Point{
		{Sequence: 1, Lat: 6.5244, Lng: 3.3792}, // Ikeja
		{Sequence: 2, Lat: 6.5600, Lng: 3.3900},
		{Sequence: 3, Lat: 6.6000, Lng: 3.4000}, // Victoria Island-ish
	}
	got, ok := ActualDistanceKm(trail)
	if !ok {
		t.Fatal("ok = false, want true for a 3-point trail")
	}
	want := geo.HaversineKm(geo.Point{Lat: trail[0].Lat, Lng: trail[0].Lng}, geo.Point{Lat: trail[1].Lat, Lng: trail[1].Lng}) +
		geo.HaversineKm(geo.Point{Lat: trail[1].Lat, Lng: trail[1].Lng}, geo.Point{Lat: trail[2].Lat, Lng: trail[2].Lng})
	if math.Abs(got-want) > 0.0001 {
		t.Errorf("ActualDistanceKm = %v, want %v (sum of consecutive legs)", got, want)
	}
}

func TestActualDistanceKm_TwoIdenticalPointsIsZero(t *testing.T) {
	trail := []Point{
		{Sequence: 1, Lat: 6.5, Lng: 3.4},
		{Sequence: 2, Lat: 6.5, Lng: 3.4},
	}
	got, ok := ActualDistanceKm(trail)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got != 0 {
		t.Errorf("ActualDistanceKm = %v, want 0 for two identical points", got)
	}
}
