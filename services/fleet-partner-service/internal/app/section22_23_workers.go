package app

import (
	"context"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

// RegisterSections22And23Workers attaches handlers to the shared leased,
// retrying durable queue in both API-worker and dedicated-worker deployments.
func RegisterSections22And23Workers(cfg config.Config, c *Components, worker *jobs.Worker) {
	if c == nil || worker == nil || cfg.IntegrationsKillSwitch {
		return
	}
	if cfg.Section22WorkersEnabled && c.FinanceOperations != nil {
		c.FinanceOperations.RegisterWorkers(worker)
	}
	if cfg.Section23WorkersEnabled && c.Engagement != nil {
		c.Engagement.RegisterWorkers(worker)
	}
}

func StartSections22And23Workers(ctx context.Context, cfg config.Config, c *Components, wg *sync.WaitGroup) {
	if c == nil || wg == nil || cfg.IntegrationsKillSwitch || (!cfg.Section22WorkersEnabled && !cfg.Section23WorkersEnabled) {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		run := func() {
			if cfg.Section22WorkersEnabled && c.FinanceOperations != nil {
				_ = c.FinanceOperations.ScheduleMaintenance(ctx)
			}
			if cfg.Section23WorkersEnabled && c.Engagement != nil {
				_ = c.Engagement.ScheduleMaintenance(ctx)
			}
		}
		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
