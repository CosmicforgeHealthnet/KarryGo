// Package geo provides geographic helpers for the Partner API pricing engine.
package geo

import "math"

// HaversineKm returns the great-circle distance in kilometres between two
// latitude/longitude points (WGS84), using the haversine formula.
func HaversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371.0

	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)

	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
