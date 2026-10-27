package journeyshare

import (
	"context"
	"math"
	"sync"
	"time"

	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/routing"
)

// ETAs is when the driver should reach the pickup and the drop-off. A time is
// nil when it cannot be estimated honestly (no fresh driver position, or the
// stop is already behind them).
type ETAs struct {
	PickupAt       *time.Time `json:"pickup_eta,omitempty"`
	DropoffAt      *time.Time `json:"dropoff_eta,omitempty"`
	PickupSeconds  *int64     `json:"pickup_eta_seconds,omitempty"`
	DropoffSeconds *int64     `json:"dropoff_eta_seconds,omitempty"`
}

// Estimator returns the driving time between two points.
type Estimator interface {
	Duration(ctx context.Context, from, to geo.Point) time.Duration
}

// handoverTime is the usual time spent at pickup before leaving: loading a
// truck takes far longer than a passenger getting in.
func handoverTime(product string) time.Duration {
	switch product {
	case "TRUCK":
		return 15 * time.Minute
	case "SEND", "COMMERCE_MISSION", "FOOD", "SHOP":
		return 3 * time.Minute
	}
	return time.Minute
}

// EstimateETAs works out the pickup and drop-off times for a projection from
// the driver's live position. It needs a fresh position: a stale one would
// promise a time the driver cannot keep.
func EstimateETAs(ctx context.Context, p Projection, est Estimator, now time.Time) ETAs {
	var out ETAs
	if est == nil || p.Terminal || p.Pickup == nil || p.Dropoff == nil {
		return out
	}
	pickup := geo.Point{Lat: p.Pickup.Lat, Lng: p.Pickup.Lng}
	dropoff := geo.Point{Lat: p.Dropoff.Lat, Lng: p.Dropoff.Lng}
	fresh := p.Location.Available && p.Location.State == "FRESH" && p.Location.Lat != nil && p.Location.Lng != nil
	var here geo.Point
	if fresh {
		here = geo.Point{Lat: *p.Location.Lat, Lng: *p.Location.Lng}
	}

	set := func(at time.Time) (*time.Time, *int64) {
		at = at.UTC().Truncate(time.Second)
		secs := int64(math.Max(0, at.Sub(now).Seconds()))
		return &at, &secs
	}
	switch p.PublicStatus {
	case "ASSIGNED", "EN_ROUTE_PICKUP":
		if !fresh {
			return out
		}
		pickupAt := now.Add(est.Duration(ctx, here, pickup))
		out.PickupAt, out.PickupSeconds = set(pickupAt)
		out.DropoffAt, out.DropoffSeconds = set(pickupAt.Add(handoverTime(p.ProductCode)).Add(est.Duration(ctx, pickup, dropoff)))
	case "ARRIVED":
		out.PickupAt, out.PickupSeconds = set(now)
		out.DropoffAt, out.DropoffSeconds = set(now.Add(handoverTime(p.ProductCode)).Add(est.Duration(ctx, pickup, dropoff)))
	case "IN_PROGRESS":
		if !fresh {
			return out
		}
		out.DropoffAt, out.DropoffSeconds = set(now.Add(est.Duration(ctx, here, dropoff)))
	}
	return out
}

// WithETAs fills the projection's ETA fields; ETA is the next stop's time.
func WithETAs(ctx context.Context, p Projection, est Estimator, now time.Time) Projection {
	etas := EstimateETAs(ctx, p, est, now)
	p.PickupETA, p.DropoffETA = etas.PickupAt, etas.DropoffAt
	p.ETA = etas.PickupAt
	if p.ETA == nil {
		p.ETA = etas.DropoffAt
	}
	return p
}

// StraightLineEstimator estimates from straight-line distance: roads are
// longer than the crow flies (RoadFactor) and city traffic is slow (SpeedKmh).
type StraightLineEstimator struct {
	SpeedKmh   float64
	RoadFactor float64
}

// Duration implements Estimator.
func (s StraightLineEstimator) Duration(_ context.Context, from, to geo.Point) time.Duration {
	speed, factor := s.SpeedKmh, s.RoadFactor
	if speed <= 0 {
		speed = 22
	}
	if factor < 1 {
		factor = 1.35
	}
	hours := geo.HaversineKm(from, to) * factor / speed
	return time.Duration(hours * float64(time.Hour)).Round(time.Second)
}

// RouteEstimator uses road routes when a routing client is configured and
// falls back to Fallback otherwise or on any failure. Routes are cached per
// pair of ~100 m cells for TTL, because apps ask every few seconds and the
// answer barely changes in that time.
type RouteEstimator struct {
	Client   *routing.Client
	Mode     routing.TravelMode
	Fallback Estimator
	TTL      time.Duration

	mu    sync.Mutex
	cache map[[4]int64]cachedDuration
}

type cachedDuration struct {
	d       time.Duration
	expires time.Time
}

const maxCachedRoutes = 5000

// Duration implements Estimator.
func (r *RouteEstimator) Duration(ctx context.Context, from, to geo.Point) time.Duration {
	fallback := r.Fallback
	if fallback == nil {
		fallback = StraightLineEstimator{}
	}
	if r.Client == nil || !r.Client.Enabled() {
		return fallback.Duration(ctx, from, to)
	}
	key := [4]int64{cell(from.Lat), cell(from.Lng), cell(to.Lat), cell(to.Lng)}
	now := time.Now()
	r.mu.Lock()
	if hit, ok := r.cache[key]; ok && now.Before(hit.expires) {
		r.mu.Unlock()
		return hit.d
	}
	r.mu.Unlock()

	routeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	route, err := r.Client.Route(routeCtx, from, to, r.Mode)
	if err != nil || route.DurationS <= 0 {
		return fallback.Duration(ctx, from, to)
	}
	d := time.Duration(route.DurationS) * time.Second
	ttl := r.TTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	r.mu.Lock()
	if r.cache == nil || len(r.cache) >= maxCachedRoutes {
		r.cache = make(map[[4]int64]cachedDuration)
	}
	r.cache[key] = cachedDuration{d: d, expires: now.Add(ttl)}
	r.mu.Unlock()
	return d
}

// cell rounds a coordinate to about 100 m.
func cell(v float64) int64 { return int64(math.Round(v * 1000)) }
