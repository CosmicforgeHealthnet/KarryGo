package pricing

import (
	"context"
	"math"
	"time"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/pricingclient"
)

// CentralPricing is the slice of the central pricing engine's client the
// shadow needs.
type CentralPricing interface {
	Quote(ctx context.Context, request pricingclient.QuoteRequest) (pricingclient.QuoteResponse, error)
	FindByRef(ctx context.Context, consumerRef, class string) (pricingclient.Quote, error)
	Bind(ctx context.Context, quoteID string, request pricingclient.BindRequest) (pricingclient.Binding, error)
	Reconcile(ctx context.Context, quoteID string, request pricingclient.ReconcileRequest) (pricingclient.Reconciliation, error)
}

// ShadowPricingClient prices exactly as the wrapped provider does, and also
// asks the central pricing engine the same question so the two can be
// compared before the engine is ever trusted with a real fare.
//
// The shadow can never change a fare, slow a booking or fail one: the wrapped
// provider's answer is returned unchanged, and the engine is asked afterwards
// in the background with its own short deadline. A shadow failure is logged
// and forgotten.
type ShadowPricingClient struct {
	inner   PricingClient
	central CentralPricing
	timeout time.Duration
	// run starts background work; tests replace it to run synchronously.
	run func(func())
}

// NewShadowPricingClient wraps inner.
func NewShadowPricingClient(inner PricingClient, central CentralPricing) *ShadowPricingClient {
	return &ShadowPricingClient{inner: inner, central: central, timeout: 5 * time.Second, run: func(f func()) { go f() }}
}

// Quote returns the wrapped provider's quotes and shadows each one.
func (s *ShadowPricingClient) Quote(ctx context.Context, req QuoteRequest) ([]Quote, error) {
	quotes, err := s.inner.Quote(ctx, req)
	if err != nil || len(quotes) == 0 || len(req.Stops) == 0 {
		return quotes, err
	}
	pickup := req.Stops[0]
	zone := req.OperatingZoneID
	snapshot := append([]Quote(nil), quotes...)
	s.run(func() {
		bg, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()
		for _, quote := range snapshot {
			_, err := s.central.Quote(bg, pricingclient.QuoteRequest{
				// The provider's own quote reference is unique per quote and is
				// what the booking stores, so reconciliation can find this again.
				ConsumerRef:    quote.QuoteReference,
				Scope:          pricingclient.ScopeTaxi,
				Channel:        pricingclient.ChannelCustomerApp,
				Classes:        []string{quote.VehicleClass},
				ConsumerZoneID: zone,
				Pickup:         &pricingclient.Point{Lat: pickup.Lat, Lng: pickup.Lng},
				Route:          routeFor(quote.DistanceKm, quote.DurationMin),
				LegacyFareKobo: map[string]int64{quote.VehicleClass: quote.FareKobo},
			})
			if err != nil {
				logging.Error("pricing-shadow", "quote %s (%s): %v", quote.QuoteReference, quote.VehicleClass, err)
			}
		}
	})
	return quotes, nil
}

// Reconcile returns the wrapped provider's final fare and shadows it: the
// engine's quote is found by reference, bound, and priced from the actual trip.
func (s *ShadowPricingClient) Reconcile(ctx context.Context, req ReconcileRequest) (FinalFare, error) {
	final, err := s.inner.Reconcile(ctx, req)
	if err != nil || req.QuoteReference == "" {
		return final, err
	}
	legacy := final.FareKobo
	s.run(func() {
		bg, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()
		quote, err := s.central.FindByRef(bg, req.QuoteReference, req.VehicleClass)
		if err != nil {
			logging.Error("pricing-shadow", "find quote %s: %v", req.QuoteReference, err)
			return
		}
		if _, err := s.central.Bind(bg, quote.ID, pricingclient.BindRequest{BookingRef: req.QuoteReference}); err != nil {
			logging.Error("pricing-shadow", "bind quote %s: %v", quote.ID, err)
			return
		}
		if _, err := s.central.Reconcile(bg, quote.ID, pricingclient.ReconcileRequest{
			Route:           routeFor(req.ActualDistanceKm, req.ActualDurationMin),
			LegacyFinalKobo: &legacy,
		}); err != nil {
			logging.Error("pricing-shadow", "reconcile quote %s: %v", quote.ID, err)
		}
	})
	return final, nil
}

// routeFor converts this service's kilometres and minutes into the whole
// metres and seconds the engine prices with.
func routeFor(distanceKm float64, durationMin int) pricingclient.Route {
	return pricingclient.Route{
		DistanceMeters:  int64(math.Round(distanceKm * 1000)),
		DurationSeconds: int64(durationMin) * 60,
		Confidence:      pricingclient.RouteConfidenceMedium,
	}
}
