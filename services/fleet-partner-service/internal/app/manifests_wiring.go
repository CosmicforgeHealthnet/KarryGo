package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	manifestsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/repositories"
	manifestsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/usecases"
	ordersrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	packagesrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/repositories"
	shipmentsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
)

type manifestResourcesAdapter struct {
	packages  *packagesrepos.Repository
	shipments *shipmentsrepos.Repository
	orders    *ordersrepos.Repository
}

func (a manifestResourcesAdapter) Resolve(ctx context.Context, q database.Querier, org, kind, id string) (manifestsusecases.ResourceInfo, error) {
	switch kind {
	case "PACKAGE":
		p, err := a.packages.Get(ctx, q, org, id)
		return manifestsusecases.ResourceInfo{ID: p.ID, ShipmentID: p.ShipmentID, BranchID: p.BranchID}, err
	case "SHIPMENT":
		s, err := a.shipments.Get(ctx, q, org, id)
		return manifestsusecases.ResourceInfo{ID: s.ID, ShipmentID: s.ID, BranchID: s.BranchID}, err
	case "ORDER_STOP":
		s, err := a.orders.GetStop(ctx, q, org, id)
		if errors.Is(err, ordersrepos.ErrNotFound) {
			return manifestsusecases.ResourceInfo{}, err
		}
		return manifestsusecases.ResourceInfo{ID: s.ID}, err
	}
	return manifestsusecases.ResourceInfo{}, errors.New("unsupported resource")
}

func BuildManifestsService(cfg config.Config, pool *pgxpool.Pool, publisher *events.DomainPublisher) *manifestsusecases.Service {
	return manifestsusecases.NewService(manifestsusecases.Options{Pool: pool, Repository: manifestsrepos.NewRepository(pool), Resources: manifestResourcesAdapter{packagesrepos.NewRepository(pool), shipmentsrepos.NewRepository(pool), ordersrepos.NewRepository(pool)}, Events: publisher, Flags: manifestsusecases.Flags{Enabled: cfg.ManifestsEnabled, WritesEnabled: cfg.ManifestsWritesEnabled, KillSwitch: cfg.ManifestsKillSwitch || cfg.IntegrationsKillSwitch}})
}
