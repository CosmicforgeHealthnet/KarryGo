package wallethttp

// Section 14 VIS-013: Wallet's provider-facing surface must not reveal what the
// execution services' visibility policy withholds.
//
// The risk is specific. Taxi, Dispatch and Hauling project provider responses
// through FinancialVisibilityPolicy, so a managed provider never sees the
// organization's or the platform's share of a job. Wallet holds those same
// figures -- it is where they are posted -- and its provider routes are reached
// with a provider's own bearer token. A route that answered from the ledger
// without regard for the policy would hand a provider, directly, exactly what
// the three services take care not to show them.
//
// This audit enumerates the real router rather than a list someone maintains by
// hand, so a route added later is covered the day it is added.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// providerFacingRoutes returns every route a provider bearer token can reach.
// providerRouter builds the wallet router the way production does.
//
// The httpx error middleware matters: httpx.Abort records the error and aborts,
// and it is that middleware which turns the record into a 401. A bare engine
// answers 200 with an empty body, which would make an authentication assertion
// silently meaningless.
func providerRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.RequestID(), httpx.Recovery(), httpx.ErrorHandler())
	RegisterRoutes(engine.Group("/api/v1/payment-wallet"), nil,
		[]byte("customer-secret"),
		map[string][]byte{"hauling": []byte("provider-secret")},
		serviceauth.Secrets{"fleet-partner-service": []byte("fleet-secret")})
	return engine
}

// providerFacingRoutes returns every route reachable with a provider token.
//
// Matched on the provider group's own prefix, not on the word appearing
// anywhere: an internal HMAC route that happens to end in "/provider" is a
// service-to-service route and belongs to a different audit.
func providerFacingRoutes(t *testing.T) []gin.RouteInfo {
	t.Helper()
	const providerPrefix = "/api/v1/payment-wallet/provider/"
	var provider []gin.RouteInfo
	for _, route := range providerRouter(t).Routes() {
		if strings.HasPrefix(route.Path, providerPrefix) {
			provider = append(provider, route)
		}
	}
	return provider
}

// The managed settlement routes carry the organization and platform shares in
// their responses. They must be reachable only by an HMAC-signed service, never
// by a provider's own bearer token.
func TestManagedSettlementIsNotOnTheProviderSurface(t *testing.T) {
	for _, route := range providerFacingRoutes(t) {
		if strings.Contains(route.Path, "managed-settlement") ||
			strings.Contains(route.Path, "managed-jobs") ||
			strings.Contains(route.Path, "finance/v1") {
			t.Fatalf("a settlement route is reachable with a provider token: %s %s", route.Method, route.Path)
		}
	}
}

// Nor may the Commerce allocation surface be: an allocation carries the
// organization's delivery entitlement and the platform's revenue.
func TestCommerceAllocationsAreNotOnTheProviderSurface(t *testing.T) {
	for _, route := range providerFacingRoutes(t) {
		if strings.Contains(route.Path, "commerce") || strings.Contains(route.Path, "allocations") {
			t.Fatalf("a commerce allocation route is reachable with a provider token: %s %s",
				route.Method, route.Path)
		}
	}
}

// The provider surface is small and every route on it is about the provider's
// own money. Anything else appearing here is a new surface that has not been
// assessed, so the audit fails until someone does.
func TestTheProviderSurfaceIsTheAuditedSet(t *testing.T) {
	audited := map[string]string{
		"/api/v1/payment-wallet/provider/banks":                     "supported payout-bank names/codes/currency only; no account or financial data",
		"/api/v1/payment-wallet/provider/earnings":                  "the provider's own balances",
		"/api/v1/payment-wallet/provider/transactions":              "the provider's own available job-earnings ledger entries; no organization or platform accounts",
		"/api/v1/payment-wallet/provider/bank-accounts":             "the provider's own payout accounts",
		"/api/v1/payment-wallet/provider/bank-accounts/:id":         "the provider's own payout account",
		"/api/v1/payment-wallet/provider/bank-accounts/resolve":     "bank name lookup, no provider data",
		"/api/v1/payment-wallet/provider/bank-accounts/:id/default": "the provider's own default account",
		"/api/v1/payment-wallet/provider/withdrawals":               "the provider's own withdrawal",
		"/api/v1/payment-wallet/provider/pin/status":                "the provider's own PIN state",
		"/api/v1/payment-wallet/provider/pin":                       "the provider's own PIN",
		// Referral cash-out: pays out the provider's OWN referral earnings
		// balance. It carries no organization entitlement and no platform
		// revenue, so it belongs on this surface. Eligibility is checked with
		// referral-service before any payout.
		"/api/v1/payment-wallet/provider/referral-withdrawals": "the provider's own referral earnings",
	}
	for _, route := range providerFacingRoutes(t) {
		if _, known := audited[route.Path]; !known {
			t.Fatalf("unaudited provider route %s %s -- assess it against "+
				"FinancialVisibilityPolicy and add it here", route.Method, route.Path)
		}
	}
	if len(providerFacingRoutes(t)) == 0 {
		t.Fatal("no provider routes found; the audit is not looking at the real router")
	}
}

// Every provider route derives its subject from the bearer token. A route that
// took a provider id from the path or the body would let one provider read
// another's money by changing a parameter.
func TestNoProviderRouteTakesAProviderIdentityFromTheRequest(t *testing.T) {
	for _, route := range providerFacingRoutes(t) {
		lowered := strings.ToLower(route.Path)
		for _, parameter := range []string{":providerid", ":provider_id", ":membershipid", ":ownerid"} {
			if strings.Contains(lowered, parameter) {
				t.Fatalf("route %s %s takes the provider identity from the request",
					route.Method, route.Path)
			}
		}
	}
}

// An unauthenticated request must not reach any of it.
func TestTheProviderSurfaceRefusesAnUnauthenticatedRequest(t *testing.T) {
	engine := providerRouter(t)
	for _, route := range providerFacingRoutes(t) {
		if route.Method != http.MethodGet {
			continue
		}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(route.Method, route.Path, nil)
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s answered %d without a token, want 401",
				route.Method, route.Path, recorder.Code)
		}
	}
}
