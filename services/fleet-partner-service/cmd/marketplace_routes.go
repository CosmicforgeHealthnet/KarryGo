package main

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	documenthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/http"
	financefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/finance"
	mkthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
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
	financeCallers := parseAllowedCallers(cfg.FinanceAllowedCallers)
	documentCallers := parseAllowedCallers(cfg.DocumentsAllowedCallers)
	documentsMountable := cfg.DocumentsEventIngestionEnabled && len(documentCallers) > 0
	providerDocCallers := parseAllowedCallers(cfg.DocumentsProviderAllowedCallers)
	customerDocCallers := parseAllowedCallers(cfg.DocumentsCustomerAllowedCallers)
	externalDocsMountable := len(providerDocCallers) > 0 || len(customerDocCallers) > 0
	if len(callers) == 0 && !documentsMountable && !externalDocsMountable &&
		(!cfg.FinanceEventIngestionEnabled || len(financeCallers) == 0) {
		// No allowlist means no caller could ever pass the per-route gate.
		// Mounting the routes anyway would only produce confusing 403s.
		logging.Notice(config.ServiceName,
			"marketplace internal routes not mounted: FLEET_MARKETPLACE_ALLOWED_CALLERS is empty")
		return nil
	}

	var handler *mkthttp.InternalHandler
	if len(callers) > 0 {
		handler = mkthttp.NewInternalHandler(newMarketplaceService(cfg, pool, metrics), callers)
	}

	return func(group *gin.RouterGroup) {
		if handler != nil {
			mkthttp.RegisterInternalMarketplaceRoutes(group, handler)
			mkthttp.RegisterProviderResponseRoutes(group, handler)
			// Section 28/34 provider self-service history: GET
			// /providers/:providerId/{offers,assignments}[/:id]. This
			// handler and its route registration have existed since the
			// marketplace feature was built, but were never mounted here --
			// confirmed by a repo-wide grep for the call site before this
			// change, finding none. The routes were reachable in source but
			// dead in every running deployment.
			mkthttp.RegisterProviderReadRoutes(group, handler)
		}
		if handler != nil && cfg.MarketplaceEventIngestionEnabled {
			mkthttp.RegisterEventIngestionRoutes(group, handler)
			logging.Success(config.ServiceName, "marketplace event ingestion mounted")
		} else {
			logging.Notice(config.ServiceName,
				"marketplace event ingestion disabled by FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED")
		}
		if cfg.FinanceEventIngestionEnabled {
			financeService := financefeature.NewService(pool, financefeature.NewRepository(pool), nil,
				events.NewDomainPublisher(events.NewOutboxRepository(pool)))
			financefeature.RegisterInternalRoutes(group, financeService, financeCallers)
		}
		if documentsMountable {
			documenthttp.RegisterInternalRoutes(group, documenthttp.NewInternalHandler(
				app.NewDocumentService(cfg, pool, metrics), documentCallers))
			logging.Success(config.ServiceName, "document media event ingestion mounted")
		} else {
			logging.Notice(config.ServiceName,
				"document media event ingestion disabled by FLEET_DOCUMENTS_EVENT_INGESTION_ENABLED")
		}

		if externalDocsMountable {
			documenthttp.RegisterExternalRoutes(group, documenthttp.NewExternalHandler(
				app.NewDocumentService(cfg, pool, metrics), providerDocCallers, customerDocCallers))
			logging.Success(config.ServiceName, "provider/customer document access mounted")
		} else {
			logging.Notice(config.ServiceName,
				"provider/customer document access disabled: no allowed callers configured")
		}
	}
}

// newMarketplaceService builds the marketplace use-case service.
//
// Shared by the HTTP surface and the reconciliation worker so both hold the
// same repository set. Building it twice invited exactly the defect this
// replaced: the Trips repository was absent from the only construction site,
// leaving Options.Trips nil in every deployment. It survived because the one
// production caller — OpenReconciliation — takes its querier as an argument
// and so never dereferenced the nil receiver. MarkStaleTrips, which uses the
// repository's own pool, would have panicked the moment a sweep ran.
func newMarketplaceService(cfg config.Config, pool *pgxpool.Pool,
	metrics *observability.Metrics) *mktusecases.Service {
	return app.BuildMarketplaceService(cfg, pool, metrics)
}

// marketplaceFlags maps configuration onto the service's write-path gates.
//
// Every gate defaults to off, and only DISPATCH can be switched on. Taxi and
// hauling have no live execution contract, so enabling them would mean fleet
// calling a service that cannot answer — the map is the single place that
// decides, and it names only the category that exists.
func marketplaceFlags(cfg config.Config) mktusecases.Flags {
	capabilities := cfg.ExecutionCapabilities()
	return mktusecases.Flags{
		ExecutionStartEnabled: capabilities.Any(),
		EnabledSourceServices: map[string]bool{
			mktmodels.SourceDispatch: capabilities.Dispatch,
			mktmodels.SourceTaxi:     false,
			mktmodels.SourceHauling:  capabilities.Hauling,
		},
	}
}

// startMarketplaceReconciliation runs projection reconciliation and the stale
// sweep until ctx ends.
//
// Disabled by default. When off, gapped projections stay GAP_DETECTED and
// commands against them keep refusing — degraded but safe — which is the
// correct posture for a deployment that has not configured a dispatch client
// to read back from.
// startMarketplaceReconciliation delegates to the shared internal/app
// implementation so both this binary and the dedicated worker binary run
// the identical reconciliation loop, built through the same composition
// root as the HTTP surface.
func startMarketplaceReconciliation(ctx context.Context, cfg config.Config,
	pool *pgxpool.Pool, metrics *observability.Metrics) {
	app.StartMarketplaceReconciliation(ctx, cfg, pool, metrics)
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
