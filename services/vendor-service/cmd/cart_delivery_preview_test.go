package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"testing"
	"time"
)

type previewMemoryCart struct{ current cart.Cart }

func (m *previewMemoryCart) GetCart(_ context.Context, customer, id string) (cart.Cart, error) {
	if customer != "customer" || id != m.current.ID {
		return cart.Cart{}, cart.ErrCartNotFound
	}
	return m.current, nil
}

type previewCatalogue struct{}

func (previewCatalogue) RevalidateFoodLine(_ context.Context, r checkout.LineRequest) (checkout.LineDecision, error) {
	return checkout.LineDecision{Sellable: true, BusinessID: "vendor", BranchID: "branch", Currency: "NGN", UnitPriceMinor: 100}, nil
}
func (p previewCatalogue) RevalidateShopLine(c context.Context, r checkout.LineRequest) (checkout.LineDecision, error) {
	return p.RevalidateFoodLine(c, r)
}

type previewRecordingQuote struct {
	requests []fulfilment.QuoteOnlyRequest
	during   func()
}

func (q *previewRecordingQuote) QuotePlan(_ context.Context, r fulfilment.QuoteOnlyRequest) (fulfilment.QuoteOnlyResult, error) {
	q.requests = append(q.requests, r)
	if q.during != nil {
		q.during()
	}
	return fulfilment.QuoteOnlyResult{Serviceable: true, DeliveryCustomerMinor: 321, Currency: "NGN"}, nil
}
func previewFixture() (*previewMemoryCart, *previewRecordingQuote, cartPreview) {
	m := &previewMemoryCart{current: cart.Cart{ID: "cart", Domain: cart.DomainShop, Status: cart.StatusActive, Version: 1, ExpiresAt: time.Now().Add(time.Hour), Address: &cart.AddressSnapshot{Address: "Destination", Latitude: 6.5, Longitude: 3.4}, Items: []cart.CartItem{{ID: "line", Domain: cart.DomainShop, BusinessID: "vendor", BranchID: "branch", ItemRef: "item", ListingRef: "listing", SellUnitRef: "unit", Quantity: 1, PricingBasis: cart.PricingBasisFixedUnit}}}}
	q := &previewRecordingQuote{}
	return m, q, cartPreview{carts: m, food: previewCatalogue{}, shop: previewCatalogue{}, quotes: q, enabled: func(cart.Domain) bool { return true }}
}
func TestCartDeliveryPreviewRecomputesMaterialChangesAndIsNonCommitting(t *testing.T) {
	m, q, p := previewFixture()
	first, err := p.preview(context.Background(), "customer", "cart")
	if err != nil {
		t.Fatal(err)
	}
	m.current.Items[0].Quantity = 30
	m.current.Version++
	second, err := p.preview(context.Background(), "customer", "cart")
	if err != nil {
		t.Fatal(err)
	}
	if first["fingerprint"] == second["fingerprint"] || len(q.requests) != 2 || q.requests[1].Groups[0].Lines[0].Quantity != 30 {
		t.Fatal("quantity did not revalidate/requote")
	}
	m.current.Address = &cart.AddressSnapshot{Address: "New recipient destination", Latitude: 6.6, Longitude: 3.5}
	m.current.Receiver = &cart.Receiver{Name: "Receiver", Phone: "123"}
	m.current.Version++
	third, err := p.preview(context.Background(), "customer", "cart")
	if err != nil {
		t.Fatal(err)
	}
	if third["fingerprint"] == second["fingerprint"] || q.requests[2].Destination.Latitude != 6.6 {
		t.Fatal("route/receiver evidence stale")
	}
	if third["delivery_customer_minor"] != int64(321) || third["guaranteed"] != false {
		t.Fatal("authority amount or preview semantics lost")
	}
	if _, err = p.preview(context.Background(), "other", "cart"); err != cart.ErrCartNotFound {
		t.Fatal("IDOR accepted")
	}
	q.during = func() { m.current.Version++ }
	if _, err = p.preview(context.Background(), "customer", "cart"); err != cart.ErrVersionConflict {
		t.Fatalf("concurrent mutation must reject: %v", err)
	}
}
