package app

import (
	"context"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
)

// StartSection16Workers runs the periodic projection-retention, freshness,
// actionable-assignment and route-progress sweep. Individual asynchronous
// operations are still executed by the shared durable jobs worker.
func StartSection16Workers(ctx context.Context, cfg config.Config, components *Components, workers *sync.WaitGroup) {
	if components == nil || components.DispatchControl == nil || workers == nil || !cfg.Section16WorkersEnabled || cfg.IntegrationsKillSwitch || cfg.PrivateOperationsKillSwitch {
		return
	}
	interval := time.Minute
	if cfg.JobPollInterval > interval {
		interval = cfg.JobPollInterval
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			_ = components.DispatchControl.RunMaintenance(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
