package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	crmrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	ordersrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	catalogrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// BuildOrdersService assembles the Section 15 private order service.
//
// It reads through the Section 14 CRM repository (customers, credit,
// contracts, quotations and the conversion intent Section 14 created for
// this exact handoff) and the Section 15 service catalogue repository — both
// sibling feature packages of the same organization database, not another
// service. It is always constructible; with the enablement flag off its
// routes still mount but answer not-found.
//
// search may be nil (search wiring failed or was skipped); orders still
// writes correctly, it simply is not indexed.
func BuildOrdersService(cfg config.Config, pool *pgxpool.Pool, metrics *observability.Metrics,
	publisher *events.DomainPublisher, search *searchusecases.Service) *ordersusecases.Service {
	return ordersusecases.NewService(ordersusecases.Options{
		Pool:       pool,
		Repository: ordersrepos.NewRepository(pool),
		Catalog:    NewOrdersCatalogAdapter(catalogrepos.NewRepository(pool)),
		CRM:        NewOrdersCRMAdapter(crmrepos.NewRepository(pool)),
		Events:     publisher,
		Search:     NewOrdersSearchAdapter(search),
		Metrics:    metrics,
		Flags: ordersusecases.Flags{
			Enabled:       cfg.OrdersEnabled,
			WritesEnabled: cfg.OrdersWritesEnabled,
			KillSwitch:    cfg.OrdersKillSwitch || cfg.IntegrationsKillSwitch,
		},
	})
}
