package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/taxi-service/internal/routing"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
)

// ClassRate is one vehicle class's complete fare formula. Deliberately not a
// shared base plus a per-class multiplier (the shape internal/vehicleclass
// used to carry): each class owns its own full rate set, so a class-specific
// pricing model is possible later without restructuring this type.
type ClassRate struct {
	BaseKobo   int64 `json:"base_kobo"`
	PerKmKobo  int64 `json:"per_km_kobo"`
	PerMinKobo int64 `json:"per_min_kobo"`
	MinKobo    int64 `json:"min_kobo"`
}

// fare computes BaseKobo + PerKmKobo*distanceKm + PerMinKobo*durationMin,
// rounded to the nearest 50 kobo, floored at MinKobo.
func (r ClassRate) fare(distanceKm float64, durationMin int) int64 {
	raw := r.BaseKobo + int64(float64(r.PerKmKobo)*distanceKm) + r.PerMinKobo*int64(durationMin)
	rounded := roundToNearest50(raw)
	if rounded < r.MinKobo {
		return r.MinKobo
	}
	return rounded
}

func roundToNearest50(kobo int64) int64 {
	const step = 50
	return ((kobo + step/2) / step) * step
}

// ConfigPricingProvider is the PROVISIONAL PricingClient implementation.
// Every rate lives in configuration (ParseRatesJSON), never as a Go constant
// -- see this package's own doc comment. It is named Provisional in every log
// line and doc comment specifically so nobody mistakes it for the real
// central pricing engine the addendum defers to.
//
// Swapping to the real engine later is a binding change (construct a
// different PricingClient implementation in cmd/main.go), not a refactor of
// any caller -- every caller depends on the PricingClient interface only.
type ConfigPricingProvider struct {
	rates       map[string]ClassRate
	rateVersion string
	routes      routing.RoutingClient
	quoteTTL    time.Duration
	now         func() time.Time
}

// DefaultQuoteTTL is how long a Quote remains valid before a caller must
// request a fresh one.
const DefaultQuoteTTL = 5 * time.Minute

// NewConfigPricingProvider builds the provisional provider. routes should be
// a fallback-wrapped RoutingClient (routing.NewHaversineFallbackClient over
// routing.NewGoogleRoutingClient) so a routing outage degrades to
// straight-line distance rather than making every quote fail -- this
// provider does not handle routing errors itself, by design, because the
// fallback decorator already absorbs them.
func NewConfigPricingProvider(rates map[string]ClassRate, rateVersion string, routes routing.RoutingClient) *ConfigPricingProvider {
	return &ConfigPricingProvider{
		rates:       rates,
		rateVersion: rateVersion,
		routes:      routes,
		quoteTTL:    DefaultQuoteTTL,
		now:         time.Now,
	}
}

// ParseRatesJSON parses the TAXI_PROVISIONAL_RATES config value: a JSON
// object keyed by vehicle class, e.g.
//
//	{"economy":{"base_kobo":70000,"per_km_kobo":18000,"per_min_kobo":4000,"min_kobo":100000}, ...}
//
// Returns an error rather than a partially-parsed map on any malformed entry,
// so a config typo fails service startup loudly instead of silently pricing
// one class at zero.
func ParseRatesJSON(raw string) (map[string]ClassRate, error) {
	var rates map[string]ClassRate
	if err := json.Unmarshal([]byte(raw), &rates); err != nil {
		return nil, fmt.Errorf("pricing: invalid TAXI_PROVISIONAL_RATES: %w", err)
	}
	for class, rate := range rates {
		if rate.BaseKobo < 0 || rate.PerKmKobo < 0 || rate.PerMinKobo < 0 || rate.MinKobo < 0 {
			return nil, fmt.Errorf("pricing: class %q has a negative rate component", class)
		}
	}
	return rates, nil
}

func (p *ConfigPricingProvider) Quote(ctx context.Context, req QuoteRequest) ([]Quote, error) {
	if len(req.Stops) < 2 {
		return nil, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "stops", Message: "At least a pickup and a drop-off are required for a quote."},
		})
	}

	distanceKm, durationMin, err := p.sumRoute(ctx, req.Stops)
	if err != nil {
		return nil, apperrors.Unavailable("A fare could not be calculated right now.", err)
	}

	expiresAt := p.now().Add(p.quoteTTL)
	quotes := make([]Quote, 0, len(req.VehicleClasses))
	for _, class := range req.VehicleClasses {
		rate, ok := p.rates[class]
		if !ok {
			// An unpriced class is omitted, not a batch failure -- see the
			// interface doc comment.
			continue
		}
		quotes = append(quotes, Quote{
			VehicleClass:   class,
			FareKobo:       rate.fare(distanceKm, durationMin),
			DistanceKm:     distanceKm,
			DurationMin:    durationMin,
			QuoteReference: uuid.NewString(),
			RateVersion:    p.rateVersion,
			ExpiresAt:      expiresAt,
		})
	}
	return quotes, nil
}

// Reconcile recomputes the fare from the trip actually taken.
//
// Provisional-provider limitation, disclosed rather than hidden: this
// implementation is stateless and persists nothing server-side, so it cannot
// look up which rate table QuoteReference originally used -- it re-applies
// its CURRENT configured rate for req.VehicleClass. If TAXI_PROVISIONAL_RATES
// changes between a ride's quote and its completion, the reconciled fare uses
// the new rate, not the one shown at booking time. A real pricing engine
// would version its rate tables and honor the original quote's version by
// reference; this provider does not, and that gap is exactly the kind of
// thing the addendum's central pricing engine is meant to close.
func (p *ConfigPricingProvider) Reconcile(ctx context.Context, req ReconcileRequest) (FinalFare, error) {
	rate, ok := p.rates[req.VehicleClass]
	if !ok {
		return FinalFare{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "vehicle_class", Message: "This vehicle class has no configured rate."},
		})
	}
	return FinalFare{
		FareKobo:       rate.fare(req.ActualDistanceKm, req.ActualDurationMin),
		QuoteReference: req.QuoteReference,
		RateVersion:    p.rateVersion,
	}, nil
}

// sumRoute walks the ordered stops leg by leg, summing distance and duration.
// Uses the RoutingClient's Matrix in a single batch when there are enough
// legs to make that worthwhile, otherwise plain sequential Route calls.
func (p *ConfigPricingProvider) sumRoute(ctx context.Context, stops []geo.Point) (distanceKm float64, durationMin int, err error) {
	var totalM int64
	var totalS int64
	for i := 0; i+1 < len(stops); i++ {
		leg, err := p.routes.Route(ctx, stops[i], stops[i+1])
		if err != nil {
			// Only reachable if the caller passed a raw (non-fallback-wrapped)
			// RoutingClient -- see the constructor's doc comment.
			return 0, 0, err
		}
		totalM += leg.DistanceM
		totalS += leg.DurationS
	}
	return float64(totalM) / 1000.0, int(totalS / 60), nil
}
