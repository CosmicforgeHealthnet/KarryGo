package missionplanning

import (
	"context"
	"fmt"

	dispatchquote "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/serviceability"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

// RouteAuthority is the narrow Section 6 authority consumed by mission
// planning. Planning stores only the route evidence reference and bounded
// decision inputs; it does not introduce another routing engine.
type RouteAuthority interface {
	Evaluate(context.Context, PlanRequest) (RouteEvidence, error)
}

type Section6RouteAuthority struct {
	planner  dispatchquote.RoutePlanner
	zones    serviceability.ZoneEvaluator
	degraded bool
}

func NewSection6RouteAuthority(planner dispatchquote.RoutePlanner, zones serviceability.ZoneEvaluator, degraded bool) *Section6RouteAuthority {
	return &Section6RouteAuthority{planner: planner, zones: zones, degraded: degraded}
}

func (a *Section6RouteAuthority) Evaluate(ctx context.Context, in PlanRequest) (RouteEvidence, error) {
	if a == nil || a.planner == nil || a.zones == nil {
		return RouteEvidence{}, fmt.Errorf("route authority unavailable")
	}
	points := make([]dispatchquote.RoutePoint, 0, len(in.Units)+1)
	for _, unit := range in.Units {
		pickup := serviceability.Point{Latitude: unit.Pickup.Latitude, Longitude: unit.Pickup.Longitude}
		dropoff := serviceability.Point{Latitude: in.Receiver.Latitude, Longitude: in.Receiver.Longitude}
		pickupInside, dropoffInside, restricted, available := a.zones.Evaluate(pickup, dropoff)
		if !available || !pickupInside || !dropoffInside || restricted {
			return RouteEvidence{Serviceable: false, ReasonCode: "OPERATING_ZONE_INCOMPATIBLE"}, nil
		}
		points = append(points, dispatchquote.RoutePoint{Ref: unit.UnitRef, Latitude: unit.Pickup.Latitude, Longitude: unit.Pickup.Longitude})
	}
	points = append(points, dispatchquote.RoutePoint{Ref: "receiver", Latitude: in.Receiver.Latitude, Longitude: in.Receiver.Longitude})
	plan, err := a.planner.Plan(ctx, points, dispatchcommerce.ModeFastest)
	if err != nil {
		if !a.degraded {
			return RouteEvidence{}, err
		}
		return RouteEvidence{Reference: "degraded:unavailable", Serviceable: true, Degraded: true, ReasonCode: "ROUTE_DEGRADED"}, nil
	}
	return RouteEvidence{Reference: plan.Provider + ":" + plan.EvidenceVersion, Serviceable: true, DistanceKM: float64(plan.DistanceMeters) / 1000, DurationSeconds: plan.DurationSeconds}, nil
}
