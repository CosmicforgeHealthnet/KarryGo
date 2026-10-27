package bookingusecases

import (
	"context"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/pricingclient"
)

type shadowCentral struct {
	quoted chan pricingclient.QuoteRequest
	bound  chan string
}

func (c *shadowCentral) Quote(_ context.Context, r pricingclient.QuoteRequest) (pricingclient.QuoteResponse, error) {
	c.quoted <- r
	return pricingclient.QuoteResponse{Quotes: []pricingclient.Quote{{ID: "pq_7"}}}, nil
}

func (c *shadowCentral) Bind(_ context.Context, id string, r pricingclient.BindRequest) (pricingclient.Binding, error) {
	c.bound <- id + "=" + r.BookingRef
	return pricingclient.Binding{}, nil
}

func TestHaulageShadowSendsTheChargedFareAndBinds(t *testing.T) {
	central := &shadowCentral{quoted: make(chan pricingclient.QuoteRequest, 1), bound: make(chan string, 1)}
	s := (&BookingService{}).WithPricingShadow(central)
	fare := int64(1035000)

	s.shadowPrice(bookingmodels.Booking{
		ID: "bk-h", PickupLat: 6.45, PickupLng: 3.39, FareEstimateKobo: &fare,
		CargoWeightKg: 2400, HelperCount: 2,
	}, 19.4049)

	select {
	case r := <-central.quoted:
		if r.ConsumerRef != "bk-h" || r.Scope != pricingclient.ScopeHaulage || r.Classes[0] != "other" {
			t.Fatalf("shadow request %+v", r)
		}
		if r.Route.DistanceMeters != 19405 || r.WeightGrams != 2400000 || r.Helpers != 2 || r.LegacyFareKobo["other"] != fare {
			t.Fatalf("shadow request attributes %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no shadow quote was requested")
	}
	select {
	case got := <-central.bound:
		if got != "pq_7=bk-h" {
			t.Fatalf("bound %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the shadow quote was not bound")
	}
}
