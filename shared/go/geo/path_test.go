package geo

import (
	"math"
	"testing"
)

// A straight west-to-east leg at the equator, one degree wide, so the expected
// distances are exact multiples of a known constant.
func equatorPath() []Point {
	return []Point{
		{Lat: 0, Lng: 0},
		{Lat: 0, Lng: 0.5},
		{Lat: 0, Lng: 1.0},
	}
}

func TestPathLengthKm(t *testing.T) {
	cases := []struct {
		name string
		path []Point
		want float64
		tol  float64
	}{
		{"nil", nil, 0, 0},
		{"single point", []Point{ikeja}, 0, 0},
		{"one degree of longitude at equator", equatorPath(), KmPerDegreeLat, 0.01},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PathLengthKm(tc.path); math.Abs(got-tc.want) > tc.tol {
				t.Fatalf("PathLengthKm = %v, want %v (+/- %v)", got, tc.want, tc.tol)
			}
		})
	}
}

func TestSnapToPath(t *testing.T) {
	path := equatorPath()
	total := PathLengthKm(path)

	cases := []struct {
		name        string
		p           Point
		wantSegment int
		wantAlongKm float64
		wantOffKm   float64
		alongTol    float64
		offTol      float64
	}{
		{
			name:        "on the first vertex",
			p:           Point{Lat: 0, Lng: 0},
			wantSegment: 0, wantAlongKm: 0, wantOffKm: 0,
			alongTol: 0.01, offTol: 0.01,
		},
		{
			name:        "midway along the first segment",
			p:           Point{Lat: 0, Lng: 0.25},
			wantSegment: 0, wantAlongKm: total / 4, wantOffKm: 0,
			alongTol: 0.05, offTol: 0.01,
		},
		{
			name:        "on the final vertex",
			p:           Point{Lat: 0, Lng: 1.0},
			wantSegment: 1, wantAlongKm: total, wantOffKm: 0,
			alongTol: 0.05, offTol: 0.01,
		},
		{
			name:        "offset north of the line",
			p:           Point{Lat: 0.01, Lng: 0.5},
			wantSegment: 0, wantAlongKm: total / 2, wantOffKm: 0.01 * KmPerDegreeLat,
			alongTol: 0.05, offTol: 0.02,
		},
		{
			name:        "before the start clamps to the start",
			p:           Point{Lat: 0, Lng: -0.5},
			wantSegment: 0, wantAlongKm: 0, wantOffKm: 0.5 * KmPerDegreeLat,
			alongTol: 0.05, offTol: 0.5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proj, ok := SnapToPath(tc.p, path)
			if !ok {
				t.Fatalf("SnapToPath(%v) reported no projection", tc.p)
			}
			if proj.SegmentIndex != tc.wantSegment {
				t.Fatalf("SegmentIndex = %d, want %d", proj.SegmentIndex, tc.wantSegment)
			}
			if math.Abs(proj.AlongKm-tc.wantAlongKm) > tc.alongTol {
				t.Fatalf("AlongKm = %v, want %v (+/- %v)", proj.AlongKm, tc.wantAlongKm, tc.alongTol)
			}
			if math.Abs(proj.OffPathKm-tc.wantOffKm) > tc.offTol {
				t.Fatalf("OffPathKm = %v, want %v (+/- %v)", proj.OffPathKm, tc.wantOffKm, tc.offTol)
			}
		})
	}
}

func TestSnapToPathNeedsTwoPoints(t *testing.T) {
	for _, path := range [][]Point{nil, {}, {ikeja}} {
		if _, ok := SnapToPath(ikeja, path); ok {
			t.Fatalf("SnapToPath with %d points should report false", len(path))
		}
	}
}

// The progress bar reads this directly: a vehicle a quarter of the way along
// its route must report 0.25, not a straight-line ratio.
func TestProgressAlongPath(t *testing.T) {
	// An L-shaped route: straight-line and along-route progress differ.
	path := []Point{
		{Lat: 0, Lng: 0},
		{Lat: 0, Lng: 1},
		{Lat: 1, Lng: 1},
	}

	cases := []struct {
		name string
		p    Point
		want float64
		tol  float64
	}{
		{"at start", Point{Lat: 0, Lng: 0}, 0, 0.01},
		{"quarter along", Point{Lat: 0, Lng: 0.5}, 0.25, 0.01},
		{"at the corner", Point{Lat: 0, Lng: 1}, 0.5, 0.01},
		{"three quarters along", Point{Lat: 0.5, Lng: 1}, 0.75, 0.01},
		{"at end", Point{Lat: 1, Lng: 1}, 1, 0.01},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ProgressAlongPath(tc.p, path)
			if !ok {
				t.Fatalf("ProgressAlongPath(%v) reported no projection", tc.p)
			}
			if math.Abs(got-tc.want) > tc.tol {
				t.Fatalf("ProgressAlongPath(%v) = %v, want %v (+/- %v)", tc.p, got, tc.want, tc.tol)
			}
		})
	}
}

func TestRemainingKm(t *testing.T) {
	path := equatorPath()
	total := PathLengthKm(path)

	got, ok := RemainingKm(Point{Lat: 0, Lng: 0.25}, path)
	if !ok {
		t.Fatal("RemainingKm reported no projection")
	}
	if want := total * 0.75; math.Abs(got-want) > 0.05 {
		t.Fatalf("RemainingKm = %v, want %v", got, want)
	}

	// Past the end must clamp to zero rather than go negative.
	got, ok = RemainingKm(Point{Lat: 0, Lng: 2}, path)
	if !ok {
		t.Fatal("RemainingKm reported no projection past the end")
	}
	if got != 0 {
		t.Fatalf("RemainingKm past the end = %v, want 0", got)
	}
}
