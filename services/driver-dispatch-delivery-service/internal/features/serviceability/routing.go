package serviceability

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	dispatchquote "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/dispatchpricing"
)

type RoutingAdapter interface {
	Route(context.Context, Point, Point, string) (RouteResult, error)
}

type RoutePlannerAdapter struct{ planner dispatchquote.RoutePlanner }

func NewRoutePlannerAdapter(planner dispatchquote.RoutePlanner) *RoutePlannerAdapter {
	return &RoutePlannerAdapter{planner: planner}
}

func (a *RoutePlannerAdapter) Route(ctx context.Context, pickup, dropoff Point, _ string) (RouteResult, error) {
	if a == nil || a.planner == nil {
		return RouteResult{}, fmt.Errorf("route authority unavailable")
	}
	plan, err := a.planner.Plan(ctx, []dispatchquote.RoutePoint{
		{Ref: "pickup", Latitude: pickup.Latitude, Longitude: pickup.Longitude},
		{Ref: "dropoff", Latitude: dropoff.Latitude, Longitude: dropoff.Longitude},
	}, dispatchcommerce.ModeFastest)
	if err != nil {
		return RouteResult{}, err
	}
	return RouteResult{DistanceMeters: plan.DistanceMeters, DurationSeconds: plan.DurationSeconds, Source: plan.Provider, EvidenceVersion: plan.EvidenceVersion, Confidence: "AUTHORITATIVE"}, nil
}

func degradedRoute(pickup, dropoff Point) RouteResult {
	// Explicit degraded mode uses a conservative road-distance multiplier and
	// labels the evidence. It is never presented as authoritative routing.
	distance := dispatchpricing.HaversineKm(pickup.Latitude, pickup.Longitude, dropoff.Latitude, dropoff.Longitude)
	meters := int64(math.Ceil(distance * 1.35 * 1000))
	return RouteResult{DistanceMeters: meters, DurationSeconds: int64(math.Ceil(float64(meters) / 5.5)), Source: "CONSERVATIVE_GEO_FALLBACK", EvidenceVersion: "degraded-v1", Confidence: "LOW", Degraded: true}
}

type ZoneEvaluator interface {
	Evaluate(Point, Point) (pickupInside, dropoffInside, restricted bool, policyAvailable bool)
}

type circularArea struct {
	center   Point
	radiusKM float64
}

type CircularZone struct {
	center     Point
	radiusKM   float64
	restricted []circularArea
	valid      bool
}

// NewCircularZone creates the server-owned operating policy. Restricted zones
// use a compact deployment value: "lat,lng,radius_km;lat,lng,radius_km".
// Malformed policy is retained as unavailable so evaluation fails closed.
func NewCircularZone(center Point, radiusKM float64, restrictedSpec string) CircularZone {
	z := CircularZone{center: center, radiusKM: radiusKM, valid: radiusKM > 0 && center.Validate() == nil}
	if !z.valid {
		return z
	}
	for _, raw := range strings.Split(strings.TrimSpace(restrictedSpec), ";") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.Split(raw, ",")
		if len(parts) != 3 {
			z.valid = false
			return z
		}
		lat, latErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lng, lngErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		radius, radiusErr := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		area := circularArea{center: Point{Latitude: lat, Longitude: lng}, radiusKM: radius}
		if latErr != nil || lngErr != nil || radiusErr != nil || radius <= 0 || area.center.Validate() != nil {
			z.valid = false
			return z
		}
		z.restricted = append(z.restricted, area)
	}
	return z
}

func (z CircularZone) Evaluate(pickup, dropoff Point) (bool, bool, bool, bool) {
	if !z.valid {
		return false, false, false, false
	}
	restricted := false
	for _, area := range z.restricted {
		if dispatchpricing.HaversineKm(area.center.Latitude, area.center.Longitude, pickup.Latitude, pickup.Longitude) <= area.radiusKM ||
			dispatchpricing.HaversineKm(area.center.Latitude, area.center.Longitude, dropoff.Latitude, dropoff.Longitude) <= area.radiusKM {
			restricted = true
			break
		}
	}
	return dispatchpricing.HaversineKm(z.center.Latitude, z.center.Longitude, pickup.Latitude, pickup.Longitude) <= z.radiusKM,
		dispatchpricing.HaversineKm(z.center.Latitude, z.center.Longitude, dropoff.Latitude, dropoff.Longitude) <= z.radiusKM,
		restricted, true
}
