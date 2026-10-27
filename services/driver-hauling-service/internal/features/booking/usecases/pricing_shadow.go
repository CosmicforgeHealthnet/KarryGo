package bookingusecases

import (
	"context"
	"math"
	"strings"
	"time"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/pricingclient"
)

// CentralPricing is the slice of the central pricing engine's client the
// shadow needs.
type CentralPricing interface {
	Quote(ctx context.Context, request pricingclient.QuoteRequest) (pricingclient.QuoteResponse, error)
	Bind(ctx context.Context, quoteID string, request pricingclient.BindRequest) (pricingclient.Binding, error)
}

// WithPricingShadow asks the central pricing engine to price every new
// booking as well, for comparison with the fare actually charged. It never
// changes a fare.
func (s *BookingService) WithPricingShadow(central CentralPricing) *BookingService {
	s.pricingShadow = central
	return s
}

// shadowPrice asks pricing what it would have charged for a booking that has
// just been created, and binds that quote to the booking. It runs in the
// background with its own deadline: pricing being slow or down can never slow
// or fail a booking, and a failure is only logged.
//
// distanceKm is the unrounded straight-line distance the charged fare was
// computed from.
func (s *BookingService) shadowPrice(created bookingmodels.Booking, distanceKm float64) {
	if s.pricingShadow == nil || created.FareEstimateKobo == nil {
		return
	}
	central := s.pricingShadow
	class := strings.TrimSpace(created.PreferredTruckType)
	if class == "" {
		// Every truck type pays the same today; "other" is the haulage
		// book's catch-all class.
		class = "other"
	}
	legacy := *created.FareEstimateKobo
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		response, err := central.Quote(ctx, pricingclient.QuoteRequest{
			ConsumerRef:    created.ID,
			Scope:          pricingclient.ScopeHaulage,
			Channel:        pricingclient.ChannelCustomerApp,
			Classes:        []string{class},
			Pickup:         &pricingclient.Point{Lat: created.PickupLat, Lng: created.PickupLng},
			Route:          pricingclient.Route{DistanceMeters: int64(math.Round(distanceKm * 1000))},
			WeightGrams:    int64(created.CargoWeightKg) * 1000,
			Helpers:        int64(created.HelperCount),
			LegacyFareKobo: map[string]int64{class: legacy},
		})
		if err != nil {
			logging.Error("pricing-shadow", "quote booking=%s: %v", created.ID, err)
			return
		}
		for _, quote := range response.Quotes {
			if _, err := central.Bind(ctx, quote.ID, pricingclient.BindRequest{BookingRef: created.ID}); err != nil {
				logging.Error("pricing-shadow", "bind booking=%s quote=%s: %v", created.ID, quote.ID, err)
			}
		}
	}()
}
