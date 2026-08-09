package app

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	walletusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// StartCompanyWalletWorkers starts the Section 13 execution and projection
// workers on the supplied wait group.
//
// A deployment that runs the dedicated worker process must get the same
// execution and reconciliation the in-process option provides; otherwise
// approved allocations would sit unexecuted the moment somebody split the
// deployment, and nothing would say so.
func StartCompanyWalletWorkers(ctx context.Context, cfg config.Config, pool *pgxpool.Pool,
	metrics *observability.Metrics, workers *sync.WaitGroup, service *walletusecases.Service) {
	if service == nil {
		return
	}
	if cfg.IntegrationsKillSwitch || cfg.CompanyWalletKillSwitch {
		logging.Notice(config.ServiceName+"-worker",
			"company wallet workers stopped by kill switch")
		return
	}

	if cfg.ProviderAllocationEnabled || cfg.ProviderWithdrawalEnabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_ = service.RunWorkers(ctx, walletusecases.WorkerConfig{
				Owner:       config.ServiceName + "-companywallet",
				BatchSize:   cfg.CompanyWalletWorkerBatchSize,
				Interval:    cfg.CompanyWalletWorkerInterval,
				MaxAttempts: cfg.CompanyWalletWorkerMaxAttempts,
			})
		}()
	}

	if cfg.WalletProjectionRefreshEnabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			interval := cfg.CompanyWalletWorkerInterval
			if interval <= 0 {
				interval = 30 * time.Second
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// A refresh failure is not fatal: the projection simply
					// stays not-current, and every sensitive gate keeps
					// refusing until it can be confirmed.
					_, _ = service.RefreshProjections(ctx, walletusecases.WorkerConfig{
						Owner: config.ServiceName + "-companywallet-refresh"},
						cfg.CompanyWalletWorkerBatchSize)
				}
			}
		}()
	}
}
