package geo

import (
	"errors"
	"math"
	"strings"
)

// Cell is a geohash string: a base32 encoding of interleaved latitude and
// longitude bits. Two properties make it the spatial partition key for live
// vehicle positions:
//
//   - it is hierarchical, so a shorter cell is the parent of every cell that
//     has it as a prefix; and
//   - cells of a given length tile the world as a regular latitude/longitude
//     grid, so a circle can be covered by walking that grid.
//
// It is also the encoding Redis GEO uses internally, so these cell ids line up
// with what a GEOHASH command would report for the same coordinate.
type Cell string

const (
	// MinPrecision is the shortest usable cell (~5000 km).
	MinPrecision = 1
	// MaxPrecision is the longest cell this package encodes (~4 cm), well
	// past the resolution of any consumer GPS.
	MaxPrecision = 12

	// CoarsePrecision cells are roughly 39 x 20 km — the right grain for
	// wide matching sweeps (hauling's 25 km radius, dispatch's 50 km cap).
	CoarsePrecision = 4
	// FinePrecision cells are roughly 4.9 x 4.9 km — the right grain for
	// short-radius lookups such as dispatch's 5 km first broadcast ring.
	FinePrecision = 5

	// DefaultMaxCoverCells caps how many cells one proximity query may read.
	// PickPrecision trades resolution for staying inside this budget.
	DefaultMaxCoverCells = 64
)

const base32Alphabet = "0123456789bcdefghjkmnpqrstuvwxyz"

// ErrInvalidCell is returned when a string is not a valid geohash.
var ErrInvalidCell = errors.New("geo: invalid geohash cell")

// cellSizeKm lists the approximate (width, height) of a cell at each
// precision, at the equator. Widths shrink with cos(latitude); the cover
// helpers account for that via BoundsAround.
var cellSizeKm = map[int][2]float64{
	1:  {5009.4, 4992.6},
	2:  {1252.3, 624.1},
	3:  {156.5, 156.0},
	4:  {39.1, 19.5},
	5:  {4.9, 4.9},
	6:  {1.2, 0.61},
	7:  {0.153, 0.153},
	8:  {0.0382, 0.0191},
	9:  {0.00477, 0.00477},
	10: {0.00119, 0.000596},
	11: {0.000149, 0.000149},
	12: {0.0000372, 0.0000186},
}

// Precision returns how many characters the cell has.
func (c Cell) Precision() int { return len(c) }

// Parent truncates the cell to a shorter precision. A precision at or above
// the cell's own length returns the cell unchanged.
func (c Cell) Parent(precision int) Cell {
	if precision < MinPrecision {
		precision = MinPrecision
	}
	if precision >= len(c) {
		return c
	}
	return c[:precision]
}

// Encode returns the geohash cell of the given precision containing p.
func Encode(p Point, precision int) Cell {
	precision = clampPrecision(precision)
	p = Normalize(p)

	latMin, latMax := -90.0, 90.0
	lngMin, lngMax := -180.0, 180.0

	var buf strings.Builder
	buf.Grow(precision)

	even := true // longitude bits come first
	bit, chunk := 0, 0

	for buf.Len() < precision {
		if even {
			mid := (lngMin + lngMax) / 2
			if p.Lng >= mid {
				chunk = chunk<<1 | 1
				lngMin = mid
			} else {
				chunk <<= 1
				lngMax = mid
			}
		} else {
			mid := (latMin + latMax) / 2
			if p.Lat >= mid {
				chunk = chunk<<1 | 1
				latMin = mid
			} else {
				chunk <<= 1
				latMax = mid
			}
		}

		even = !even
		bit++
		if bit == 5 {
			buf.WriteByte(base32Alphabet[chunk])
			bit, chunk = 0, 0
		}
	}
	return Cell(buf.String())
}

// DecodeBounds returns the latitude/longitude rectangle the cell covers.
func DecodeBounds(c Cell) (Bounds, error) {
	if len(c) == 0 || len(c) > MaxPrecision {
		return Bounds{}, ErrInvalidCell
	}

	latMin, latMax := -90.0, 90.0
	lngMin, lngMax := -180.0, 180.0
	even := true

	for i := 0; i < len(c); i++ {
		idx := strings.IndexByte(base32Alphabet, lowerASCII(c[i]))
		if idx < 0 {
			return Bounds{}, ErrInvalidCell
		}
		for b := 4; b >= 0; b-- {
			set := (idx>>b)&1 == 1
			if even {
				mid := (lngMin + lngMax) / 2
				if set {
					lngMin = mid
				} else {
					lngMax = mid
				}
			} else {
				mid := (latMin + latMax) / 2
				if set {
					latMin = mid
				} else {
					latMax = mid
				}
			}
			even = !even
		}
	}

	return Bounds{
		Min: Point{Lat: latMin, Lng: lngMin},
		Max: Point{Lat: latMax, Lng: lngMax},
	}, nil
}

// Decode returns the centre point of the cell.
func Decode(c Cell) (Point, error) {
	b, err := DecodeBounds(c)
	if err != nil {
		return Point{}, err
	}
	return b.Center(), nil
}

// Neighbors returns the up-to-eight cells surrounding c at the same precision.
// Near a pole several offsets collapse onto the same cell, so the result is
// deduplicated and c itself is never included.
func Neighbors(c Cell) ([]Cell, error) {
	b, err := DecodeBounds(c)
	if err != nil {
		return nil, err
	}

	center := b.Center()
	dLat := b.Max.Lat - b.Min.Lat
	dLng := b.Max.Lng - b.Min.Lng
	precision := len(c)

	offsets := [8][2]float64{
		{1, -1}, {1, 0}, {1, 1},
		{0, -1}, {0, 1},
		{-1, -1}, {-1, 0}, {-1, 1},
	}

	seen := map[Cell]struct{}{c: {}}
	out := make([]Cell, 0, len(offsets))
	for _, o := range offsets {
		n := Encode(Point{
			Lat: center.Lat + o[0]*dLat,
			Lng: center.Lng + o[1]*dLng,
		}, precision)
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out, nil
}

// CoverRadius returns every cell of the given precision that overlaps the
// circle of radiusKm around center. The cover is a superset of the circle, so
// callers still filter results by true distance.
func CoverRadius(center Point, radiusKm float64, precision int) []Cell {
	precision = clampPrecision(precision)
	rows, cols, anchor, dLat, dLng := coverGrid(center, radiusKm, precision)

	seen := make(map[Cell]struct{}, (2*rows+1)*(2*cols+1))
	out := make([]Cell, 0, (2*rows+1)*(2*cols+1))

	for i := -rows; i <= rows; i++ {
		for j := -cols; j <= cols; j++ {
			cell := Encode(Point{
				Lat: anchor.Lat + float64(i)*dLat,
				Lng: anchor.Lng + float64(j)*dLng,
			}, precision)
			if _, dup := seen[cell]; dup {
				continue
			}
			seen[cell] = struct{}{}
			out = append(out, cell)
		}
	}
	return out
}

// CoverCellCount reports how many cells CoverRadius would return, without
// building them. PickPrecision uses it to stay inside a read budget.
func CoverCellCount(center Point, radiusKm float64, precision int) int {
	rows, cols, _, _, _ := coverGrid(center, radiusKm, clampPrecision(precision))
	return (2*rows + 1) * (2*cols + 1)
}

// PickPrecision chooses the finest precision from candidates whose cover of
// the given radius stays within maxCells. Finer cells mean fewer irrelevant
// candidates to filter, so this trades resolution away only when the read
// budget forces it. Candidates need not be sorted. Returns the coarsest
// candidate when none fits.
func PickPrecision(center Point, radiusKm float64, candidates []int, maxCells int) int {
	if len(candidates) == 0 {
		return FinePrecision
	}
	if maxCells <= 0 {
		maxCells = DefaultMaxCoverCells
	}

	best, bestFits := 0, false
	coarsest := clampPrecision(candidates[0])

	for _, candidate := range candidates {
		p := clampPrecision(candidate)
		if p < coarsest {
			coarsest = p
		}
		if CoverCellCount(center, radiusKm, p) <= maxCells && p > best {
			best, bestFits = p, true
		}
	}

	if bestFits {
		return best
	}
	return coarsest
}

// CellDimensionsKm returns the approximate width and height of a cell at the
// given precision, at the equator.
func CellDimensionsKm(precision int) (widthKm, heightKm float64) {
	size, ok := cellSizeKm[clampPrecision(precision)]
	if !ok {
		return 0, 0
	}
	return size[0], size[1]
}

// coverGrid returns how many cell steps in each direction are needed to cover
// the radius, anchored on the centre of the cell containing center.
func coverGrid(center Point, radiusKm float64, precision int) (rows, cols int, anchor Point, dLat, dLng float64) {
	centerCell := Encode(center, precision)
	bounds, err := DecodeBounds(centerCell)
	if err != nil {
		return 0, 0, center, 0, 0
	}

	anchor = bounds.Center()
	dLat = bounds.Max.Lat - bounds.Min.Lat
	dLng = bounds.Max.Lng - bounds.Min.Lng
	if dLat <= 0 || dLng <= 0 {
		return 0, 0, anchor, dLat, dLng
	}

	box := BoundsAround(center, radiusKm)
	// The anchor is the cell centre, which may sit up to half a cell away
	// from `center`, so measure the span from the anchor itself.
	latSpan := math.Max(math.Abs(box.Max.Lat-anchor.Lat), math.Abs(anchor.Lat-box.Min.Lat))
	lngSpan := math.Max(math.Abs(box.Max.Lng-anchor.Lng), math.Abs(anchor.Lng-box.Min.Lng))

	rows = int(math.Ceil(latSpan / dLat))
	cols = int(math.Ceil(lngSpan / dLng))
	return rows, cols, anchor, dLat, dLng
}

func clampPrecision(precision int) int {
	if precision < MinPrecision {
		return MinPrecision
	}
	if precision > MaxPrecision {
		return MaxPrecision
	}
	return precision
}

func lowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
