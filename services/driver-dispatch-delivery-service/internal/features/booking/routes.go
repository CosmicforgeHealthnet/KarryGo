package booking

import (
	"context"
	"log"
	"time"

	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/routing"
)

// routeFetchTimeout bounds a background route lookup. Route geometry is a
// display nicety -- the booking exists and the rider is assigned by the time
// these run -- so a slow routing API must never hold either up.
const routeFetchTimeout = 10 * time.Second

// fetchDropoffRoute stores the road route from pickup to drop-off.
//
// Runs in the background at booking creation, while the customer is watching a
// "finding you a rider" screen. A missing route only costs them a straight
// line on the map, so it must never be on the create path.
//
// Bike deliveries route as TWO_WHEELER: a rider can legally use roads a car
// route would avoid, so a car polyline would misplace both the drawn route and
// the progress measured along it.
func (s *Service) fetchDropoffRoute(bookingID string, pickup, dropoff geo.Point) {
	if s.routes == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), routeFetchTimeout)
		defer cancel()

		route, err := s.routes.Route(ctx, pickup, dropoff, routing.TravelModeTwoWheeler)
		if err != nil {
			log.Printf("booking %s: drop-off route unavailable, falling back to straight line: %v", bookingID, err)
			return
		}
		if !route.HasGeometry() {
			return
		}
		if err := s.repository.SaveDropoffRoute(ctx, bookingID, route.Polyline, route.DistanceM, route.DurationS); err != nil {
			log.Printf("booking %s: could not store drop-off route: %v", bookingID, err)
		}
	}()
}

// fetchPickupRoute stores the road route from the rider's current position to
// the pickup. Acceptance is the first moment both ends are known.
func (s *Service) fetchPickupRoute(bookingID, providerID string, pickup geo.Point) {
	if s.routes == nil || s.live == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), routeFetchTimeout)
		defer cancel()

		location, ok, err := s.live.GetLocation(ctx, providerID)
		if err != nil || !ok {
			return
		}
		origin := geo.Point{Lat: location.Lat, Lng: location.Lng}
		if !origin.HasFix() {
			return
		}

		route, err := s.routes.Route(ctx, origin, pickup, routing.TravelModeTwoWheeler)
		if err != nil {
			log.Printf("booking %s: pickup route unavailable, falling back to straight line: %v", bookingID, err)
			return
		}
		if !route.HasGeometry() {
			return
		}
		if err := s.repository.SavePickupRoute(ctx, bookingID, route.Polyline, route.DistanceM, route.DurationS); err != nil {
			log.Printf("booking %s: could not store pickup route: %v", bookingID, err)
		}
	}()
}
