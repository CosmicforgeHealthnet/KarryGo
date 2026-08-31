package geo

import "math"

// Projection is the result of snapping a point onto a path. It is the
// primitive behind the trip progress bar: AlongKm/PathLengthKm is how far the
// vehicle has travelled along its actual route rather than as the crow flies.
type Projection struct {
	// SegmentIndex is the index of the path vertex that starts the closest
	// segment.
	SegmentIndex int `json:"segment_index"`
	// T is the position along that segment, 0 at its start and 1 at its end.
	T float64 `json:"t"`
	// Point is the snapped position on the path.
	Point Point `json:"point"`
	// AlongKm is the distance from the start of the path to Point.
	AlongKm float64 `json:"along_km"`
	// OffPathKm is how far the input point sits from the path. A large value
	// means the vehicle has deviated and the projection should be distrusted.
	OffPathKm float64 `json:"off_path_km"`
}

// PathLengthKm returns the summed great-circle length of a path.
func PathLengthKm(path []Point) float64 {
	if len(path) < 2 {
		return 0
	}
	total := 0.0
	for i := 1; i < len(path); i++ {
		total += HaversineKm(path[i-1], path[i])
	}
	return total
}

// SnapToPath projects p onto the closest segment of path. It reports false
// when the path has fewer than two points, in which case callers fall back to
// straight-line measurement.
func SnapToPath(p Point, path []Point) (Projection, bool) {
	if len(path) < 2 {
		return Projection{}, false
	}

	best := Projection{OffPathKm: math.Inf(1)}
	cumulative := 0.0

	for i := 0; i < len(path)-1; i++ {
		a, b := path[i], path[i+1]
		segmentKm := HaversineKm(a, b)

		t, offKm := projectOntoSegment(p, a, b)
		if offKm < best.OffPathKm {
			best = Projection{
				SegmentIndex: i,
				T:            t,
				Point:        Lerp(a, b, t),
				AlongKm:      cumulative + segmentKm*t,
				OffPathKm:    offKm,
			}
		}
		cumulative += segmentKm
	}

	if math.IsInf(best.OffPathKm, 1) {
		return Projection{}, false
	}
	return best, true
}

// ProgressAlongPath returns how far along the path p sits, as a fraction in
// [0, 1]. A zero-length path yields 0.
func ProgressAlongPath(p Point, path []Point) (float64, bool) {
	proj, ok := SnapToPath(p, path)
	if !ok {
		return 0, false
	}
	total := PathLengthKm(path)
	if total <= 0 {
		return 0, true
	}
	return clamp01(proj.AlongKm / total), true
}

// RemainingKm returns the distance left along the path from p to its end.
func RemainingKm(p Point, path []Point) (float64, bool) {
	proj, ok := SnapToPath(p, path)
	if !ok {
		return 0, false
	}
	remaining := PathLengthKm(path) - proj.AlongKm
	if remaining < 0 {
		return 0, true
	}
	return remaining, true
}

// projectOntoSegment returns the clamped position of p along segment a→b and
// the distance from p to that position, both computed in a local flat frame
// anchored at a.
func projectOntoSegment(p, a, b Point) (t, offKm float64) {
	bx, by := planar(a, b)
	px, py := planar(a, p)

	lengthSq := bx*bx + by*by
	if lengthSq == 0 {
		return 0, math.Hypot(px, py)
	}

	t = clamp01((px*bx + py*by) / lengthSq)
	sx, sy := bx*t, by*t
	return t, math.Hypot(px-sx, py-sy)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
