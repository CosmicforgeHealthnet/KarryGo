package main

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	providershttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/http"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
)

// buildProviderMembershipInternalRoutes mounts Section 30's provider
// membership-summary surface (providerMode, tenant-safe organization
// summary, assignment/field-visibility policy for one provider) --
// reachable only by an HMAC-authenticated caller on the configured
// allowlist, the same "existing internal/platform-operator authorization
// mechanism" every other internal surface in this file uses.
//
// providers is the ALREADY fully-wired providers usecases.Service (built
// once in internal/app/wiring.go with every cross-feature dependency it
// needs, e.g. OrganizationReader); this file does not construct a second,
// competing instance.
//
// Returns nil when no caller is allowlisted, matching every other internal
// route builder in this file (buildWorkerOperationsInternalRoutes,
// buildMarketplaceInternalRoutes) -- a nil registrar leaves the route
// absent entirely rather than present-and-failing.
func buildProviderMembershipInternalRoutes(cfg config.Config, providers *providerusecases.Service) app.RouteRegistrar {
	callers := parseAllowedCallers(cfg.ProviderMembershipAllowedCallers)
	if len(callers) == 0 {
		logging.Notice(config.ServiceName,
			"provider membership-summary internal route not mounted: PROVIDER_MEMBERSHIP_ALLOWED_CALLERS is empty")
		return nil
	}
	handler := providershttp.NewInternalHandler(providers, callers)
	return func(group *gin.RouterGroup) {
		providershttp.RegisterInternalRoutes(group, handler)
		logging.Success(config.ServiceName, "provider membership-summary internal route mounted")
	}
}
