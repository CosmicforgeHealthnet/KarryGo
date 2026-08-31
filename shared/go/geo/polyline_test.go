package geo

import (
	"errors"
	"math"
	"testing"
)

// The canonical example from Google's encoded-polyline specification. If this
// ever fails, every route polyline the Routes API returns is being decoded to
// the wrong place.
func TestDecodePolylineGoogleReferenceVector(t *testing.T) {
	const encoded = "_p~iF~ps|U_ulLnnqC_mqNvxq`@"
	want := []Point{
		{Lat: 38.5, Lng: -120.2},
		{Lat: 40.7, Lng: -120.95},
		{Lat: 43.252, Lng: -126.453},
	}

	got, err := DecodePolyline(encoded)
	if err != nil {
		t.Fatalf("DecodePolyline(%q) returned error: %v", encoded, err)
	}
	if len(got) != len(want) {
		t.Fatalf("DecodePolyline(%q) returned %d points, want %d", encoded, len(got), len(want))
	}
	for i := range want {
		if math.Abs(got[i].Lat-want[i].Lat) > 1e-6 || math.Abs(got[i].Lng-want[i].Lng) > 1e-6 {
			t.Fatalf("point %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestEncodePolylineGoogleReferenceVector(t *testing.T) {
	const want = "_p~iF~ps|U_ulLnnqC_mqNvxq`@"
	points := []Point{
		{Lat: 38.5, Lng: -120.2},
		{Lat: 40.7, Lng: -120.95},
		{Lat: 43.252, Lng: -126.453},
	}

	if got := EncodePolyline(points); got != want {
		t.Fatalf("EncodePolyline(%v) = %q, want %q", points, got, want)
	}
}

func TestPolylineRoundTrip(t *testing.T) {
	route := []Point{ikeja, {Lat: 6.5244, Lng: 3.3792}, victoriaIsl, lekki}

	decoded, err := DecodePolyline(EncodePolyline(route))
	if err != nil {
		t.Fatalf("round trip returned error: %v", err)
	}
	if len(decoded) != len(route) {
		t.Fatalf("round trip returned %d points, want %d", len(decoded), len(route))
	}
	for i := range route {
		// Precision 5 keeps roughly 1 m of resolution.
		if math.Abs(decoded[i].Lat-route[i].Lat) > 1e-5 || math.Abs(decoded[i].Lng-route[i].Lng) > 1e-5 {
			t.Fatalf("point %d = %v, want %v", i, decoded[i], route[i])
		}
	}
}

func TestDecodePolylineEmptyIsNotAnError(t *testing.T) {
	// An unset route column must not read as a failure.
	got, err := DecodePolyline("")
	if err != nil {
		t.Fatalf("DecodePolyline(\"\") returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("DecodePolyline(\"\") = %v, want nil", got)
	}
	if got := EncodePolyline(nil); got != "" {
		t.Fatalf("EncodePolyline(nil) = %q, want empty", got)
	}
}

func TestDecodePolylineRejectsMalformedInput(t *testing.T) {
	cases := map[string]string{
		"truncated mid value":        "_p~iF",
		"latitude without longitude": "_p~iF~ps|U_ulL",
		"continuation byte at end":   "_p~iF~ps|U_",
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodePolyline(encoded); !errors.Is(err, ErrInvalidPolyline) {
				t.Fatalf("DecodePolyline(%q) error = %v, want ErrInvalidPolyline", encoded, err)
			}
		})
	}
}
