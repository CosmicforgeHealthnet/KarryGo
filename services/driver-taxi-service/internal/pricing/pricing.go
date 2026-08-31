// Package pricing is the boundary the addendum draws around Taxi: this
// service consumes fare quotes, it does not compute permanent commercial
// rates. Addendum §1: "Taxi will consume an authoritative platform pricing
// service when that service is defined. Taxi must not hard-code permanent
// commercial rates." §37 explicitly excludes "per-kilometre/per-metre rates,
// base fare values... minimum fare" from this service's scope.
//
// No implementation in this package may contain a rate, a multiplier, or a
// minimum fare as a Go constant. The one provisional implementation
// (ConfigPricingProvider) reads every number from configuration precisely so
// that rule holds even before the real central pricing engine exists.
package pricing

import (
	"context"
	"time"

	"cosmicforge/logistics/shared/go/geo"
)

// Quote is one vehicle class's fare for a route, as returned by the pricing
// authority. Bookings store this as a snapshot (FareKobo) plus a reference
// (QuoteReference, RateVersion) -- never recomputed locally later, so a
// future rate change can never silently restate a historical ride.
type Quote struct {
	VehicleClass   string
	FareKobo       int64
	DistanceKm     float64
	DurationMin    int
	QuoteReference string
	RateVersion    string
	ExpiresAt      time.Time
}

// FinalFare is the reconciled fare at trip completion, computed from the
// actual distance/duration traveled rather than the original quote's
// estimate.
type FinalFare struct {
	FareKobo       int64
	QuoteReference string
	RateVersion    string
}

// QuoteRequest asks for one quote per requested vehicle class over an ordered
// route. Stops must have at least 2 points (pickup + dropoff); more than 2
// means a multi-stop / hourly-mode itinerary, and distance/duration are
// summed leg by leg in order.
type QuoteRequest struct {
	MarketID        string
	OperatingZoneID string
	Stops           []geo.Point
	VehicleClasses  []string
}

// ReconcileRequest recomputes a fare from what actually happened on the trip.
// VehicleClass is required: a stateless provisional provider (see
// ConfigPricingProvider) has nowhere else to look up which rate table the
// original quote used, since it persists nothing server-side.
type ReconcileRequest struct {
	QuoteReference    string
	VehicleClass      string
	ActualDistanceKm  float64
	ActualDurationMin int
}

// PricingClient is the only way any fare number may enter this service.
type PricingClient interface {
	// Quote returns one Quote per class in req.VehicleClasses that the
	// provider recognizes. An unrecognized class is silently omitted from the
	// result rather than failing the whole batch -- the caller (booking
	// creation) is expected to treat a missing class as "not quotable right
	// now" for that one class, not abort the entire fare-tile screen.
	Quote(ctx context.Context, req QuoteRequest) ([]Quote, error)
	// Reconcile computes the final fare at trip completion.
	Reconcile(ctx context.Context, req ReconcileRequest) (FinalFare, error)
}
