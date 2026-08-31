package serviceability

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRoutes struct {
	result RouteResult
	err    error
}

func (f fakeRoutes) Route(context.Context, Point, Point, string) (RouteResult, error) {
	return f.result, f.err
}

type fakeZones struct{ pickup, dropoff, restricted, available bool }

func (f fakeZones) Evaluate(Point, Point) (bool, bool, bool, bool) {
	return f.pickup, f.dropoff, f.restricted, f.available
}

type memoryStore struct{ byKey map[string]Quote }

func (s *memoryStore) CreateOrReplay(_ context.Context, quote Quote) (Quote, error) {
	if s.byKey == nil {
		s.byKey = map[string]Quote{}
	}
	key := quote.CustomerID + ":" + quote.IdempotencyKey
	if existing, ok := s.byKey[key]; ok {
		if existing.RequestFingerprint != quote.RequestFingerprint {
			return Quote{}, ErrFingerprintConflict
		}
		return existing, nil
	}
	s.byKey[key] = quote
	return quote, nil
}

func validRequest() Request {
	return Request{Pickup: Point{Latitude: 6.5244, Longitude: 3.3792}, Dropoff: Point{Latitude: 6.60, Longitude: 3.40}, VehicleClass: "motorcycle", ServiceTier: "standard", Package: normalPackage()}
}

func serviceFor(store Store, routes RoutingAdapter, zones ZoneEvaluator, degraded bool, now time.Time) *Service {
	return New(Options{Store: store, Routes: routes, Zones: zones, DegradedMode: degraded, TTL: 10 * time.Minute, MaxRouteKM: 50, Now: func() time.Time { return now }})
}

func TestServiceabilityStructuredDecisions(t *testing.T) {
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	road := RouteResult{DistanceMeters: 12_000, DurationSeconds: 1800, Source: "route-authority", EvidenceVersion: "road-v1", Confidence: "AUTHORITATIVE"}
	tests := []struct {
		name       string
		request    func() Request
		routes     fakeRoutes
		zones      fakeZones
		degraded   bool
		want       Decision
		wantReason string
	}{
		{"serviceable", validRequest, fakeRoutes{result: road}, fakeZones{true, true, false, true}, false, DecisionServiceable, ReasonServiceable},
		{"pickup outside", validRequest, fakeRoutes{result: road}, fakeZones{false, true, false, true}, false, DecisionOutsideServiceArea, ReasonPickupOutsideZone},
		{"dropoff outside", validRequest, fakeRoutes{result: road}, fakeZones{true, false, false, true}, false, DecisionOutsideServiceArea, ReasonDropoffOutsideZone},
		{"restricted area", validRequest, fakeRoutes{result: road}, fakeZones{true, true, true, true}, false, DecisionOperationalReview, ReasonRestrictedArea},
		{"zone policy unavailable", validRequest, fakeRoutes{result: road}, fakeZones{}, false, DecisionOperationalReview, ReasonPolicyReviewRequired},
		{"route unavailable fail closed", validRequest, fakeRoutes{err: errors.New("down")}, fakeZones{true, true, false, true}, false, DecisionOperationalReview, ReasonRouteUnavailable},
		{"degraded explicit", validRequest, fakeRoutes{err: errors.New("down")}, fakeZones{true, true, false, true}, true, DecisionServiceableWithConditions, ReasonDegradedRouting},
		{"unsupported vehicle", func() Request { r := validRequest(); r.VehicleClass = "CAR"; return r }, fakeRoutes{result: road}, fakeZones{true, true, false, true}, false, DecisionOutsideServiceArea, ReasonVehicleUnsupported},
		{"package refusal", func() Request { r := validRequest(); r.Package.CategoryCode = "PHARMACEUTICAL"; return r }, fakeRoutes{result: road}, fakeZones{true, true, false, true}, false, DecisionOperationalReview, ReasonPackageRefused},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			quote, err := serviceFor(&memoryStore{}, test.routes, test.zones, test.degraded, now).Evaluate(context.Background(), "9bcd1c37-edcc-4206-a2a8-37622f999999", "key-"+string(rune('a'+index)), test.request())
			if err != nil {
				t.Fatal(err)
			}
			if quote.Decision != test.want || len(quote.ReasonCodes) == 0 || quote.ReasonCodes[0] != test.wantReason {
				t.Fatalf("quote=%+v", quote)
			}
			if !quote.ExpiresAt.Equal(now.Add(10*time.Minute)) || quote.PolicyVersion != PolicyVersion || quote.Admission.PolicyVersion != PackagePolicyVersion {
				t.Fatalf("policy/expiry evidence=%+v", quote)
			}
			if test.want == DecisionServiceable && (quote.FareEstimateKobo <= 0 || quote.Currency != "NGN") {
				t.Fatalf("pricing not reused: %+v", quote)
			}
		})
	}
}

func TestServiceabilityIdempotencyAndFingerprintConflict(t *testing.T) {
	store := &memoryStore{}
	service := serviceFor(store, fakeRoutes{result: RouteResult{DistanceMeters: 5000, DurationSeconds: 900, Source: "roads", EvidenceVersion: "v1"}}, fakeZones{true, true, false, true}, false, time.Now())
	request := validRequest()
	first, err := service.Evaluate(context.Background(), "9bcd1c37-edcc-4206-a2a8-37622f999999", "same-key", request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Evaluate(context.Background(), "9bcd1c37-edcc-4206-a2a8-37622f999999", "same-key", request)
	if err != nil || first.ID != second.ID || !first.CreatedAt.Equal(second.CreatedAt) {
		t.Fatalf("replay changed result: first=%+v second=%+v err=%v", first, second, err)
	}
	request.Dropoff.Latitude += .01
	if _, err = service.Evaluate(context.Background(), "9bcd1c37-edcc-4206-a2a8-37622f999999", "same-key", request); !errors.Is(err, ErrFingerprintConflict) {
		t.Fatalf("changed fingerprint err=%v", err)
	}
}

func TestQuoteExpiry(t *testing.T) {
	now := time.Now().UTC()
	quote, err := serviceFor(&memoryStore{}, fakeRoutes{result: RouteResult{DistanceMeters: 5000, DurationSeconds: 900, Source: "roads", EvidenceVersion: "v1"}}, fakeZones{true, true, false, true}, false, now).Evaluate(context.Background(), "9bcd1c37-edcc-4206-a2a8-37622f999999", "expiry", validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if quote.Expired(now.Add(9*time.Minute)) || !quote.Expired(now.Add(10*time.Minute)) {
		t.Fatal("quote expiry boundary incorrect")
	}
}
