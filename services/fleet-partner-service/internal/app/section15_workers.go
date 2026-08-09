package app

import (
	"context"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	bulkusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	recurringusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/usecases"
)

// StartSection15Workers starts only the durable local workers that have been
// explicitly enabled. Private execution receives only production HMAC
// authority adapters; missing configuration prevents the worker from starting.
func StartSection15Workers(ctx context.Context, cfg config.Config, components *Components, workers *sync.WaitGroup) {
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
	if cfg.RecurringOrdersWorkerEnabled && components.RecurringOrders != nil {
		start(func(runCtx context.Context) {
			_, _ = components.RecurringOrders.RunGenerationPass(runCtx, recurringusecases.WorkerConfig{Owner: config.ServiceName + "-recurring", Lease: cfg.JobLeaseDuration, BatchSize: 25, MaxAttempts: 8})
		})
	}
	if cfg.BulkImportsWorkerEnabled && components.BulkImports != nil {
		start(func(runCtx context.Context) {
			_, _ = components.BulkImports.RunWorkerPass(runCtx, bulkusecases.WorkerConfig{Owner: config.ServiceName + "-bulk-import", Lease: cfg.JobLeaseDuration, BatchSize: 10, MaxAttempts: 8})
		})
	}
	// Section 25C generic import workers, additive alongside the legacy
	// bulk-import worker above — same Service, disjoint claim queries.
	if cfg.ImportsValidationWorkerEnabled && components.BulkImports != nil {
		start(func(runCtx context.Context) {
			_, _ = components.BulkImports.RunValidationPass(runCtx, bulkusecases.GenericWorkerConfig{Owner: config.ServiceName + "-import-validate", Lease: cfg.JobLeaseDuration, BatchSize: 10, MaxAttempts: 8})
		})
	}
	if cfg.ImportsCommitWorkerEnabled && components.BulkImports != nil {
		start(func(runCtx context.Context) {
			_, _ = components.BulkImports.RunCommitPass(runCtx, bulkusecases.GenericWorkerConfig{Owner: config.ServiceName + "-import-commit", Lease: cfg.JobLeaseDuration, BatchSize: 10, MaxAttempts: 8})
		})
	}
	if cfg.ImportsRetryWorkerEnabled && components.BulkImports != nil {
		start(func(runCtx context.Context) {
			_, _ = components.BulkImports.RunRetryPass(runCtx, bulkusecases.GenericWorkerConfig{Owner: config.ServiceName + "-import-retry", Lease: cfg.JobLeaseDuration, BatchSize: 10, MaxAttempts: 8, Backoff: 30 * time.Second})
		})
	}
	if cfg.PrivateAssignmentsWorkerEnabled && components.PrivateOperations != nil {
		start(func(runCtx context.Context) {
			_, _ = components.PrivateOperations.ExpirePass(runCtx, 100)
		})
	}
	if cfg.PrivateExecutionWorkerEnabled && components.PrivateOperations != nil &&
		(cfg.PrivateDispatchExecutionEnabled || cfg.PrivateHaulingExecutionEnabled) {
		adapter := BuildPrivateExecutionAdapter(cfg, nil)
		projector := NewPrivateExecutionEventProjector(components.Pool,
			events.NewDomainPublisher(events.NewOutboxRepository(components.Pool)))
		start(func(runCtx context.Context) {
			_, _ = components.PrivateOperations.RunExecutionPass(runCtx, adapter,
				privateusecases.ExecutionWorkerConfig{Owner: config.ServiceName + "-private-execution",
					Lease: cfg.JobLeaseDuration, BatchSize: 20, MaxAttempts: 8, BaseBackoff: 5 * time.Second})
		})
		start(func(runCtx context.Context) {
			_, _ = components.PrivateOperations.RunProjectionReconciliationPass(runCtx, adapter, projector,
				privateusecases.ReconciliationWorkerConfig{Owner: config.ServiceName + "-private-projection",
					Lease: cfg.JobLeaseDuration, BatchSize: 20, MaxAttempts: 8, BaseBackoff: 5 * time.Second})
		})
	}
}
