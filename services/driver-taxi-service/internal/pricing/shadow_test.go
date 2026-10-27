package pricing

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/pricingclient"
)

type fixedProvider struct{}

func (fixedProvider) Quote(_ context.Context, req QuoteRequest) ([]Quote, error) {
	return []Quote{{VehicleClass: "economy", FareKobo: 165000, DistanceKm: 8, DurationMin: 20, QuoteReference: "q-1"}}, nil
}

func (fixedProvider) Reconcile(_ context.Context, req ReconcileRequest) (FinalFare, error) {
	return FinalFare{FareKobo: 195000, QuoteReference: req.QuoteReference}, nil
}

type recordingCentral struct {
	quotes     []pricingclient.QuoteRequest
	bound      []string
	reconciled []pricingclient.ReconcileRequest
	fail       bool
}

func (c *recordingCentral) Quote(_ context.Context, r pricingclient.QuoteRequest) (pricingclient.QuoteResponse, error) {
	c.quotes = append(c.quotes, r)
	if c.fail {
		return pricingclient.QuoteResponse{}, errors.New("pricing is down")
	}
	return pricingclient.QuoteResponse{Quotes: []pricingclient.Quote{{ID: "pq_1"}}}, nil
}

func (c *recordingCentral) FindByRef(_ context.Context, ref, class string) (pricingclient.Quote, error) {
	if c.fail {
		return pricingclient.Quote{}, errors.New("pricing is down")
	}
	return pricingclient.Quote{ID: "pq_1"}, nil
}

func (c *recordingCentral) Bind(_ context.Context, id string, r pricingclient.BindRequest) (pricingclient.Binding, error) {
	c.bound = append(c.bound, id+"="+r.BookingRef)
	return pricingclient.Binding{}, nil
}

func (c *recordingCentral) Reconcile(_ context.Context, id string, r pricingclient.ReconcileRequest) (pricingclient.Reconciliation, error) {
	c.reconciled = append(c.reconciled, r)
	return pricingclient.Reconciliation{}, nil
}

func synchronous(inner PricingClient, central CentralPricing) *ShadowPricingClient {
	s := NewShadowPricingClient(inner, central)
	s.run = func(f func()) { f() }
	return s
}

func TestShadowKeepsTheLegacyFareAndReportsIt(t *testing.T) {
	central := &recordingCentral{}
	s := synchronous(fixedProvider{}, central)

	quotes, err := s.Quote(context.Background(), QuoteRequest{
		OperatingZoneID: "zone-lagos",
		Stops:           []geo.Point{{Lat: 6.45, Lng: 3.39}, {Lat: 6.6, Lng: 3.35}},
		VehicleClasses:  []string{"economy"},
	})
	if err != nil || len(quotes) != 1 || quotes[0].FareKobo != 165000 {
		t.Fatalf("the customer must see the legacy fare unchanged: %+v, %v", quotes, err)
	}
	if len(central.quotes) != 1 {
		t.Fatalf("want one shadow quote, got %d", len(central.quotes))
	}
	asked := central.quotes[0]
	if asked.ConsumerRef != "q-1" || asked.ConsumerZoneID != "zone-lagos" || asked.Pickup == nil || asked.Pickup.Lat != 6.45 {
		t.Fatalf("shadow request %+v", asked)
	}
	if asked.Route.DistanceMeters != 8000 || asked.Route.DurationSeconds != 1200 || asked.LegacyFareKobo["economy"] != 165000 {
		t.Fatalf("shadow route/legacy %+v", asked)
	}

	final, err := s.Reconcile(context.Background(), ReconcileRequest{QuoteReference: "q-1", VehicleClass: "economy", ActualDistanceKm: 9.25, ActualDurationMin: 24})
	if err != nil || final.FareKobo != 195000 {
		t.Fatalf("the final fare must be the legacy one: %+v, %v", final, err)
	}
	if len(central.bound) != 1 || central.bound[0] != "pq_1=q-1" || len(central.reconciled) != 1 {
		t.Fatalf("want bind and reconcile, got bound=%v reconciled=%v", central.bound, central.reconciled)
	}
	if r := central.reconciled[0]; r.Route.DistanceMeters != 9250 || r.LegacyFinalKobo == nil || *r.LegacyFinalKobo != 195000 {
		t.Fatalf("reconcile request %+v", r)
	}
}

func TestShadowFailureNeverReachesTheRide(t *testing.T) {
	central := &recordingCentral{fail: true}
	s := synchronous(fixedProvider{}, central)

	quotes, err := s.Quote(context.Background(), QuoteRequest{
		Stops: []geo.Point{{Lat: 6.45, Lng: 3.39}, {Lat: 6.6, Lng: 3.35}}, VehicleClasses: []string{"economy"},
	})
	if err != nil || len(quotes) != 1 || quotes[0].FareKobo != 165000 {
		t.Fatalf("pricing being down must not affect the quote: %+v, %v", quotes, err)
	}
	final, err := s.Reconcile(context.Background(), ReconcileRequest{QuoteReference: "q-1", VehicleClass: "economy", ActualDistanceKm: 9})
	if err != nil || final.FareKobo != 195000 {
		t.Fatalf("pricing being down must not affect completion: %+v, %v", final, err)
	}
	if len(central.bound) != 0 {
		t.Fatal("nothing should be bound when the quote cannot be found")
	}
}
