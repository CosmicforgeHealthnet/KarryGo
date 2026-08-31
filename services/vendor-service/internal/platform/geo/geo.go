// Package geo is the service-local great-circle distance helper.
//
// It lives here rather than being taken from shared/go/dispatchpricing on
// purpose: that package is the dispatch FARE formula, and a vendor-owned
// delivery charge or a service-area reach test has nothing to do with dispatch
// pricing. Importing it for its haversine would quietly couple vendor-owned
// delivery to a platform fare model it must never use.
//
// Nothing here prices anything. It answers distance and reach only.
package geo

import "math"

// EarthRadiusKm is the mean earth radius used by every distance calculation in
// this platform (dispatch, hauling and here all use 6371).
const EarthRadiusKm = 6371.0

// HaversineKm is the great-circle distance between two WGS84 points.
func HaversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }

	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLng/2)*math.Sin(dLng/2)

	return EarthRadiusKm * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// ValidCoordinate rejects out-of-range coordinates and null island.
//
// Null island is rejected because it is what an unset/failed geocode looks
// like, and quoting a delivery to (0,0) is worse than refusing to quote one.
func ValidCoordinate(lat, lng float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lng) ||
		math.IsInf(lat, 0) || math.IsInf(lng, 0) {
		return false
	}

	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return false
	}

	return lat != 0 || lng != 0
}

// WithinRadiusMeters reports whether a point falls inside a circular area.
func WithinRadiusMeters(
	centerLat, centerLng, lat, lng float64,
	radiusMeters int,
) bool {
	if radiusMeters <= 0 {
		return false
	}

	return HaversineKm(centerLat, centerLng, lat, lng)*1000 <=
		float64(radiusMeters)
}
