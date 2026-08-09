package app

import (
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	packagesrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/repositories"
	packagesusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/usecases"
	shipmentsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"github.com/jackc/pgx/v5/pgxpool"
)

func BuildPackagesService(cfg config.Config, pool *pgxpool.Pool, publisher *events.DomainPublisher, media platformclients.MediaContract) *packagesusecases.Service {
	return packagesusecases.NewService(packagesusecases.Options{Pool: pool, Repository: packagesrepos.NewRepository(pool), Shipments: NewPackagesShipmentsAdapter(shipmentsrepos.NewRepository(pool)), Media: NewPackagesMediaAdapter(media), Devices: NewPackagesDeviceAdapter(pool), ProviderScope: NewPackagesProviderScopeAdapter(pool), Events: publisher, Flags: packagesusecases.Flags{Enabled: cfg.PackagesEnabled, WritesEnabled: cfg.PackagesWritesEnabled, ScanningEnabled: cfg.PackageScanningEnabled, KillSwitch: cfg.PackagesKillSwitch || cfg.IntegrationsKillSwitch}})
}
