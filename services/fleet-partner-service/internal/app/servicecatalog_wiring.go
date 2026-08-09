package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	catalogrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/repositories"
	catalogusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// BuildServiceCatalogService assembles the Section 15 service catalogue
// service.
//
// It is always constructible. With the enablement flag off its routes still
// mount but answer not-found, matching the Section 14 CRM convention: an
// absent capability must be observable, not indistinguishable from a
// deployment that never had it.
func BuildServiceCatalogService(cfg config.Config, pool *pgxpool.Pool, metrics *observability.Metrics,
	publisher *events.DomainPublisher) *catalogusecases.Service {
	return catalogusecases.NewService(catalogusecases.Options{
		Pool:       pool,
		Repository: catalogrepos.NewRepository(pool),
		Events:     publisher,
		Metrics:    metrics,
		Flags: catalogusecases.Flags{
			Enabled:       cfg.ServiceCatalogEnabled,
			WritesEnabled: cfg.ServiceCatalogWritesEnabled,
			KillSwitch:    cfg.IntegrationsKillSwitch,
		},
	})
}
