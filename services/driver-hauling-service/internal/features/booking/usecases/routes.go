package bookingusecases

import (
	"context"
	"log"
	"time"

	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/routing"
)

// routeFetchTimeout bounds a background route lookup. Route geometry is a
// display nicety: the booking is already created and the driver already
// assigned by the time these run, so a slow routing API must never hold either
// up.
const routeFetchTimeout = 10 * time.Second

// fetchDropoffRoute stores the road route from pickup to drop-off.
//
// Runs in the background at booking creation: the customer is watching a
// "finding you a truck" screen at that point, and a missing route only costs
// them a straight line on the map, so this must never be on the create path.
func (s *BookingService) fetchDropoffRoute(bookingID string, pickup, dropoff geo.Point) {
	if s.routes == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), routeFetchTimeout)
		defer cancel()

		route, err := s.routes.Route(ctx, pickup, dropoff, routing.TravelModeDrive)
		if err != nil {
			log.Printf("booking %s: drop-off route unavailable, falling back to straight line: %v", bookingID, err)
			return
		}
		if !route.HasGeometry() {
			return
		}
		if err := s.bookings.SaveDropoffRoute(ctx, bookingID, route.Polyline, route.DistanceM, route.DurationS); err != nil {
			log.Printf("booking %s: could not store drop-off route: %v", bookingID, err)
		}
	}()
}

// fetchPickupRoute stores the road route from the driver's current position to
// the pickup.
//
// Runs in the background on acceptance, which is the first moment both ends
// are known. Without it the customer's "driver is on the way" map has a
// straight line and the approach-leg progress bar falls back to straight-line
// distance.
func (s *BookingService) fetchPickupRoute(bookingID, providerID string, pickup geo.Point) {
	if s.routes == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), routeFetchTimeout)
		defer cancel()

		status, ok, err := s.availability.GetProviderStatus(ctx, providerID)
		if err != nil || !ok || !status.Point().HasFix() {
			return
		}

		route, err := s.routes.Route(ctx, status.Point(), pickup, routing.TravelModeDrive)
		if err != nil {
			log.Printf("booking %s: pickup route unavailable, falling back to straight line: %v", bookingID, err)
			return
		}
		if !route.HasGeometry() {
			return
		}
		if err := s.bookings.SavePickupRoute(ctx, bookingID, route.Polyline, route.DistanceM, route.DurationS); err != nil {
			log.Printf("booking %s: could not store pickup route: %v", bookingID, err)
		}
	}()
}
