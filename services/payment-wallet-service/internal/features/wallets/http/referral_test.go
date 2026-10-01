package wallethttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Only referral-service may move money through the referral routes; any
// other service holding an internal secret is refused.
func TestReferralRoutesAcceptOnlyReferralService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		caller  string
		allowed bool
	}{{"referral-service", true}, {"vendor-service", false}, {"driver-taxi-service", false}, {"", false}} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Set("service_name", test.caller)
		if got := requireReferralCaller(ctx); got != test.allowed {
			t.Fatalf("caller %q allowed=%v", test.caller, got)
		}
		if !test.allowed && !ctx.IsAborted() {
			t.Fatalf("caller %q was not stopped", test.caller)
		}
	}
}

// The referral cash-out route only answers a driver's own login.
func TestReferralWithdrawalRouteNeedsADriverLogin(t *testing.T) {
	customerSecret, taxiSecret := []byte("customer-secret"), []byte("taxi-secret")
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	RegisterRoutes(router.Group("/api/v1/payment-wallet"), nil, customerSecret, map[string][]byte{"taxi": taxiSecret}, serviceauth.Secrets{})
	for name, token := range map[string]string{
		"no login":       "",
		"customer login": providerToken(t, customerSecret, "cust-1", "customer", "customer"),
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/payment-wallet/provider/referral-withdrawals", strings.NewReader(`{}`))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want the request refused", name, rec.Code)
		}
	}
}
