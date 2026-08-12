package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// startExecutionStartWorker builds and runs the execution-start worker.
//
// It returns a stop function that blocks until the loop has exited, so the
// process does not terminate while a claimed operation is mid-flight.
//
// Nothing starts when the feature flag is off or the source execution clients
// are unconfigured: a worker that can only fail would consume every queued
// operation's retry budget and dead-letter provider acceptances that were
// perfectly valid.
func startExecutionStartWorker(ctx context.Context, cfg config.Config,
	pool *pgxpool.Pool, metrics *observability.Metrics) func() {
	capabilities := cfg.ExecutionCapabilities()
	if !capabilities.Any() {
		logging.Notice(config.ServiceName+"-worker",
			"execution-start worker disabled: no execution source is enabled")
		return func() {}
	}

	executions := platformclients.BuildExecutionSet(platformclients.Options{
		ServiceName: config.ServiceName,
		BaseURL:     cfg.DispatchServiceURL,
		Secret:      cfg.DispatchServiceSecret,
		Timeout:     cfg.DispatchServiceTimeout,
		Metrics:     metrics,
	})
	if cfg.HaulingIntegrationEnabled && !cfg.IntegrationsKillSwitch {
		hauling := cfg.Clients["driver-hauling-service"]
		executions = platformclients.BuildExecutionSet(platformclients.Options{
			ServiceName: config.ServiceName, BaseURL: cfg.DispatchServiceURL,
			Secret: cfg.DispatchServiceSecret, Timeout: cfg.DispatchServiceTimeout, Metrics: metrics,
		}, platformclients.Options{
			ServiceName: config.ServiceName, BaseURL: hauling.BaseURL,
			Secret: hauling.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics,
		})
	}
	if !executions.AnyConfigured() {
		logging.Notice(config.ServiceName+"-worker",
			"execution-start worker not started: no execution service is configured")
		return func() {}
	}

	service := mktusecases.NewService(mktusecases.Options{
		Pool:            pool,
		Projections:     mktrepos.NewProjectionRepository(pool),
		Offers:          mktrepos.NewOfferRepository(pool),
		Assignments:     mktrepos.NewAssignmentRepository(pool),
		ExecutionStarts: mktrepos.NewExecutionStartRepository(pool),
		Executions:      executions,
		Events:          events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Metrics:         metrics,
	})

	workerConfig := mktusecases.ExecutionStartWorkerConfig{
		Owner:         config.ServiceName + "-execution-start",
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

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		interval := cfg.ExecutionStartInterval
		if interval <= 0 {
			interval = 5 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				// RunExpiredLeases runs at the top of every pass inside
				// RunExecutionStart, so work abandoned by a previous process
				// is recovered here rather than stranded.
				if _, err := service.RunExecutionStart(workerCtx, workerConfig); err != nil &&
					workerCtx.Err() == nil {
					// A failed pass must not kill the worker: the operations
					// are durable and the next tick retries them.
					logging.Notice(config.ServiceName+"-worker",
						"execution-start pass failed: %v", err)
				}
			}
		}
	}()

	logging.Success(config.ServiceName+"-worker", "execution-start worker running")
	return func() {
		cancel()
		<-done
		logging.Notice(config.ServiceName+"-worker", "execution-start worker stopped")
	}
}
