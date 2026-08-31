package routeintegrity

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

type Section6RouteAuthority struct{ planner commercequote.RoutePlanner }

func NewSection6RouteAuthority(planner commercequote.RoutePlanner) *Section6RouteAuthority {
	return &Section6RouteAuthority{planner: planner}
}
func (a *Section6RouteAuthority) Plan(ctx context.Context, points []Point) (RoutePlan, error) {
	if a == nil || a.planner == nil {
		return RoutePlan{}, errors.New("route authority unavailable")
	}
	input := make([]commercequote.RoutePoint, 0, len(points))
	for _, p := range points {
		input = append(input, commercequote.RoutePoint{Ref: p.Ref, Latitude: p.Latitude, Longitude: p.Longitude})
	}
	plan, err := a.planner.Plan(ctx, input, dispatchcommerce.ModeFastest)
	if err != nil {
		return RoutePlan{}, err
	}
	return RoutePlan{Reference: "route:" + plan.Provider + ":" + plan.EvidenceVersion, Provider: plan.Provider, EvidenceVersion: plan.EvidenceVersion, DistanceMeters: plan.DistanceMeters, DurationSeconds: plan.DurationSeconds, OrderedPoints: points}, nil
}
