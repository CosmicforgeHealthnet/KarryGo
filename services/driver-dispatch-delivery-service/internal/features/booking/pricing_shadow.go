package booking

import (
	"context"
	"log"
	"math"
	"time"

	"cosmicforge/logistics/shared/go/pricingclient"
)

// pricingShadowClass is the vehicle class dispatch prices: every customer
// delivery today is a motorcycle job.
const pricingShadowClass = "MOTORCYCLE"

// CentralPricing is the slice of the central pricing engine's client the
// shadow needs.
type CentralPricing interface {
	Quote(ctx context.Context, request pricingclient.QuoteRequest) (pricingclient.QuoteResponse, error)
	Bind(ctx context.Context, quoteID string, request pricingclient.BindRequest) (pricingclient.Binding, error)
}

// WithPricingShadow asks the central pricing engine to price every new
// booking as well, for comparison with the fare actually charged. It never
// changes a fare.
func (s *Service) WithPricingShadow(central CentralPricing) *Service {
	s.pricingShadow = central
	return s
}

// shadowPrice asks pricing what it would have charged for a booking that has
// just been created, and binds that quote to the booking. It runs in the
// background with its own deadline: pricing being slow or down can never slow
// or fail a booking, and a failure is only logged.
//
// distanceKM is the unrounded distance the charged fare was computed from;
// the booking stores a rounded copy, which would make the comparison drift.
func (s *Service) shadowPrice(created Booking, distanceKM float64) {
	if s.pricingShadow == nil {
		return
	}
	central := s.pricingShadow
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		response, err := central.Quote(ctx, pricingclient.QuoteRequest{
			ConsumerRef:    created.ID,
			Scope:          pricingclient.ScopeDispatch,
			Channel:        pricingclient.ChannelCustomerApp,
			Classes:        []string{pricingShadowClass},
			Pickup:         &pricingclient.Point{Lat: created.PickupLat, Lng: created.PickupLng},
			Route:          pricingclient.Route{DistanceMeters: int64(math.Round(distanceKM * 1000))},
			PackageSize:    created.PackageSize,
			ServiceTier:    created.ServiceTier,
			LegacyFareKobo: map[string]int64{pricingShadowClass: created.FareAmount},
		})
		if err != nil {
			log.Printf("pricing shadow: quote booking_id=%s: %v", created.ID, err)
			return
		}
		for _, quote := range response.Quotes {
			if _, err := central.Bind(ctx, quote.ID, pricingclient.BindRequest{BookingRef: created.ID}); err != nil {
				log.Printf("pricing shadow: bind booking_id=%s quote=%s: %v", created.ID, quote.ID, err)
			}
		}
	}()
}
