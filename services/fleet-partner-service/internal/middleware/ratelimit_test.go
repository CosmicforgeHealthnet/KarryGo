package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func ratelimitTestRouter(limiter ratelimit.Limiter, rule ratelimit.Rule, withTenant tenant.Context, withCaller string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// httpx.Abort only queues the error (c.Error + c.Abort); respond.
	// ErrorHandler is what actually converts it into a written status code
	// and body -- BuildRouter always installs it in production. Without it
	// here, an aborted request would leave httptest.ResponseRecorder at its
	// zero-value 200, silently hiding a real 429.
	router.Use(respond.ErrorHandler())
	router.Use(func(c *gin.Context) {
		if withTenant.OrganizationID != "" {
			c.Set(TenantContextKey, withTenant)
		}
		if withCaller != "" {
			c.Set(ServiceCallerKey, withCaller)
		}
		c.Next()
	})
	router.GET("/x", RateLimit(limiter, nil, ratelimit.RiskHighRisk, rule), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}

func TestRateLimit_AllowsUnderLimitDeniesOverLimitWithRetryAfter(t *testing.T) {
	limiter := ratelimit.NewMemoryLimiter()
	router := ratelimitTestRouter(limiter, ratelimit.Rule{Limit: 2, Window: time.Minute},
		tenant.Context{}, "")

	for i := 0; i < 2; i++ {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, resp.Code)
		}
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd request: status = %d, want 429", resp.Code)
	}
	if resp.Header().Get("Retry-After") == "" {
		t.Fatal("429 response must carry Retry-After")
	}
}

func TestRateLimit_NilLimiterDisablesEnforcement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/x", RateLimit(nil, nil, ratelimit.RiskHighRisk, ratelimit.Rule{Limit: 1, Window: time.Minute}),
		func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 5; i++ {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("request %d with nil limiter: status = %d, want 200 (nil limiter must never enforce)", i+1, resp.Code)
		}
	}
}

// TestRateLimit_OrganizationAndUserAreIndependentDimensions proves the
// layered design: two different users under the SAME organization each get
// their own budget (the user key), but the organization as a whole is
// separately bounded too (the org key) -- exhausting one dimension refuses
// the request even if the other dimension has room.
func TestRateLimit_OrganizationAndUserAreIndependentDimensions(t *testing.T) {
	limiter := ratelimit.NewMemoryLimiter()
	rule := ratelimit.Rule{Limit: 3, Window: time.Minute}

	// User A and User B share organization "org_1". The organization-level
	// budget (3) is smaller than what two users making 2 requests each (4
	// total) would need, so the org key must trip first.
	routerFor := func(userID string) *gin.Engine {
		return ratelimitTestRouter(limiter, rule, tenant.Context{OrganizationID: "org_1", OrganizationUserID: userID}, "")
	}

	hit := func(router *gin.Engine) int {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
		return resp.Code
	}

	userA := routerFor("user_a")
	userB := routerFor("user_b")

	codes := []int{hit(userA), hit(userB), hit(userA)}
	for i, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("request %d = %d, want 200 (org budget not yet exhausted)", i+1, code)
		}
	}
	// The 4th request against the shared organization, regardless of which
	// user makes it, exceeds the org-level budget of 3.
	if code := hit(userB); code != http.StatusTooManyRequests {
		t.Fatalf("4th request (org budget exhausted) = %d, want 429", code)
	}
}

func TestRateLimitKeys_IncludesEveryDimensionPresentPlusIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	c.Set(TenantContextKey, tenant.Context{OrganizationID: "org_1", OrganizationUserID: "ou_1"})
	c.Set(ServiceCallerKey, "driver-hauling-service")

	keys := rateLimitKeys(c)
	seen := map[string]bool{}
	for _, k := range keys {
		seen[k] = true
	}
	for _, want := range []string{"org:org_1", "user:ou_1", "svc:driver-hauling-service"} {
		if !seen[want] {
			t.Errorf("missing expected key %q in %v", want, keys)
		}
	}
	hasIP := false
	for _, k := range keys {
		if len(k) >= 3 && k[:3] == "ip:" {
			hasIP = true
		}
	}
	if !hasIP {
		t.Errorf("keys must always include an ip: floor, got %v", keys)
	}
}

// globalRateLimitTestRouter mirrors ratelimitTestRouter but wires
// GlobalRateLimit (registry-driven) instead of RateLimit (fixed class), and
// registers a real path so c.FullPath() resolves to something
// riskregistry.Lookup can actually match.
func globalRateLimitTestRouter(limiter ratelimit.Limiter, method, path string, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(respond.ErrorHandler())
	router.Handle(method, path, GlobalRateLimit(limiter, nil), handler)
	return router
}

// TestGlobalRateLimit_UsesRegistryClassification proves the registry-driven
// path end to end: a real, already-classified route
// (GET /fleet-partner/v1/legal-holds — riskregistry classifies a GET on a
// high-risk-named resource as SENSITIVE, reserving HIGH_RISK for the
// mutating create/release routes on the same resource) is actually bounded
// at that class's limit, resolved purely from c.FullPath()+method, with no
// risk class passed at route-registration time.
func TestGlobalRateLimit_UsesRegistryClassification(t *testing.T) {
	original := ratelimit.DefaultRiskRules[ratelimit.RiskSensitive]
	ratelimit.DefaultRiskRules[ratelimit.RiskSensitive] = ratelimit.Rule{Limit: 2, Window: time.Minute}
	defer func() { ratelimit.DefaultRiskRules[ratelimit.RiskSensitive] = original }()

	limiter := ratelimit.NewMemoryLimiter()
	router := globalRateLimitTestRouter(limiter, http.MethodGet, "/fleet-partner/v1/legal-holds",
		func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 2; i++ {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/legal-holds", nil))
		if resp.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, resp.Code)
		}
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/legal-holds", nil))
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd request against a SENSITIVE-classified route: status = %d, want 429", resp.Code)
	}
}

// TestGlobalRateLimit_DifferentRiskClassesGetDifferentBudgets proves two
// real registry-classified routes (GET /legal-holds is SENSITIVE, GET
// /orders is STANDARD) enforce independently — exhausting the stricter
// SENSITIVE route's budget does not touch the STANDARD route's own,
// separate budget, because they are keyed by the class-specific scope
// ("risk:SENSITIVE" vs "risk:STANDARD"), not a single shared counter.
func TestGlobalRateLimit_DifferentRiskClassesGetDifferentBudgets(t *testing.T) {
	originalSensitive := ratelimit.DefaultRiskRules[ratelimit.RiskSensitive]
	originalStandard := ratelimit.DefaultRiskRules[ratelimit.RiskStandard]
	ratelimit.DefaultRiskRules[ratelimit.RiskSensitive] = ratelimit.Rule{Limit: 1, Window: time.Minute}
	ratelimit.DefaultRiskRules[ratelimit.RiskStandard] = ratelimit.Rule{Limit: 1, Window: time.Minute}
	defer func() {
		ratelimit.DefaultRiskRules[ratelimit.RiskSensitive] = originalSensitive
		ratelimit.DefaultRiskRules[ratelimit.RiskStandard] = originalStandard
	}()

	limiter := ratelimit.NewMemoryLimiter()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(respond.ErrorHandler())
	router.GET("/fleet-partner/v1/legal-holds", GlobalRateLimit(limiter, nil), func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/fleet-partner/v1/orders", GlobalRateLimit(limiter, nil), func(c *gin.Context) { c.Status(http.StatusOK) })

	// Exhaust the HIGH_RISK route's budget of 1.
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/legal-holds", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("legal-holds 1st request = %d, want 200", first.Code)
	}
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/legal-holds", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("legal-holds 2nd request = %d, want 429", second.Code)
	}

	// The STANDARD-classified /orders route is unaffected -- its own budget
	// of 1 is still fresh.
	third := httptest.NewRecorder()
	router.ServeHTTP(third, httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/orders", nil))
	if third.Code != http.StatusOK {
		t.Fatalf("orders request after legal-holds exhausted = %d, want 200 (independent budgets)", third.Code)
	}
}

// TestGlobalRateLimit_UnclassifiedRouteFallsBackToStandardNotUnbounded
// proves a route absent from riskregistry (which
// TestRouteRiskRegistry_CoversEveryMountedRoute in section28contract
// guarantees cannot happen for any real mounted route, but this proves the
// runtime fallback itself, independent of that guarantee) is still bounded
// rather than silently unlimited.
func TestGlobalRateLimit_UnclassifiedRouteFallsBackToStandardNotUnbounded(t *testing.T) {
	original := ratelimit.DefaultRiskRules[ratelimit.RiskStandard]
	ratelimit.DefaultRiskRules[ratelimit.RiskStandard] = ratelimit.Rule{Limit: 1, Window: time.Minute}
	defer func() { ratelimit.DefaultRiskRules[ratelimit.RiskStandard] = original }()

	limiter := ratelimit.NewMemoryLimiter()
	router := globalRateLimitTestRouter(limiter, http.MethodGet, "/fleet-partner/v1/this-route-does-not-exist-in-the-registry",
		func(c *gin.Context) { c.Status(http.StatusOK) })

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/this-route-does-not-exist-in-the-registry", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("1st request to unclassified route = %d, want 200", first.Code)
	}
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/this-route-does-not-exist-in-the-registry", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("2nd request to unclassified route = %d, want 429 (fallback must still bound it)", second.Code)
	}
}

// TestClientIP_UntrustedProxyIgnoresForwardedForHeader proves the actual
// HTTP-layer mechanism app.BuildRouter relies on: with no trusted proxies
// configured (this service's default -- config.Config.TrustedProxies
// unset), gin's ClientIP() ignores an X-Forwarded-For header entirely and
// uses the raw connection's RemoteAddr, so a caller cannot pick its own
// rate-limit bucket by spoofing that header.
func TestClientIP_UntrustedProxyIgnoresForwardedForHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	_ = router.SetTrustedProxies(nil) // this service's default -- see config.Config.TrustedProxies
	var observedIP string
	router.GET("/x", func(c *gin.Context) { observedIP = c.ClientIP() })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "203.0.113.9:5555"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	router.ServeHTTP(httptest.NewRecorder(), req)

	if observedIP != "203.0.113.9" {
		t.Fatalf("ClientIP() with no trusted proxies = %q, want the raw RemoteAddr %q (X-Forwarded-For must be ignored)",
			observedIP, "203.0.113.9")
	}
}

// TestClientIP_TrustedProxyHonorsForwardedForHeader proves the other half:
// once an operator explicitly names a proxy as trusted (SetTrustedProxies
// with that proxy's CIDR), ClientIP() DOES honor X-Forwarded-For from a
// connection originating at that trusted proxy -- the intended behavior
// for a real deployment sitting behind a load balancer.
func TestClientIP_TrustedProxyHonorsForwardedForHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	_ = router.SetTrustedProxies([]string{"203.0.113.0/24"})
	var observedIP string
	router.GET("/x", func(c *gin.Context) { observedIP = c.ClientIP() })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "203.0.113.9:5555" // inside the trusted CIDR
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	router.ServeHTTP(httptest.NewRecorder(), req)

	if observedIP != "198.51.100.7" {
		t.Fatalf("ClientIP() from a trusted proxy = %q, want the forwarded value %q", observedIP, "198.51.100.7")
	}
}
