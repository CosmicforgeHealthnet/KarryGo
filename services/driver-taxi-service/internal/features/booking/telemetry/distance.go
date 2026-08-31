package bookingtelemetry

import "cosmicforge/logistics/shared/go/geo"

// ActualDistanceKm sums consecutive haversine distances across an
// already-ordered, already-filtered trail -- the server-computed distance
// PricingClient.Reconcile is meant to use (addendum §8), in place of a
// client-submitted total. Callers pass a trail pre-filtered to clean points
// only (see Repository.ValidatedTrail): this function trusts its input's
// ordering and cleanliness, it does not re-check either.
//
// Returns 0, false when fewer than two points are given -- one point (or
// none) cannot describe a path, and returning a confident 0km would be
// worse than telling the caller there is nothing to derive a distance from
// yet, so they can fall back to whatever estimate they already have.
func ActualDistanceKm(trail []Point) (float64, bool) {
	if len(trail) < 2 {
		return 0, false
	}
	points := make([]geo.Point, len(trail))
	for i, p := range trail {
		points[i] = geo.Point{Lat: p.Lat, Lng: p.Lng}
	}
	return geo.PathLengthKm(points), true
}
