package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/riskregistry"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func lifecycleRouter(status string, statusErr error) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(respond.ErrorHandler())
	group := router.Group("/fleet-partner/v1")
	group.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: "org_1", OrganizationUserID: "ou_1"})
		c.Next()
	})
	group.Use(middleware.RequireOperationalOrganization(func(context.Context, string) (string, error) {
		return status, statusErr
	}))
	ok := func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) }
	group.GET("/orders", ok)
	group.POST("/orders", ok)
	group.POST("/providers", ok)
	group.POST("/dispatch/plans", ok)
	group.PUT("/organizations/:organizationId/profile", ok)
	group.POST("/organizations/:organizationId/submit", ok)
	group.POST("/support/cases", ok)
	return router
}

func lifecycleCall(router *gin.Engine, method, path string) (int, string) {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader("{}")))
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder.Code, body.Error.Code
}

func TestOrganizationLifecycleGate(t *testing.T) {
	cases := []struct {
		status string
		code   string
	}{
		{"BUSINESS_SUBMITTED", "organization_verification_pending"},
		{"UNDER_REVIEW", "organization_verification_pending"},
		{"CHANGES_REQUIRED", "organization_changes_required"},
		{"DRAFT", "organization_onboarding_incomplete"},
		{"PENDING_REVIEW", "organization_onboarding_incomplete"},
		{"CONTACT_VERIFIED", "organization_onboarding_incomplete"},
		{"SUSPENDED", "organization_not_eligible"},
		{"CLOSED", "organization_not_eligible"},
		{"SOMETHING_NEW", "organization_not_eligible"},
	}
	for _, tc := range cases {
		router := lifecycleRouter(tc.status, nil)
		if code, _ := lifecycleCall(router, "GET", "/fleet-partner/v1/orders"); code != 200 {
			t.Fatalf("%s: reads must stay available, got %d", tc.status, code)
		}
		for _, path := range []string{"/fleet-partner/v1/orders", "/fleet-partner/v1/providers", "/fleet-partner/v1/dispatch/plans"} {
			code, errCode := lifecycleCall(router, "POST", path)
			if code != 403 || errCode != tc.code {
				t.Fatalf("%s POST %s: got %d %q, want 403 %q", tc.status, path, code, errCode, tc.code)
			}
		}
		for _, call := range [][2]string{{"PUT", "/fleet-partner/v1/organizations/org_1/profile"}, {"POST", "/fleet-partner/v1/organizations/org_1/submit"}, {"POST", "/fleet-partner/v1/support/cases"}} {
			if code, _ := lifecycleCall(router, call[0], call[1]); code != 200 {
				t.Fatalf("%s %s %s: pre-activation route refused with %d", tc.status, call[0], call[1], code)
			}
		}
	}
}

func TestOrganizationLifecycleGateAllowsActiveOrganizations(t *testing.T) {
	router := lifecycleRouter("ACTIVE", nil)
	for _, path := range []string{"/fleet-partner/v1/orders", "/fleet-partner/v1/providers", "/fleet-partner/v1/dispatch/plans"} {
		if code, _ := lifecycleCall(router, "POST", path); code != 200 {
			t.Fatalf("active organization POST %s refused: %d", path, code)
		}
	}
}

func TestOrganizationLifecycleGateFailsClosed(t *testing.T) {
	router := lifecycleRouter("", errors.New("database unavailable"))
	if code, _ := lifecycleCall(router, "POST", "/fleet-partner/v1/orders"); code != 503 {
		t.Fatalf("unreadable status must refuse writes, got %d", code)
	}
	if code, _ := lifecycleCall(router, "GET", "/fleet-partner/v1/orders"); code != 200 {
		t.Fatalf("reads must not depend on the status read, got %d", code)
	}
}

// Every pre-activation exception must name a real, classified route, so a
// typo cannot silently leave a route that was meant to be allowed refused —
// or a renamed route unexpectedly allowed.
func TestPreActivationWriteRoutesAreRealRoutes(t *testing.T) {
	for key := range middleware.PreActivationWriteRoutes {
		method, path, _ := strings.Cut(key, " ")
		if method == "GET" || method == "HEAD" {
			t.Fatalf("%s: reads never need an exception", key)
		}
		if _, ok := riskregistry.Lookup(method, path); !ok {
			t.Fatalf("%s is not a registered route", key)
		}
	}
}
