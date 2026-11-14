package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/riskregistry"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// TestBuildRouterMountsStepUpOnTenantRoutes proves the production router
// enforces the step-up registry on every tenant route a feature registers,
// and that leaving the verifier out fails closed rather than open.
func TestBuildRouterMountsStepUpOnTenantRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := BuildRouter(Options{
		Config:      testConfig(),
		Metrics:     observability.NewMetrics(),
		TokenSigner: auth.NewTokenSigner(testAccessSecret),
		Verifier:    serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": testServiceSecret}, 5*time.Minute),
		Resolver: staticResolver{membership: tenant.Membership{
			OrganizationID: "org_11111111111111111111111111111111", OrganizationUserID: "usr_1",
		}},
		TenantRoutes: func(group *gin.RouterGroup) {
			ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }
			group.POST("/roles", ok)
			group.GET("/roles", ok)
		},
	})
	call := func(method string) int {
		request := httptest.NewRequest(method, "/fleet-partner/v1/roles", nil)
		request.Header.Set("Authorization", "Bearer "+corporateToken(t))
		if method == http.MethodPost {
			request.Header.Set("Content-Type", "application/json")
		}
		request.Header.Set(stepup.HeaderReference, "chl_any")
		return do(router, request).Code
	}
	if code := call(http.MethodPost); code != http.StatusServiceUnavailable {
		t.Fatalf("protected tenant route without a configured verifier = %d, want 503", code)
	}
	if code := call(http.MethodGet); code != http.StatusNoContent {
		t.Fatalf("unprotected tenant route = %d, want 204", code)
	}
}

// TestStepUpRegistryRoutesAreHighRisk keeps the step-up registry honest: every
// protected route must be a real, classified route, and a route that demands
// step-up is by definition high risk.
func TestStepUpRegistryRoutesAreHighRisk(t *testing.T) {
	routes := stepup.Routes()
	if len(routes) == 0 {
		t.Fatal("step-up registry is empty")
	}
	for _, route := range routes {
		class, ok := riskregistry.Lookup(route.Method, route.Path)
		if !ok {
			t.Errorf("%s %s requires step-up but is not in the risk registry (typo or removed route?)", route.Method, route.Path)
			continue
		}
		if class != ratelimit.RiskHighRisk {
			t.Errorf("%s %s requires step-up but is classified %s, want %s", route.Method, route.Path, class, ratelimit.RiskHighRisk)
		}
		if route.ActionClass == "" {
			t.Errorf("%s %s has no action class", route.Method, route.Path)
		}
	}
}
