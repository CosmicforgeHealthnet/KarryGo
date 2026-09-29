package customertracking

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

// Customer V4 §24: TAXI_CUSTOMER_LIVE_LOCATION_ENABLED off refuses a valid,
// authenticated customer server-side, before any read of Ride state.
func TestLiveLocationFlagOffRefusesValidCustomer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer := auth.NewTokenSigner([]byte("secret"))
	token, err := signer.Sign(auth.Claims{Subject: "customer-1", Role: "customer", Service: "customer", SessionID: "s", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	e := gin.New()
	e.Use(httpx.ErrorHandler())
	RegisterCustomer(e.Group("/api/v1/taxi"), nil, signer, false)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/taxi/customer/bookings/ride-1/location", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("flag off: status=%d cache=%q", w.Code, w.Header().Get("Cache-Control"))
	}
}
