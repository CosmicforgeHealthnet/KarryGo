// Package geo provides geographic helpers for the Partner API pricing engine.
//
// The distance maths itself lives in shared/go/geo so the partner quote, the
// dispatch customer quote and the matching engines can never drift apart; this
// package is the gateway-local scalar-argument shim over it.
package geo

import sharedgeo "cosmicforge/logistics/shared/go/geo"

// HaversineKm returns the great-circle distance in kilometres between two
// latitude/longitude points (WGS84), using the haversine formula.
func HaversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	return sharedgeo.HaversineKm(
		sharedgeo.Point{Lat: lat1, Lng: lng1},
		sharedgeo.Point{Lat: lat2, Lng: lng2},
	)
}
