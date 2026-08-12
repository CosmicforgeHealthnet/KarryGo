package app

import (
	"context"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	exportsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/usecases"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
)

// StartSection25Workers starts the durable local workers behind Section 25's
// asynchronous export framework: generation (claim → dataset read → CSV
// render → Media upload → READY) and expiry (READY past its retention
// window → EXPIRED). Both are no-ops when their flags are off, matching the
// established Section 15 convention.
func StartSection25Workers(ctx context.Context, cfg config.Config, components *Components, workers *sync.WaitGroup) {
	if components == nil || workers == nil || cfg.IntegrationsKillSwitch {
		return
	}
	interval := cfg.JobPollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	start := func(pass func(context.Context)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				pass(ctx)
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
	if cfg.ExportsWorkerEnabled && components.Exports != nil {
		start(func(runCtx context.Context) {
			_, _ = components.Exports.RunGenerationPass(runCtx, exportsusecases.WorkerConfig{
				Owner: config.ServiceName + "-export-generation", Lease: cfg.JobLeaseDuration,
				BatchSize: 10, MaxAttempts: 8,
			})
		})
		start(func(runCtx context.Context) {
			_, _ = components.Exports.RunExpiryPass(runCtx, 200)
		})
	}
	if cfg.SearchEnabled && components.Search != nil {
		start(func(runCtx context.Context) {
			_, _ = components.Search.RunIndexingPass(runCtx, searchusecases.IndexWorkerConfig{Owner: config.ServiceName + "-search-index", Lease: cfg.JobLeaseDuration, BatchSize: 25, MaxAttempts: 8})
		})
	}
}
