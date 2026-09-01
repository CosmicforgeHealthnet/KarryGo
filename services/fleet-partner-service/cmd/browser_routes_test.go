package main

import (
	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	authhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/http"
	providerhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"github.com/gin-gonic/gin"
	"testing"
)

func TestBrowserAndProviderRoutesConstructWithoutWildcardConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := authhttp.NewHandler(nil, nil)
	r := app.BuildRouter(app.Options{Config: config.Config{AppEnv: "test"}, Metrics: observability.NewMetrics(), PublicAuthRoutes: handler.RegisterPublicRoutes, IdentityRoutes: handler.RegisterAuthenticatedRoutes})
	internal := r.Group("/internal/fleet-partner/v1")
	internal.GET("/providers/:providerId", func(c *gin.Context) { c.Status(204) })
	providerhttp.RegisterInternalRoutes(internal, providerhttp.NewInternalHandler(nil, []string{"driver-dispatch-delivery-service"}))
	found := false
	for _, route := range r.Routes() {
		if route.Path == "/internal/fleet-partner/v1/providers/:providerId/services/:providerService/membership-summary" {
			found = true
		}
	}
	if !found {
		t.Fatal("membership route not registered")
	}
}
