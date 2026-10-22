package bookinghttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/customerstanding/standingtest"
	"cosmicforge/logistics/shared/go/httpx"
)

// TestCreateBookingRouteRefusesHeldCustomer drives the customer create
// handler behind the real bearer middleware. The service has NO repository
// or availability store: a refusal that reached either would panic, which
// proves the standing gate runs before any side effect. The held identity is
// the token subject; a body customer_id/eligibility claim is ignored.
func TestCreateBookingRouteRefusesHeldCustomer(t *testing.T) {
	srv := standingtest.New(t, "driver-hauling-service")
	subject := uuid.NewString()
	srv.Hold(subject, "BOOKING_HOLD", 3)
	svc := bookingusecases.NewBookingService(bookingusecases.Options{CustomerStanding: srv.Gate(300 * time.Millisecond)})
	signer := sharedauth.NewTokenSigner([]byte("hauling-customer-test-secret"))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	group := engine.Group("/api/v1/hauling/customer")
	group.Use(sharedauth.BearerMiddleware(signer, "customer", "customer"))
	group.POST("/bookings", NewBookingHandler(svc, nil, nil).CreateBooking)

	post := func() (int, string) {
		t.Helper()
		token, err := signer.Sign(sharedauth.Claims{Subject: subject, Role: "customer", Service: "customer",
			SessionID: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour).Unix()})
		if err != nil {
			t.Fatal(err)
		}
		body := `{"pickup_address":"A","pickup_lat":6.5,"pickup_lng":3.3,"dropoff_address":"B","dropoff_lat":6.6,"dropoff_lng":3.4,
			"cargo_weight_kg":100,"payment_method":"wallet","customer_id":"` + uuid.NewString() + `","booking_eligible":true}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/hauling/customer/bookings", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		var out struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out.Error.Code
	}
	if status, code := post(); status != http.StatusForbidden || code != "customer_booking_hold" {
		t.Fatalf("held: %d %s", status, code)
	}
	srv.FailWith(http.StatusServiceUnavailable)
	if status, code := post(); status != http.StatusServiceUnavailable || code != "customer_standing_unavailable" {
		t.Fatalf("outage: %d %s", status, code)
	}
}
