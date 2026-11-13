package app

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/scheduler"
)

// Sections 11-12 / 27 background work that both binaries must run identically.
//
// Each Start* function here is called from cmd/main.go (in-process, when
// FLEET_PARTNER_WORKER_ENABLED) and from cmd/worker/main.go (the dedicated
// worker process). Running them in both places is safe by construction: every
// claim is a row lock with SKIP LOCKED or a lease, and every state change a
// compare-and-set, so two processes never apply the same change twice.

// StartMarketplaceLifecycleWorker runs offer expiry (spec 62 "offer-expiry")
// and the trip -> assignment synchronization sweep on one ticker.
//
// Both halves are local-only: they read and write Fleet-owned rows and make no
// external call, so the worker is on by default
// (FLEET_MARKETPLACE_LIFECYCLE_WORKER_ENABLED). Each pass first sweeps once
// immediately so work left behind by a crashed process is recovered at
// startup rather than after a full interval.
func StartMarketplaceLifecycleWorker(ctx context.Context, cfg config.Config, pool *pgxpool.Pool,
	metrics *observability.Metrics, wg *sync.WaitGroup) {
	if !cfg.MarketplaceLifecycleWorkerEnabled {
		logging.Notice(config.ServiceName,
			"marketplace lifecycle worker disabled by FLEET_MARKETPLACE_LIFECYCLE_WORKER_ENABLED")
		return
	}
	service := BuildMarketplaceService(cfg, pool, metrics)
	interval := cfg.MarketplaceLifecycleInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	batch := cfg.MarketplaceLifecycleBatchSize
	if batch <= 0 {
		batch = 100
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		logging.Success(config.ServiceName, "marketplace lifecycle worker started (offer expiry, trip->assignment sync)")
		runMarketplaceLifecyclePass(ctx, service, batch)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runMarketplaceLifecyclePass(ctx, service, batch)
			}
		}
	}()
}

// RunMarketplaceLifecyclePass performs one offer-expiry and trip-sync pass.
// Exported for the worker contract tests.
func RunMarketplaceLifecyclePass(ctx context.Context, service *mktusecases.Service, batch int) (int, int) {
	return runMarketplaceLifecyclePass(ctx, service, batch)
}

func runMarketplaceLifecyclePass(ctx context.Context, service *mktusecases.Service, batch int) (int, int) {
	expired, err := service.ExpireOffers(ctx, batch)
	if err != nil && ctx.Err() == nil {
		logging.Notice(config.ServiceName, "offer expiry pass error: %v", err)
	}
	synced, err := service.SyncAssignmentsFromTrips(ctx, batch)
	if err != nil && ctx.Err() == nil {
		logging.Notice(config.ServiceName, "trip assignment sync pass error: %v", err)
	}
	return expired, synced
}

// StartExecutionStartWorker turns a provider's durable acceptance into the
// canonical trip in the owning execution service.
//
// Built through BuildMarketplaceService so its execution set is the SAME one
// the HTTP surface uses -- previously the dedicated worker assembled its own
// set and never included Taxi, so a Taxi acceptance could never start. Nothing
// starts when no execution source is enabled and configured: a worker that can
// only fail would burn every operation's retry budget. Work abandoned by a
// previous process is recovered at the top of every pass (expired leases).
func StartExecutionStartWorker(ctx context.Context, cfg config.Config, pool *pgxpool.Pool,
	metrics *observability.Metrics, wg *sync.WaitGroup) {
	capabilities := cfg.ExecutionCapabilities()
	if !capabilities.Any() || cfg.IntegrationsKillSwitch {
		logging.Notice(config.ServiceName, "execution-start worker disabled: no execution source is enabled")
		return
	}
	service := BuildMarketplaceService(cfg, pool, metrics)
	workerConfig := ExecutionStartWorkerConfig(cfg)
	interval := cfg.ExecutionStartInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		logging.Success(config.ServiceName, "execution-start worker running (sources=%v)", workerConfig.EnabledSources)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := service.RunExecutionStart(ctx, workerConfig); err != nil && ctx.Err() == nil {
					// A failed pass must not kill the worker: the operations
					// are durable and the next tick retries them.
					logging.Notice(config.ServiceName, "execution-start pass failed: %v", err)
				}
			}
		}
	}()
}

// ExecutionStartWorkerConfig maps configuration onto the worker's settings.
func ExecutionStartWorkerConfig(cfg config.Config) mktusecases.ExecutionStartWorkerConfig {
	capabilities := cfg.ExecutionCapabilities()
	workerConfig := mktusecases.ExecutionStartWorkerConfig{
		Owner:         config.ServiceName + "-execution-start-" + ids.New("own"),
		Lease:         cfg.ExecutionStartLease,
		BatchSize:     cfg.ExecutionStartBatchSize,
		MaxAttempts:   cfg.ExecutionStartMaxAttempts,
		BaseBackoff:   cfg.ExecutionStartBackoffBase,
		WritesEnabled: func() bool { return !cfg.IntegrationsKillSwitch },
	}
	if capabilities.Dispatch {
		workerConfig.EnabledSources = append(workerConfig.EnabledSources, mktmodels.SourceDispatch)
	}
	if capabilities.Hauling {
		workerConfig.EnabledSources = append(workerConfig.EnabledSources, mktmodels.SourceHauling)
	}
	if capabilities.Taxi {
		workerConfig.EnabledSources = append(workerConfig.EnabledSources, mktmodels.SourceTaxi)
	}
	return workerConfig
}

// StartScheduler runs the Section 27 scheduled-task runner. It only claims due
// scheduled tasks and enqueues durable job occurrences; the job worker owns
// execution. Several runners may run concurrently (claims are SKIP LOCKED and
// advancing is a lease-guarded compare-and-set), and a task claimed by a
// crashed runner is reclaimed once its lease expires.
func StartScheduler(ctx context.Context, cfg config.Config, pool *pgxpool.Pool,
	metrics *observability.Metrics, wg *sync.WaitGroup) *scheduler.Runner {
	if !cfg.SchedulerEnabled {
		logging.Notice(config.ServiceName, "scheduled-task runner disabled by FLEET_SCHEDULER_ENABLED")
		return nil
	}
	runner := &scheduler.Runner{
		Repo:         scheduler.NewRepository(pool),
		Jobs:         jobs.NewQueue(pool),
		DB:           pool,
		Owner:        ids.New("sch"),
		Metrics:      metrics,
		PollInterval: cfg.SchedulerPollInterval,
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		logging.Success(config.ServiceName, "scheduled-task runner started")
		runner.Run(ctx)
	}()
	return runner
}
