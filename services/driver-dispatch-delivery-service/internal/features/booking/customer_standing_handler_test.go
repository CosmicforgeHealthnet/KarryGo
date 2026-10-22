package booking

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

// TestCustomerStandingCreateRouteContract drives the real customer route:
// the customer id is the bearer subject only (a body customer_id or
// eligibility claim is ignored), a hold is 403 customer_booking_hold and an
// outage is 503 customer_standing_unavailable.
func TestCustomerStandingCreateRouteContract(t *testing.T) {
	h := newStandingHarness(t)
	signer := sharedauth.NewTokenSigner([]byte("dispatch-customer-test-secret"))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	RegisterRoutes(engine, signer, NewHandler(h.service, nil))

	other := "22222222-2222-2222-2222-222222222222"
	h.srv.Hold(testCustomer, "BOOKING_HOLD", 9)
	post := func() (int, string) {
		t.Helper()
		token, err := signer.Sign(sharedauth.Claims{Subject: testCustomer, Role: "customer", Service: "customer",
			SessionID: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour).Unix()})
		if err != nil {
			t.Fatal(err)
		}
		in := validCreateInput()
		raw, _ := json.Marshal(in)
		body := strings.TrimSuffix(string(raw), "}") + `,"customer_id":"` + other + `","booking_eligible":true}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/dispatch/bookings", strings.NewReader(body))
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
	h.assertNoSideEffects(t)
	h.srv.FailWith(http.StatusInternalServerError)
	if status, code := post(); status != http.StatusServiceUnavailable || code != "customer_standing_unavailable" {
		t.Fatalf("outage: %d %s", status, code)
	}
	h.assertNoSideEffects(t)
	h.srv.FailWith(0)
	h.srv.Lift(testCustomer, 10)
	if status, _ := post(); status != http.StatusCreated {
		t.Fatalf("lifted: %d", status)
	}
	h.repo.mu.Lock()
	defer h.repo.mu.Unlock()
	for _, b := range h.repo.bookings {
		if b.CustomerID != testCustomer {
			t.Fatalf("booking attributed to %s, want token subject", b.CustomerID)
		}
	}
}
