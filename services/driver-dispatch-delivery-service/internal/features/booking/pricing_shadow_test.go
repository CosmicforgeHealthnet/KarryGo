package booking

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/pricingclient"
)

type shadowCentral struct {
	quoted chan pricingclient.QuoteRequest
	bound  chan string
}

func (c *shadowCentral) Quote(_ context.Context, r pricingclient.QuoteRequest) (pricingclient.QuoteResponse, error) {
	c.quoted <- r
	return pricingclient.QuoteResponse{Quotes: []pricingclient.Quote{{ID: "pq_9"}}}, nil
}

func (c *shadowCentral) Bind(_ context.Context, id string, r pricingclient.BindRequest) (pricingclient.Binding, error) {
	c.bound <- id + "=" + r.BookingRef
	return pricingclient.Binding{}, nil
}

func TestShadowPriceSendsTheChargedFareAndBinds(t *testing.T) {
	central := &shadowCentral{quoted: make(chan pricingclient.QuoteRequest, 1), bound: make(chan string, 1)}
	s := (&Service{}).WithPricingShadow(central)

	s.shadowPrice(Booking{
		ID: "bk-1", PickupLat: 6.45, PickupLng: 3.39, FareAmount: 172500,
		PackageSize: "medium", ServiceTier: "express",
	}, 7.3456)

	select {
	case r := <-central.quoted:
		if r.ConsumerRef != "bk-1" || r.Scope != pricingclient.ScopeDispatch || r.Route.DistanceMeters != 7346 {
			t.Fatalf("shadow request %+v", r)
		}
		if r.LegacyFareKobo[pricingShadowClass] != 172500 || r.PackageSize != "medium" || r.ServiceTier != "express" {
			t.Fatalf("shadow request attributes %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no shadow quote was requested")
	}
	select {
	case got := <-central.bound:
		if got != "pq_9=bk-1" {
			t.Fatalf("bound %s, want pq_9=bk-1", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the shadow quote was not bound to the booking")
	}
}

func TestNoShadowWithoutPricing(t *testing.T) {
	// Must not panic or block when the shadow is not configured.
	(&Service{}).shadowPrice(Booking{ID: "bk-2"}, 1)
}
