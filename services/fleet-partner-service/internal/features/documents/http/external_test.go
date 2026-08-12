package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
)

// TestRequireProviderCaller_StoresRawCallerNameNotMarketplaceAbbreviation
// pins the fix: fleet_partner_organization_provider_memberships.
// provider_service is populated at membership creation with the caller's own
// raw HMAC service name (platformclients.OwningService), e.g.
// "driver-dispatch-delivery-service" -- never the "DISPATCH"/"TAXI"/
// "HAULING" abbreviation the marketplace feature uses for its own,
// unrelated assignments/offers tables. Before this fix,
// requireProviderCaller stored the abbreviation, so every provider-facing
// document request would resolve a membership lookup against a value no
// real membership row ever has.
func TestRequireProviderCaller_StoresRawCallerNameNotMarketplaceAbbreviation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewExternalHandler(nil, []string{"driver-dispatch-delivery-service"}, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ServiceCallerKey, "driver-dispatch-delivery-service")
		c.Next()
	})
	router.GET("/x", handler.requireProviderCaller, func(c *gin.Context) {
		c.String(http.StatusOK, handler.providerService(c))
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}
	got := resp.Body.String()
	if got != "driver-dispatch-delivery-service" {
		t.Fatalf("providerService = %q, want the raw caller name %q (not a marketplace abbreviation like %q)",
			got, "driver-dispatch-delivery-service", "DISPATCH")
	}
}

// TestRequireProviderCaller_UnknownCallerRefused proves the allowlist check
// still gates access even though its values are no longer read.
func TestRequireProviderCaller_UnknownCallerRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewExternalHandler(nil, []string{"driver-dispatch-delivery-service"}, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ServiceCallerKey, "some-unrelated-service")
		c.Next()
	})
	router.GET("/x", handler.requireProviderCaller, func(c *gin.Context) {
		c.String(http.StatusOK, "unreachable")
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
	if resp.Code == http.StatusOK {
		t.Fatal("an unallowlisted caller must be refused before reaching the handler")
	}
}
