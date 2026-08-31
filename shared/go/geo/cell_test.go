package geo

import (
	"errors"
	"testing"
)

// Published geohash vectors. These are the same values Redis GEOHASH reports
// for the coordinates, which is what makes the cell ids interchangeable with
// what dispatch's GEO set already stores.
func TestEncodeKnownVectors(t *testing.T) {
	cases := []struct {
		name      string
		p         Point
		precision int
		want      Cell
	}{
		{"null island", Point{Lat: 0, Lng: 0}, 5, "s0000"},
		{"wikipedia worked example", Point{Lat: 42.6, Lng: -5.6}, 5, "ezs42"},
		{"murmansk", Point{Lat: 68.9704, Lng: 33.0925}, 12, "usr8359jxnkm"},
		{"jutland", Point{Lat: 57.64911, Lng: 10.40744}, 11, "u4pruydqqvj"},
		{"jutland truncated", Point{Lat: 57.64911, Lng: 10.40744}, 5, "u4pru"},
		{"san francisco", Point{Lat: 37.7749, Lng: -122.4194}, 7, "9q8yyk8"},
		{"precision clamped low", Point{Lat: 0, Lng: 0}, 0, "s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Encode(tc.p, tc.precision); got != tc.want {
				t.Fatalf("Encode(%v, %d) = %q, want %q", tc.p, tc.precision, got, tc.want)
			}
		})
	}
}

// Hierarchy is what lets a coarse cell act as the parent of every finer cell
// inside it, and is why PickPrecision can trade resolution for read budget.
func TestCellsArePrefixHierarchical(t *testing.T) {
	fine := Encode(ikeja, 9)
	for precision := 1; precision <= 9; precision++ {
		coarse := Encode(ikeja, precision)
		if got := fine.Parent(precision); got != coarse {
			t.Fatalf("Parent(%d) of %q = %q, want %q", precision, fine, got, coarse)
		}
		if fine[:precision] != coarse {
			t.Fatalf("%q is not a prefix of %q", coarse, fine)
		}
	}
	if got := fine.Parent(20); got != fine {
		t.Fatalf("Parent beyond own precision = %q, want %q", got, fine)
	}
	if got := fine.Precision(); got != 9 {
		t.Fatalf("Precision() = %d, want 9", got)
	}
}

func TestDecodeBoundsContainsEncodedPoint(t *testing.T) {
	for _, p := range []Point{ikeja, victoriaIsl, lekki, {Lat: 0, Lng: 0}, {Lat: -33.86, Lng: 151.2}} {
		for _, precision := range []int{1, 4, 5, 7, 9, 12} {
			cell := Encode(p, precision)
			bounds, err := DecodeBounds(cell)
			if err != nil {
				t.Fatalf("DecodeBounds(%q) returned error: %v", cell, err)
			}
			if !bounds.Contains(p) {
				t.Fatalf("cell %q (precision %d) does not contain its own point %v", cell, precision, p)
			}
			// The centre of a cell must re-encode to that same cell.
			center, err := Decode(cell)
			if err != nil {
				t.Fatalf("Decode(%q) returned error: %v", cell, err)
			}
			if got := Encode(center, precision); got != cell {
				t.Fatalf("Encode(centre of %q) = %q, want %q", cell, got, cell)
			}
		}
	}
}

func TestDecodeRejectsInvalidCells(t *testing.T) {
	// 'a', 'i', 'l' and 'o' are excluded from the geohash alphabet.
	for _, cell := range []Cell{"", "abc", "u4pri", "u4prl", "u4pro", "0123456789abc"} {
		if _, err := DecodeBounds(cell); !errors.Is(err, ErrInvalidCell) {
			t.Fatalf("DecodeBounds(%q) error = %v, want ErrInvalidCell", cell, err)
		}
	}
}

func TestDecodeIsCaseInsensitive(t *testing.T) {
	lower, err := DecodeBounds("u4pru")
	if err != nil {
		t.Fatalf("DecodeBounds returned error: %v", err)
	}
	upper, err := DecodeBounds("U4PRU")
	if err != nil {
		t.Fatalf("DecodeBounds returned error: %v", err)
	}
	if lower != upper {
		t.Fatalf("case changed the bounds: %v vs %v", lower, upper)
	}
}

func TestNeighbors(t *testing.T) {
	cell := Encode(ikeja, 6)
	neighbors, err := Neighbors(cell)
	if err != nil {
		t.Fatalf("Neighbors(%q) returned error: %v", cell, err)
	}
	if len(neighbors) != 8 {
		t.Fatalf("Neighbors(%q) returned %d cells, want 8", cell, len(neighbors))
	}

	seen := map[Cell]struct{}{}
	for _, n := range neighbors {
		if n == cell {
			t.Fatalf("Neighbors(%q) included the cell itself", cell)
		}
		if _, dup := seen[n]; dup {
			t.Fatalf("Neighbors(%q) returned %q twice", cell, n)
		}
		seen[n] = struct{}{}
		if n.Precision() != cell.Precision() {
			t.Fatalf("neighbour %q has precision %d, want %d", n, n.Precision(), cell.Precision())
		}
	}

	// Every neighbour must be adjacent: its centre lies within two cell
	// diagonals of the origin cell's centre.
	origin, _ := Decode(cell)
	bounds, _ := DecodeBounds(cell)
	maxStep := HaversineKm(bounds.Min, bounds.Max) * 2
	for _, n := range neighbors {
		c, _ := Decode(n)
		if d := HaversineKm(origin, c); d > maxStep {
			t.Fatalf("neighbour %q centre is %v km away, want <= %v", n, d, maxStep)
		}
	}
}

// The cover must be a genuine superset of the circle, otherwise proximity
// queries would silently miss providers sitting near the radius edge.
func TestCoverRadiusIsSuperset(t *testing.T) {
	cases := []struct {
		radiusKm  float64
		precision int
	}{
		{1, FinePrecision},
		{5, FinePrecision},
		{25, CoarsePrecision},
		{50, CoarsePrecision},
	}

	for _, tc := range cases {
		cover := CoverRadius(ikeja, tc.radiusKm, tc.precision)
		covered := make(map[Cell]struct{}, len(cover))
		for _, c := range cover {
			covered[c] = struct{}{}
		}

		if _, ok := covered[Encode(ikeja, tc.precision)]; !ok {
			t.Fatalf("cover of %v km at precision %d omits the centre cell", tc.radiusKm, tc.precision)
		}

		// Sample the disc densely: every point inside the radius must live
		// in a cell the cover returned.
		for bearing := 0.0; bearing < 360; bearing += 10 {
			for _, frac := range []float64{0.25, 0.5, 0.75, 0.999} {
				p := Destination(ikeja, bearing, tc.radiusKm*frac)
				if _, ok := covered[Encode(p, tc.precision)]; !ok {
					t.Fatalf("cover of %v km at precision %d omits %v (bearing %v, %v of radius)",
						tc.radiusKm, tc.precision, p, bearing, frac)
				}
			}
		}
	}
}

func TestCoverCellCountMatchesCoverRadius(t *testing.T) {
	for _, radiusKm := range []float64{0, 1, 5, 25, 50} {
		for _, precision := range []int{CoarsePrecision, FinePrecision} {
			want := len(CoverRadius(ikeja, radiusKm, precision))
			if got := CoverCellCount(ikeja, radiusKm, precision); got != want {
				t.Fatalf("CoverCellCount(%v km, precision %d) = %d, want %d", radiusKm, precision, got, want)
			}
		}
	}
}

// PickPrecision is what keeps a 50 km sweep from turning into hundreds of
// Redis reads while still using fine cells for a 5 km one.
func TestPickPrecision(t *testing.T) {
	candidates := []int{CoarsePrecision, FinePrecision}

	cases := []struct {
		name     string
		radiusKm float64
		want     int
	}{
		{"short radius uses fine cells", 1, FinePrecision},
		{"dispatch first broadcast ring", 5, FinePrecision},
		{"hauling match radius falls back to coarse", 25, CoarsePrecision},
		{"dispatch nearby cap stays coarse", 50, CoarsePrecision},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PickPrecision(ikeja, tc.radiusKm, candidates, DefaultMaxCoverCells)
			if got != tc.want {
				t.Fatalf("PickPrecision(%v km) = %d, want %d (cover would be %d cells)",
					tc.radiusKm, got, tc.want, CoverCellCount(ikeja, tc.radiusKm, got))
			}
			if n := CoverCellCount(ikeja, tc.radiusKm, got); n > DefaultMaxCoverCells {
				t.Fatalf("PickPrecision(%v km) chose %d, which covers %d cells, over the %d budget",
					tc.radiusKm, got, n, DefaultMaxCoverCells)
			}
		})
	}
}

func TestPickPrecisionFallsBackToCoarsestWhenNothingFits(t *testing.T) {
	// A radius so large no candidate fits the budget: take the coarsest
	// rather than returning nothing.
	if got := PickPrecision(ikeja, 5000, []int{CoarsePrecision, FinePrecision}, 4); got != CoarsePrecision {
		t.Fatalf("PickPrecision with an impossible budget = %d, want %d", got, CoarsePrecision)
	}
	if got := PickPrecision(ikeja, 5, nil, DefaultMaxCoverCells); got != FinePrecision {
		t.Fatalf("PickPrecision with no candidates = %d, want %d", got, FinePrecision)
	}
}

func TestCellDimensionsKm(t *testing.T) {
	// Coarse cells must be materially bigger than fine ones, which is the
	// whole premise of the two-precision index.
	coarseW, coarseH := CellDimensionsKm(CoarsePrecision)
	fineW, fineH := CellDimensionsKm(FinePrecision)

	if coarseW <= fineW || coarseH <= fineH {
		t.Fatalf("coarse cell %vx%v km is not larger than fine cell %vx%v km", coarseW, coarseH, fineW, fineH)
	}
}
