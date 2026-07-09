package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

// --- in-memory delivery fakes ----------------------------------------------

type memBookings struct {
	m map[string]*delivery.PartnerBooking
}

func newMemBookings() *memBookings { return &memBookings{m: map[string]*delivery.PartnerBooking{}} }

func (r *memBookings) CreatePendingBooking(_ context.Context, b *delivery.PartnerBooking) error {
	cp := *b
	r.m[b.ID] = &cp
	return nil
}
func (r *memBookings) UpdatePaymentAndDispatching(_ context.Context, id, pay, dispatch string, at time.Time) error {
	if b, ok := r.m[id]; ok {
		b.Status = delivery.StatusDispatching
		b.PaymentIntentID = &pay
		b.DispatchBookingID = &dispatch
		b.EventPublishedAt = &at
	}
	return nil
}
func (r *memBookings) FindByPartnerAndIDOrRef(_ context.Context, partnerID, idOrRef string) (*delivery.PartnerBooking, error) {
	for _, b := range r.m {
		if b.PartnerID == partnerID && (b.ID == idOrRef || b.BookingRef == idOrRef) {
			cp := *b
			return &cp, nil
		}
	}
	return nil, nil
}
func (r *memBookings) ListByPartner(_ context.Context, partnerID string, f delivery.ListFilter) ([]delivery.PartnerBooking, int, error) {
	var out []delivery.PartnerBooking
	for _, b := range r.m {
		if b.PartnerID == partnerID && (f.Status == "" || b.Status == f.Status) {
			out = append(out, *b)
		}
	}
	return out, len(out), nil
}
func (r *memBookings) UpdateStatus(_ context.Context, id, status string) error {
	if b, ok := r.m[id]; ok {
		b.Status = status
	}
	return nil
}

type memIdem struct {
	m map[string]*delivery.IdempotencyRecord
}

func newMemIdem() *memIdem { return &memIdem{m: map[string]*delivery.IdempotencyRecord{}} }
func (s *memIdem) Get(_ context.Context, key string) (*delivery.IdempotencyRecord, error) {
	if r, ok := s.m[key]; ok {
		return r, nil
	}
	return nil, nil
}
func (s *memIdem) Put(_ context.Context, rec delivery.IdempotencyRecord) error {
	cp := rec
	s.m[rec.Key] = &cp
	return nil
}

type okWallet struct{}

func (okWallet) CreatePaymentIntent(context.Context, delivery.PaymentIntentInput) (delivery.PaymentIntentResult, error) {
	return delivery.PaymentIntentResult{ID: "intent", Reference: "PAY-REF"}, nil
}
func (okWallet) PayFromWallet(context.Context, string, string) error       { return nil }
func (okWallet) RequestRefund(context.Context, delivery.RefundInput) error { return nil }

type okPublisher struct{}

func (okPublisher) Publish(context.Context, string, []byte) error { return nil }

func newTestService() (*delivery.Service, *memBookings) {
	repo := newMemBookings()
	svc := delivery.NewService(delivery.Deps{
		Bookings:    repo,
		Idempotency: newMemIdem(),
		Wallet:      okWallet{},
		Publisher:   okPublisher{},
	})
	return svc, repo
}

func newRouterWithService(t *testing.T, svc *delivery.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPartnerRoutes(r, Dependencies{KeyRepo: newTestRepo(t), Deliveries: svc, Pricing: testPricingConfig})
	return r
}

// --- handler tests ----------------------------------------------------------

func TestHandlerQuoteReturns200(t *testing.T) {
	svc, _ := newTestService()
	r := newRouterWithService(t, svc)
	body := `{"pickup":{"lat":6.5244,"lng":3.3792},"dropoff":{"lat":6.6018,"lng":3.3515},"tier":"express","size":"small"}`
	w := requestBody(r, http.MethodPost, "/api/v1/partner/v1/quotes", body, true)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			DistanceKM float64 `json:"distance_km"`
			Currency   string  `json:"currency"`
			AmountKobo int64   `json:"amount_kobo"`
			Tier       string  `json:"tier"`
			Size       string  `json:"size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !env.Success || env.Data.AmountKobo <= 0 || env.Data.Currency != "NGN" ||
		env.Data.Tier != "express" || env.Data.Size != "small" || env.Data.DistanceKM <= 0 {
		t.Fatalf("unexpected quote response: %s", w.Body.String())
	}
}

func TestHandlerCreateDeliveryReturns201(t *testing.T) {
	svc, _ := newTestService()
	r := newRouterWithService(t, svc)
	body := `{"pickup_address":"A","pickup_lat":6.4474,"pickup_lng":3.4343,"dest_address":"B","dest_lat":6.4969,"dest_lng":3.3481,"receiver_name":"Chidi","receiver_phone":"+2348011223344","package_size":"small","service_tier":"express","payment_method":"wallet","customer_id":"cust-1","customer_email":"c@x.com","idempotency_key":"k-1"}`
	w := requestBody(r, http.MethodPost, "/api/v1/partner/v1/deliveries", body, true)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", w.Code, w.Body.String())
	}
}

func TestHandlerGetUnknownReturns404(t *testing.T) {
	svc, _ := newTestService()
	r := newRouterWithService(t, svc)
	w := requestBody(r, http.MethodGet, "/api/v1/partner/v1/deliveries/KGP-NONE0000", "", true)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", w.Code, w.Body.String())
	}
}

func TestHandlerCreateValidationReturns400(t *testing.T) {
	svc, _ := newTestService()
	r := newRouterWithService(t, svc)
	w := requestBody(r, http.MethodPost, "/api/v1/partner/v1/deliveries", `{}`, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}
