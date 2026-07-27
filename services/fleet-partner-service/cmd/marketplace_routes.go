package main

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	mkthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// buildMarketplaceInternalRoutes mounts the service-to-service marketplace
// surface: acceptance verification, provider reads, provider responses and
// marketplace event ingestion.
//
// Returns nil when nothing should be mounted. A nil registrar leaves the route
// absent entirely rather than present-and-failing, so an unconfigured
// deployment answers 404 instead of accepting events it cannot apply.
func buildMarketplaceInternalRoutes(cfg config.Config, pool *pgxpool.Pool,
	metrics *observability.Metrics) app.RouteRegistrar {
	callers := parseAllowedCallers(cfg.MarketplaceAllowedCallers)
	if len(callers) == 0 {
		// No allowlist means no caller could ever pass the per-route gate.
		// Mounting the routes anyway would only produce confusing 403s.
		logging.Notice(config.ServiceName,
			"marketplace internal routes not mounted: FLEET_MARKETPLACE_ALLOWED_CALLERS is empty")
		return nil
	}

	service := mktusecases.NewService(mktusecases.Options{
		Pool:            pool,
		Projections:     mktrepos.NewProjectionRepository(pool),
		Offers:          mktrepos.NewOfferRepository(pool),
		Assignments:     mktrepos.NewAssignmentRepository(pool),
		ExecutionStarts: mktrepos.NewExecutionStartRepository(pool),
		Executions: platformclients.BuildExecutionSet(platformclients.Options{
			ServiceName: config.ServiceName,
			BaseURL:     cfg.DispatchServiceURL,
			Secret:      cfg.DispatchServiceSecret,
			Timeout:     cfg.DispatchServiceTimeout,
			Metrics:     metrics,
		}),
		Events:  events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Metrics: metrics,
	})
	handler := mkthttp.NewInternalHandler(service, callers)

	return func(group *gin.RouterGroup) {
		mkthttp.RegisterInternalMarketplaceRoutes(group, handler)
		mkthttp.RegisterProviderResponseRoutes(group, handler)
		if cfg.MarketplaceEventIngestionEnabled {
			mkthttp.RegisterEventIngestionRoutes(group, handler)
			logging.Success(config.ServiceName, "marketplace event ingestion mounted")
		} else {
			logging.Notice(config.ServiceName,
				"marketplace event ingestion disabled by FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED")
		}
	}
}

// parseAllowedCallers splits the configured caller allowlist.
func parseAllowedCallers(raw string) []string {
	var callers []string
	for _, part := range strings.Split(raw, ",") {
		if name := strings.TrimSpace(part); name != "" {
			callers = append(callers, name)
		}
	}
	return callers
}
