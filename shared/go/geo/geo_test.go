package geo

import (
	"math"
	"testing"
)

// Reference points used across the suite. Lagos coordinates keep the numbers
// in the range the platform actually serves.
var (
	ikeja       = Point{Lat: 6.6018, Lng: 3.3515}
	victoriaIsl = Point{Lat: 6.4281, Lng: 3.4219}
	lekki       = Point{Lat: 6.4698, Lng: 3.5852}
)

func TestHaversineKm(t *testing.T) {
	cases := []struct {
		name string
		a, b Point
		want float64 // kilometres
		tol  float64
	}{
		{"same point", ikeja, ikeja, 0, 0},
		{"ikeja to victoria island", ikeja, victoriaIsl, 20.8217, 0.01},
		{"victoria island to lekki", victoriaIsl, lekki, 18.6295, 0.01},
		{"symmetric", victoriaIsl, ikeja, 20.8217, 0.01},
		{"one degree of latitude", Point{Lat: 0, Lng: 0}, Point{Lat: 1, Lng: 0}, KmPerDegreeLat, 0.001},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := HaversineKm(tc.a, tc.b)
			if math.Abs(got-tc.want) > tc.tol {
				t.Fatalf("HaversineKm(%v, %v) = %v, want %v (+/- %v)", tc.a, tc.b, got, tc.want, tc.tol)
			}
		})
	}
}

// The four services this package replaces all hardcoded R = 6371 km with the
// same formula. Delegation must not shift a single fare or matching decision,
// so pin the constant explicitly.
func TestHaversineMatchesLegacyEarthRadius(t *testing.T) {
	if EarthRadiusKm != 6371.0 {
		t.Fatalf("EarthRadiusKm = %v, want 6371 (the value every service previously inlined)", EarthRadiusKm)
	}
	if got, want := HaversineMeters(ikeja, victoriaIsl), HaversineKm(ikeja, victoriaIsl)*1000; got != want {
		t.Fatalf("HaversineMeters = %v, want %v", got, want)
	}
}

func TestBearingDegrees(t *testing.T) {
	origin := Point{Lat: 0, Lng: 0}
	cases := []struct {
		name string
		to   Point
		want float64
	}{
		{"due north", Point{Lat: 1, Lng: 0}, 0},
		{"due east", Point{Lat: 0, Lng: 1}, 90},
		{"due south", Point{Lat: -1, Lng: 0}, 180},
		{"due west", Point{Lat: 0, Lng: -1}, 270},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BearingDegrees(origin, tc.to)
			if math.Abs(got-tc.want) > 0.01 {
				t.Fatalf("BearingDegrees(%v, %v) = %v, want %v", origin, tc.to, got, tc.want)
			}
		})
	}
}

func TestNormalizeBearing(t *testing.T) {
	cases := map[float64]float64{
		0: 0, 359: 359, 360: 0, 361: 1, -1: 359, -90: 270, 720: 0,
	}
	for in, want := range cases {
		if got := NormalizeBearing(in); math.Abs(got-want) > 1e-9 {
			t.Fatalf("NormalizeBearing(%v) = %v, want %v", in, got, want)
		}
	}
}

// angularDiff returns the shortest distance between two compass bearings, so
// 359.999 and 0 read as adjacent rather than 360 apart.
func angularDiff(a, b float64) float64 {
	d := math.Abs(NormalizeBearing(a) - NormalizeBearing(b))
	if d > 180 {
		d = 360 - d
	}
	return d
}

func TestDestinationRoundTrips(t *testing.T) {
	for _, bearing := range []float64{0, 45, 90, 180, 270, 315} {
		for _, distKm := range []float64{0.1, 1, 25} {
			got := Destination(ikeja, bearing, distKm)

			if d := HaversineKm(ikeja, got); math.Abs(d-distKm) > distKm*0.001 {
				t.Fatalf("Destination(%v, %v, %v) landed %v km away, want %v", ikeja, bearing, distKm, d, distKm)
			}
			// Compare as angles: a due-north bearing round-trips to either
			// 0 or a hair under 360, and both are correct.
			if b := BearingDegrees(ikeja, got); angularDiff(b, bearing) > 0.1 {
				t.Fatalf("Destination(%v, %v, %v) bearing = %v, want %v", ikeja, bearing, distKm, b, bearing)
			}
		}
	}
}

func TestBoundsAroundEnclosesCircle(t *testing.T) {
	const radiusKm = 5
	box := BoundsAround(ikeja, radiusKm)

	// Every point exactly on the circle must fall inside the box.
	for bearing := 0.0; bearing < 360; bearing += 15 {
		edge := Destination(ikeja, bearing, radiusKm)
		if !box.Contains(edge) {
			t.Fatalf("BoundsAround(%v, %v) excludes edge point %v at bearing %v", ikeja, radiusKm, edge, bearing)
		}
	}
	if !box.Contains(ikeja) {
		t.Fatalf("BoundsAround(%v, %v) excludes its own centre", ikeja, radiusKm)
	}
}

func TestPointValidity(t *testing.T) {
	cases := []struct {
		name             string
		p                Point
		valid, zero, fix bool
	}{
		{"lagos", ikeja, true, false, true},
		{"null island", Point{}, true, true, false},
		{"latitude out of range", Point{Lat: 91, Lng: 3}, false, false, false},
		{"longitude out of range", Point{Lat: 6, Lng: 181}, false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.Valid(); got != tc.valid {
				t.Fatalf("Valid() = %v, want %v", got, tc.valid)
			}
			if got := tc.p.IsZero(); got != tc.zero {
				t.Fatalf("IsZero() = %v, want %v", got, tc.zero)
			}
			if got := tc.p.HasFix(); got != tc.fix {
				t.Fatalf("HasFix() = %v, want %v", got, tc.fix)
			}
		})
	}
}

func TestLerp(t *testing.T) {
	a := Point{Lat: 0, Lng: 0}
	b := Point{Lat: 10, Lng: 20}

	cases := []struct {
		t    float64
		want Point
	}{
		{-1, a}, {0, a}, {0.5, Point{Lat: 5, Lng: 10}}, {1, b}, {2, b},
	}
	for _, tc := range cases {
		if got := Lerp(a, b, tc.t); got != tc.want {
			t.Fatalf("Lerp(%v, %v, %v) = %v, want %v", a, b, tc.t, got, tc.want)
		}
	}
}

func TestWrapLng(t *testing.T) {
	cases := map[float64]float64{
		0: 0, 179: 179, -180: -180, 180: -180, 181: -179, -181: 179, 540: -180,
	}
	for in, want := range cases {
		if got := wrapLng(in); math.Abs(got-want) > 1e-9 {
			t.Fatalf("wrapLng(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestPointInPolygon(t *testing.T) {
	square := []Point{
		{Lat: 0, Lng: 0}, {Lat: 0, Lng: 10}, {Lat: 10, Lng: 10}, {Lat: 10, Lng: 0},
	}
	cases := []struct {
		name string
		p    Point
		want bool
	}{
		{"centre", Point{Lat: 5, Lng: 5}, true},
		{"outside east", Point{Lat: 5, Lng: 15}, false},
		{"outside north", Point{Lat: 15, Lng: 5}, false},
		{"negative", Point{Lat: -1, Lng: -1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PointInPolygon(tc.p, square); got != tc.want {
				t.Fatalf("PointInPolygon(%v) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}

	if PointInPolygon(Point{Lat: 5, Lng: 5}, square[:2]) {
		t.Fatal("PointInPolygon with fewer than 3 vertices should be false")
	}
}
