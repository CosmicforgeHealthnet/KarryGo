package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	ordersrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	shipmentsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// BuildShipmentsService assembles the Section 15 shipment and cargo item
// service.
//
// It is always constructible. With the enablement flag off its routes still
// mount but answer not-found, matching the Section 14/15 convention: an
// absent capability must be observable, not indistinguishable from a
// deployment that never had it.
func BuildShipmentsService(cfg config.Config, pool *pgxpool.Pool, metrics *observability.Metrics,
	publisher *events.DomainPublisher) *shipmentsusecases.Service {
	return shipmentsusecases.NewService(shipmentsusecases.Options{
		Pool:       pool,
		Repository: shipmentsrepos.NewRepository(pool),
		Orders:     NewShipmentsOrdersAdapter(ordersrepos.NewRepository(pool)),
		Events:     publisher,
		Metrics:    metrics,
		Flags: shipmentsusecases.Flags{
			Enabled:       cfg.ShipmentsEnabled,
			WritesEnabled: cfg.ShipmentsWritesEnabled,
			KillSwitch:    cfg.ShipmentsKillSwitch || cfg.IntegrationsKillSwitch,
		},
	})
}
