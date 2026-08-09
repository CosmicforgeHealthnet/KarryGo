package app

import (
	"context"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
)

// StartSection17And18Workers schedules periodic work through the shared durable
// queue; claims, SKIP LOCKED, leases, heartbeat, retry, dead-letter and replay
// remain owned by the existing jobs.Worker.
func StartSection17And18Workers(ctx context.Context, cfg config.Config, c *Components, wg *sync.WaitGroup) {
	if c == nil || wg == nil || cfg.IntegrationsKillSwitch {
		return
	}
	if cfg.Section17WorkersEnabled && c.Workforce != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tick := time.NewTicker(time.Minute)
			defer tick.Stop()
			for {
				_ = c.Workforce.ScheduleMaintenance(ctx)
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
			}
		}()
	}
	if cfg.Section18WorkersEnabled && c.Assets != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tick := time.NewTicker(time.Minute)
			defer tick.Stop()
			for {
				_ = c.Assets.ScheduleMaintenance(ctx)
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
			}
		}()
	}
}
