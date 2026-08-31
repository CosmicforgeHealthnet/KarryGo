// Package geo provides pure geographic primitives shared by every Cosmicforge
// Logistics service: great-circle distance, bearing, encoded polylines,
// projection of a point onto a route, and a geohash cell index used to
// spatially partition live vehicle positions.
//
// Deterministic and pure: no external maps API, no Redis, no config, no I/O,
// and no dependency outside the standard library. Persistence, fan-out and
// TTL semantics live in shared/go/livetrack; each service owns its own wiring.
package geo

import "math"

const (
	// EarthRadiusKm is the mean Earth radius used by every distance helper
	// here. It matches the value the services previously hardcoded, so
	// delegating to this package does not change any existing fare or
	// matching result.
	EarthRadiusKm = 6371.0
	// EarthRadiusMeters is EarthRadiusKm expressed in metres.
	EarthRadiusMeters = EarthRadiusKm * 1000

	// KmPerDegreeLat is the length of one degree of latitude. Longitude
	// degrees shrink by cos(latitude); see kmPerDegreeLng.
	KmPerDegreeLat = math.Pi * EarthRadiusKm / 180
)

// Point is a WGS84 coordinate. The JSON names match the wire format the
// platform already uses for provider locations (`lat`/`lng`).
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Valid reports whether the coordinate is inside the WGS84 range.
func (p Point) Valid() bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lng >= -180 && p.Lng <= 180
}

// IsZero reports whether the point is null island. Callers treat this as
// "no fix" because a genuine (0,0) reading is not serviceable territory.
func (p Point) IsZero() bool { return p.Lat == 0 && p.Lng == 0 }

// HasFix reports whether the point is usable as a real position.
func (p Point) HasFix() bool { return p.Valid() && !p.IsZero() }

// Bounds is an axis-aligned latitude/longitude rectangle.
type Bounds struct {
	Min Point `json:"min"`
	Max Point `json:"max"`
}

// Center returns the midpoint of the rectangle.
func (b Bounds) Center() Point {
	return Point{Lat: (b.Min.Lat + b.Max.Lat) / 2, Lng: (b.Min.Lng + b.Max.Lng) / 2}
}

// Contains reports whether p lies inside the rectangle (edges included).
func (b Bounds) Contains(p Point) bool {
	return p.Lat >= b.Min.Lat && p.Lat <= b.Max.Lat &&
		p.Lng >= b.Min.Lng && p.Lng <= b.Max.Lng
}

// HaversineKm returns the great-circle distance in kilometres between two
// points.
func HaversineKm(a, b Point) float64 {
	dLat := radians(b.Lat - a.Lat)
	dLng := radians(b.Lng - a.Lng)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(radians(a.Lat))*math.Cos(radians(b.Lat))*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return EarthRadiusKm * 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
}

// HaversineMeters returns the great-circle distance in metres.
func HaversineMeters(a, b Point) float64 { return HaversineKm(a, b) * 1000 }

// BearingDegrees returns the initial compass bearing from `from` to `to`,
// normalised to [0, 360). It is what a map marker is rotated by so the
// vehicle icon points the way it is travelling.
func BearingDegrees(from, to Point) float64 {
	lat1 := radians(from.Lat)
	lat2 := radians(to.Lat)
	dLng := radians(to.Lng - from.Lng)

	y := math.Sin(dLng) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLng)
	return NormalizeBearing(degrees(math.Atan2(y, x)))
}

// NormalizeBearing folds any angle into [0, 360).
func NormalizeBearing(deg float64) float64 {
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	return d
}

// Destination returns the point reached by travelling distanceKm from p along
// the given compass bearing. Used for dead-reckoning between GPS fixes and for
// building bounding boxes.
func Destination(p Point, bearingDeg, distanceKm float64) Point {
	angular := distanceKm / EarthRadiusKm
	brg := radians(bearingDeg)
	lat1 := radians(p.Lat)
	lng1 := radians(p.Lng)

	lat2 := math.Asin(math.Sin(lat1)*math.Cos(angular) +
		math.Cos(lat1)*math.Sin(angular)*math.Cos(brg))
	lng2 := lng1 + math.Atan2(
		math.Sin(brg)*math.Sin(angular)*math.Cos(lat1),
		math.Cos(angular)-math.Sin(lat1)*math.Sin(lat2),
	)
	return Normalize(Point{Lat: degrees(lat2), Lng: degrees(lng2)})
}

// boundsEpsilonDeg pads BoundsAround by about a centimetre. The flat
// degrees-per-km conversion here and the spherical one in Destination agree
// analytically but not bit-for-bit, and a cover that misses a point by
// nanodegrees would silently drop providers sitting on the radius edge.
const boundsEpsilonDeg = 1e-7

// BoundsAround returns the smallest lat/lng rectangle enclosing the circle of
// radiusKm around center. Longitude degrees are widened by 1/cos(lat) so the
// box stays a true superset of the circle.
func BoundsAround(center Point, radiusKm float64) Bounds {
	if radiusKm < 0 {
		radiusKm = 0
	}
	dLat := radiusKm/KmPerDegreeLat + boundsEpsilonDeg
	dLng := radiusKm/kmPerDegreeLng(center.Lat) + boundsEpsilonDeg

	return Bounds{
		Min: Point{Lat: clampLat(center.Lat - dLat), Lng: clampLng(center.Lng - dLng)},
		Max: Point{Lat: clampLat(center.Lat + dLat), Lng: clampLng(center.Lng + dLng)},
	}
}

// Normalize clamps latitude into [-90, 90] and wraps longitude into
// [-180, 180).
func Normalize(p Point) Point {
	return Point{Lat: clampLat(p.Lat), Lng: wrapLng(p.Lng)}
}

// Lerp linearly interpolates between a and b. t is clamped to [0, 1]. Over the
// segment lengths involved in city routing the error against a great-circle
// interpolation is far below GPS accuracy.
func Lerp(a, b Point, t float64) Point {
	switch {
	case t <= 0:
		return a
	case t >= 1:
		return b
	}
	return Point{Lat: a.Lat + (b.Lat-a.Lat)*t, Lng: a.Lng + (b.Lng-a.Lng)*t}
}

// PointInPolygon reports whether p lies inside the polygon using even-odd ray
// casting. The polygon may be open or explicitly closed.
func PointInPolygon(p Point, polygon []Point) bool {
	if len(polygon) < 3 {
		return false
	}
	inside := false
	for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
		pi, pj := polygon[i], polygon[j]
		if (pi.Lat > p.Lat) != (pj.Lat > p.Lat) &&
			p.Lng < (pj.Lng-pi.Lng)*(p.Lat-pi.Lat)/(pj.Lat-pi.Lat)+pi.Lng {
			inside = !inside
		}
	}
	return inside
}

// planar projects p into a local flat frame centred on origin, in kilometres.
// Accurate to well under a metre over city-scale distances, and far cheaper
// than repeated haversine calls inside the per-segment projection loop.
func planar(origin, p Point) (x, y float64) {
	x = (p.Lng - origin.Lng) * kmPerDegreeLng(origin.Lat)
	y = (p.Lat - origin.Lat) * KmPerDegreeLat
	return x, y
}

func kmPerDegreeLng(lat float64) float64 {
	c := math.Cos(radians(lat))
	// Guard the poles so a division never explodes.
	if c < 1e-6 {
		c = 1e-6
	}
	return KmPerDegreeLat * c
}

func clampLat(lat float64) float64 {
	if lat > 90 {
		return 90
	}
	if lat < -90 {
		return -90
	}
	return lat
}

func clampLng(lng float64) float64 {
	if lng > 180 {
		return 180
	}
	if lng < -180 {
		return -180
	}
	return lng
}

func wrapLng(lng float64) float64 {
	if lng >= -180 && lng < 180 {
		return lng
	}
	w := math.Mod(lng+180, 360)
	if w < 0 {
		w += 360
	}
	return w - 180
}

func radians(deg float64) float64 { return deg * math.Pi / 180 }
func degrees(rad float64) float64 { return rad * 180 / math.Pi }
