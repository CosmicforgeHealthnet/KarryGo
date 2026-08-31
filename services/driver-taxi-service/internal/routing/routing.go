// Package routing is taxi's road-routing abstraction (addendum §4). Taxi
// depends on the RoutingClient interface defined here, never directly on
// shared/go/routing, so the provider is swappable and a fake can be injected
// into matching tests without a network call.
//
// shared/go/routing.Client already wraps the Google Routes API and is reused
// unchanged for the Route method. Matrix, SnapToRoad and ReverseGeocode have
// no equivalent there yet -- see google_client.go for how each is handled.
package routing

import (
	"context"
	"time"

	"cosmicforge/logistics/shared/go/geo"
)

// RouteResult is one computed road route. Every field the addendum §4.1
// bullet list asks for is present: road distance, expected duration,
// traffic-adjusted duration when available, encoded polyline geometry, route
// confidence, and provider metadata (Provider/GeneratedAt/RequestID) for
// investigation.
type RouteResult struct {
	DistanceM int64
	DurationS int64
	// TrafficDurationS is 0 when no traffic-adjusted estimate is available --
	// callers must treat zero as "unknown", not as "zero seconds".
	TrafficDurationS int64
	// Polyline is Google's encoded polyline format; decode with
	// geo.DecodePolyline. Empty when no geometry is available (e.g. a
	// haversine-fallback result never carries one).
	Polyline string
	// Confidence is 1.0 for a real routing-provider result, 0 for a
	// haversine-fallback substitute. Matching (phase B) must treat any
	// Confidence == 0 result as candidate pre-filtering only, per addendum
	// §4.1's explicit prohibition on trusting haversine as a final route.
	Confidence float64
	// Provider names which implementation produced this result --
	// "google-routes" or "haversine-fallback" -- so a stored result is
	// self-describing without a separate flag column.
	Provider    string
	GeneratedAt time.Time
	// RequestID is a stable id for investigation. For a real Google call this
	// is a locally generated correlation id (the Routes API response carries
	// no request id of its own); for a fallback result it marks that no real
	// request was made.
	RequestID string
}

// HasGeometry reports whether the result carries a usable polyline.
func (r RouteResult) HasGeometry() bool { return r.Polyline != "" }

// Points decodes the route geometry.
func (r RouteResult) Points() ([]geo.Point, error) { return geo.DecodePolyline(r.Polyline) }

// MatrixResult is a grid of routes, one per (origin, destination) pair.
// Rows[i][j] is the route from origins[i] to destinations[j].
type MatrixResult struct {
	Rows [][]RouteResult
}

// SnappedRoute is a sequence of points corrected onto the actual road
// network. No P0 caller exists yet (see google_client.go).
type SnappedRoute struct {
	Points []geo.Point
}

// AddressResult is a human-readable address for a coordinate. No P0 caller
// exists yet (see google_client.go).
type AddressResult struct {
	Address string
}

// RoutingClient is the addendum §4.1 interface. Matching (phase B) depends on
// Route and Matrix; SnapToRoad and ReverseGeocode are defined for interface
// completeness and future callers, not because anything in this build phase
// invokes them.
type RoutingClient interface {
	Route(ctx context.Context, origin, destination geo.Point) (RouteResult, error)
	Matrix(ctx context.Context, origins, destinations []geo.Point) (MatrixResult, error)
	SnapToRoad(ctx context.Context, points []geo.Point) (SnappedRoute, error)
	ReverseGeocode(ctx context.Context, lat, lng float64) (AddressResult, error)
}

// ErrNotImplemented is returned by SnapToRoad and ReverseGeocode. Addendum
// architecture principle "advanced features must degrade safely" is read here
// as: an explicit error is a safe degrade, a silent zero-value is not. A
// caller that starts needing either method will find out immediately, not
// discover a quietly-empty result in production.
var ErrNotImplemented = errNotImplemented{}

type errNotImplemented struct{}

func (errNotImplemented) Error() string {
	return "routing: not implemented for this release"
}
