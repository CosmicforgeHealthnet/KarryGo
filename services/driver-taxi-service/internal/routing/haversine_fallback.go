package routing

import (
	"context"
	"time"

	"cosmicforge/logistics/shared/go/geo"
)

// averageUrbanSpeedKmh backs the fallback's duration estimate: a haversine
// distance divided by a plausible average city driving speed, not a real
// route time. It exists only so a fallback result has SOME duration value for
// candidate pre-filtering; it must never be mistaken for a real ETA.
const averageUrbanSpeedKmh = 25.0

// HaversineFallbackClient wraps a RoutingClient with a circuit breaker. While
// the breaker is closed or half-open, calls pass straight through to the
// underlying client. Once it opens, Route and Matrix are answered with a
// straight-line distance and a rough speed-based duration instead of
// attempting the real network call -- addendum §4.1 permits haversine as a
// degraded fallback for candidate pre-filtering, but is explicit that it must
// never be treated as a trusted final route. Every fallback result carries
// Confidence 0 specifically so a caller cannot use it as one by accident.
//
// SnapToRoad and ReverseGeocode are never substituted -- there is no
// reasonable straight-line stand-in for either -- so they always call
// through to the underlying client, whatever the breaker's state.
type HaversineFallbackClient struct {
	underlying RoutingClient
	breaker    *Breaker
}

// NewHaversineFallbackClient wraps underlying with a breaker using the given
// failure threshold and cooldown (0 for either uses the package defaults).
func NewHaversineFallbackClient(underlying RoutingClient, failureThreshold int, cooldown time.Duration) *HaversineFallbackClient {
	return &HaversineFallbackClient{
		underlying: underlying,
		breaker:    NewBreaker(failureThreshold, cooldown),
	}
}

// Breaker exposes the underlying breaker for observability (e.g. an ops
// endpoint reporting whether routing is currently degraded).
func (c *HaversineFallbackClient) Breaker() *Breaker { return c.breaker }

func (c *HaversineFallbackClient) Route(ctx context.Context, origin, destination geo.Point) (RouteResult, error) {
	if !c.breaker.Allow() {
		return fallbackRoute(origin, destination), nil
	}
	result, err := c.underlying.Route(ctx, origin, destination)
	if err != nil {
		c.breaker.Failure()
		return fallbackRoute(origin, destination), nil
	}
	c.breaker.Success()
	return result, nil
}

func (c *HaversineFallbackClient) Matrix(ctx context.Context, origins, destinations []geo.Point) (MatrixResult, error) {
	if !c.breaker.Allow() {
		return fallbackMatrix(origins, destinations), nil
	}
	result, err := c.underlying.Matrix(ctx, origins, destinations)
	if err != nil {
		c.breaker.Failure()
		return fallbackMatrix(origins, destinations), nil
	}
	c.breaker.Success()
	return result, nil
}

// SnapToRoad always calls through: no straight-line stand-in exists for it.
func (c *HaversineFallbackClient) SnapToRoad(ctx context.Context, points []geo.Point) (SnappedRoute, error) {
	return c.underlying.SnapToRoad(ctx, points)
}

// ReverseGeocode always calls through: no straight-line stand-in exists for it.
func (c *HaversineFallbackClient) ReverseGeocode(ctx context.Context, lat, lng float64) (AddressResult, error) {
	return c.underlying.ReverseGeocode(ctx, lat, lng)
}

func fallbackRoute(origin, destination geo.Point) RouteResult {
	distanceKm := geo.HaversineKm(origin, destination)
	durationS := int64((distanceKm / averageUrbanSpeedKmh) * 3600)
	return RouteResult{
		DistanceM:   int64(distanceKm * 1000),
		DurationS:   durationS,
		Polyline:    "",
		Confidence:  0,
		Provider:    "haversine-fallback",
		GeneratedAt: time.Now(),
		RequestID:   "",
	}
}

func fallbackMatrix(origins, destinations []geo.Point) MatrixResult {
	rows := make([][]RouteResult, len(origins))
	for i, origin := range origins {
		rows[i] = make([]RouteResult, len(destinations))
		for j, dest := range destinations {
			rows[i][j] = fallbackRoute(origin, dest)
		}
	}
	return MatrixResult{Rows: rows}
}
