package app

import (
	"context"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

// RegisterSections19To21Workers binds all handlers to the one shared durable
// jobs worker. Registration is separate from scheduling so API and dedicated
// worker deployments have the same handler graph.
func RegisterSections19To21Workers(cfg config.Config, c *Components, worker *jobs.Worker) {
	if c == nil || worker == nil || cfg.IntegrationsKillSwitch {
		return
	}
	if cfg.Section19WorkersEnabled {
		if c.Compliance != nil {
			c.Compliance.RegisterWorkers(worker)
		}
		if c.SafetyOperations != nil {
			c.SafetyOperations.RegisterWorkers(worker)
		}
		if c.Proof != nil {
			c.Proof.RegisterWorkers(worker)
		}
	}
	if cfg.Section20WorkersEnabled && c.Specialized != nil {
		c.Specialized.RegisterWorkers(worker)
	}
	if cfg.Section21WorkersEnabled && c.Warehouse != nil {
		c.Warehouse.RegisterWorkers(worker)
	}
}

// StartSections19To21Workers schedules idempotent maintenance passes. Actual
// work, retries, heartbeats, dead-lettering and replay remain owned by jobs.Worker.
func StartSections19To21Workers(ctx context.Context, cfg config.Config, c *Components, wg *sync.WaitGroup) {
	if c == nil || wg == nil || cfg.IntegrationsKillSwitch ||
		(!cfg.Section19WorkersEnabled && !cfg.Section20WorkersEnabled && !cfg.Section21WorkersEnabled) {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		run := func() {
			if cfg.Section19WorkersEnabled {
				if c.Compliance != nil {
					_ = c.Compliance.ScheduleMaintenance(ctx)
				}
				if c.SafetyOperations != nil {
					_ = c.SafetyOperations.ScheduleMaintenance(ctx)
				}
				if c.Proof != nil {
					_ = c.Proof.ScheduleMaintenance(ctx)
				}
			}
			if cfg.Section20WorkersEnabled && c.Specialized != nil {
				_ = c.Specialized.ScheduleMaintenance(ctx)
			}
			if cfg.Section21WorkersEnabled && c.Warehouse != nil {
				_ = c.Warehouse.ScheduleMaintenance(ctx)
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
