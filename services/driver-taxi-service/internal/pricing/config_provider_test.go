package pricing

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/routing"
	"cosmicforge/logistics/shared/go/geo"
)

// fakeRoutes is a minimal routing.RoutingClient for pricing tests: a fixed
// distance/duration per leg, so fare math can be checked against a known
// input rather than real geometry.
type fakeRoutes struct {
	distanceMPerLeg int64
	durationSPerLeg int64
	err             error
}

func (f *fakeRoutes) Route(context.Context, geo.Point, geo.Point) (routing.RouteResult, error) {
	if f.err != nil {
		return routing.RouteResult{}, f.err
	}
	return routing.RouteResult{DistanceM: f.distanceMPerLeg, DurationS: f.durationSPerLeg}, nil
}
func (f *fakeRoutes) Matrix(context.Context, []geo.Point, []geo.Point) (routing.MatrixResult, error) {
	return routing.MatrixResult{}, nil
}
func (f *fakeRoutes) SnapToRoad(context.Context, []geo.Point) (routing.SnappedRoute, error) {
	return routing.SnappedRoute{}, routing.ErrNotImplemented
}
func (f *fakeRoutes) ReverseGeocode(context.Context, float64, float64) (routing.AddressResult, error) {
	return routing.AddressResult{}, routing.ErrNotImplemented
}

var testRates = map[string]ClassRate{
	"economy": {BaseKobo: 70000, PerKmKobo: 18000, PerMinKobo: 4000, MinKobo: 100000},
	"comfort": {BaseKobo: 90000, PerKmKobo: 22000, PerMinKobo: 5000, MinKobo: 130000},
}

func TestParseRatesJSON_ParsesValidConfig(t *testing.T) {
	rates, err := ParseRatesJSON(`{"economy":{"base_kobo":70000,"per_km_kobo":18000,"per_min_kobo":4000,"min_kobo":100000}}`)
	if err != nil {
		t.Fatalf("ParseRatesJSON: %v", err)
	}
	if rates["economy"].BaseKobo != 70000 {
		t.Fatalf("unexpected parse result: %+v", rates)
	}
}

func TestParseRatesJSON_RejectsMalformedJSON(t *testing.T) {
	if _, err := ParseRatesJSON(`not json`); err == nil {
		t.Fatal("malformed config must fail loudly, not silently produce an empty map")
	}
}

func TestParseRatesJSON_RejectsNegativeRateComponents(t *testing.T) {
	if _, err := ParseRatesJSON(`{"economy":{"base_kobo":-1,"per_km_kobo":0,"per_min_kobo":0,"min_kobo":0}}`); err == nil {
		t.Fatal("a negative rate component must fail startup, not silently produce a negative fare")
	}
}

func TestClassRate_RoundsToNearest50Kobo(t *testing.T) {
	rate := ClassRate{BaseKobo: 70000, PerKmKobo: 18000, PerMinKobo: 4000, MinKobo: 100000}
	// 70000 + 18000*10.02 + 4000*8 = 70000 + 180360 + 32000 = 282360 -> nearest 50 = 282350
	got := rate.fare(10.02, 8)
	if got != 282350 {
		t.Fatalf("fare() = %d, want 282350", got)
	}
}

func TestClassRate_FloorsAtMinimum(t *testing.T) {
	rate := ClassRate{BaseKobo: 70000, PerKmKobo: 18000, PerMinKobo: 4000, MinKobo: 100000}
	// A very short hop computes well under the minimum.
	got := rate.fare(0.1, 1)
	if got != 100000 {
		t.Fatalf("fare() for a short hop = %d, want the floor 100000", got)
	}
}

func TestQuote_ReturnsOneQuotePerRecognizedClass(t *testing.T) {
	provider := NewConfigPricingProvider(testRates, "v1", &fakeRoutes{distanceMPerLeg: 5000, durationSPerLeg: 600})

	quotes, err := provider.Quote(context.Background(), QuoteRequest{
		Stops:          []geo.Point{{Lat: 6.52, Lng: 3.37}, {Lat: 6.60, Lng: 3.35}},
		VehicleClasses: []string{"economy", "comfort", "xl"}, // xl has no configured rate
	})
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("expected 2 quotes (xl has no rate and should be silently omitted), got %d: %+v", len(quotes), quotes)
	}
	for _, q := range quotes {
		if q.QuoteReference == "" {
			t.Fatalf("every quote needs a reference to snapshot on the booking: %+v", q)
		}
		if q.RateVersion != "v1" {
			t.Fatalf("expected rate version v1, got %q", q.RateVersion)
		}
		if q.ExpiresAt.Before(time.Now()) {
			t.Fatalf("a freshly issued quote must not already be expired: %+v", q)
		}
	}
}

func TestQuote_SumsMultiLegDistance(t *testing.T) {
	// 2 legs of 5km/10min each = 10km/20min total, not just the first leg.
	provider := NewConfigPricingProvider(testRates, "v1", &fakeRoutes{distanceMPerLeg: 5000, durationSPerLeg: 600})

	quotes, err := provider.Quote(context.Background(), QuoteRequest{
		Stops:          []geo.Point{{Lat: 6.52, Lng: 3.37}, {Lat: 6.55, Lng: 3.38}, {Lat: 6.60, Lng: 3.35}},
		VehicleClasses: []string{"economy"},
	})
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(quotes))
	}
	if quotes[0].DistanceKm != 10 {
		t.Fatalf("expected the two legs summed to 10km, got %v", quotes[0].DistanceKm)
	}
	if quotes[0].DurationMin != 20 {
		t.Fatalf("expected the two legs summed to 20min, got %v", quotes[0].DurationMin)
	}
}

func TestQuote_RequiresAtLeastPickupAndDropoff(t *testing.T) {
	provider := NewConfigPricingProvider(testRates, "v1", &fakeRoutes{})
	if _, err := provider.Quote(context.Background(), QuoteRequest{
		Stops:          []geo.Point{{Lat: 6.52, Lng: 3.37}},
		VehicleClasses: []string{"economy"},
	}); err == nil {
		t.Fatal("a single stop is not a route; Quote must reject it")
	}
}

func TestQuote_PropagatesARawRoutingErrorAsUnavailable(t *testing.T) {
	// This only matters if a caller passes a raw (non-fallback-wrapped) routes
	// client -- see the constructor's doc comment on why that should not
	// happen in production wiring.
	provider := NewConfigPricingProvider(testRates, "v1", &fakeRoutes{err: errors.New("boom")})
	_, err := provider.Quote(context.Background(), QuoteRequest{
		Stops:          []geo.Point{{Lat: 6.52, Lng: 3.37}, {Lat: 6.60, Lng: 3.35}},
		VehicleClasses: []string{"economy"},
	})
	if err == nil {
		t.Fatal("expected an error when the routing dependency fails outright")
	}
}

func TestReconcile_UsesRequestedClassRate(t *testing.T) {
	provider := NewConfigPricingProvider(testRates, "v1", &fakeRoutes{})
	final, err := provider.Reconcile(context.Background(), ReconcileRequest{
		QuoteReference: "ref-123", VehicleClass: "comfort", ActualDistanceKm: 10, ActualDurationMin: 15,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// 90000 + 22000*10 + 5000*15 = 90000 + 220000 + 75000 = 385000
	if final.FareKobo != 385000 {
		t.Fatalf("FareKobo = %d, want 385000", final.FareKobo)
	}
	if final.QuoteReference != "ref-123" {
		t.Fatal("the reference should be echoed back so the caller can correlate the settlement")
	}
}

func TestReconcile_RejectsUnknownClass(t *testing.T) {
	provider := NewConfigPricingProvider(testRates, "v1", &fakeRoutes{})
	if _, err := provider.Reconcile(context.Background(), ReconcileRequest{
		VehicleClass: "helicopter", ActualDistanceKm: 1, ActualDurationMin: 1,
	}); err == nil {
		t.Fatal("an unconfigured class must be rejected, not silently priced at zero")
	}
}
