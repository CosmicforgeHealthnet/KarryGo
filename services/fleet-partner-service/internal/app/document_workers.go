package app

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	documentusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// NewDocumentService builds the document use-case service from configuration.
//
// Shared by the internal ingestion route, the API binary's in-process workers
// and the dedicated worker binary, so all of them hold the same repositories,
// the same media client and the same flags. Building it separately in each
// place invited exactly the class of defect this pattern replaced elsewhere in
// this service: one construction site gaining a dependency the others silently
// lacked.
func NewDocumentService(cfg config.Config, pool *pgxpool.Pool,
	metrics *observability.Metrics) *documentusecases.Service {
	return BuildDocumentService(cfg, pool, metrics,
		events.NewDomainPublisher(events.NewOutboxRepository(pool)))
}

// StartDocumentWorkers runs the reconciliation, expiry and deletion loops.
//
// All three are disabled by default and each is independently flagged. When
// off, documents whose projection went quiet stay CURRENT until an event
// arrives, expired documents keep their stated expiry date without the
// lifecycle flag, and scheduled deletions simply wait — degraded but safe,
// which is the correct posture for a deployment that has not configured a
// media client to act against.
//
// Every loop returns when ctx is cancelled, so a composition root that waits
// on the WaitGroup lets an in-flight pass finish rather than killing it
// mid-transaction.
func StartDocumentWorkers(ctx context.Context, cfg config.Config, pool *pgxpool.Pool,
	metrics *observability.Metrics, workers *sync.WaitGroup) {
	if cfg.DocumentsKillSwitch || cfg.IntegrationsKillSwitch {
		logging.Notice(config.ServiceName, "document workers stopped by FLEET_DOCUMENTS_KILL_SWITCH")
		return
	}
	if !cfg.DocumentReconcileEnabled && !cfg.DocumentExpiryEnabled && !cfg.DocumentDeletionEnabled {
		logging.Notice(config.ServiceName, "document workers disabled by configuration")
		return
	}

	service := NewDocumentService(cfg, pool, metrics)
	owner := config.ServiceName + ":" + ids.New("dwk")

	if cfg.DocumentReconcileEnabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_ = service.RunReconciliationWorker(ctx, cfg.DocumentReconcileInterval,
				documentusecases.ReconciliationConfig{
					Owner:       owner,
					BatchSize:   cfg.DocumentReconcileBatchSize,
					Lease:       cfg.DocumentReconcileLease,
					Backoff:     cfg.DocumentReconcileBackoff,
					MaxAttempts: cfg.DocumentReconcileMaxAttempts,
					StaleAfter:  cfg.DocumentProjectionStaleAfter,
					SweepLimit:  cfg.DocumentReconcileSweepLimit,
				})
		}()
		logging.Success(config.ServiceName, "document reconciliation worker started")
	}

	if cfg.DocumentExpiryEnabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_ = service.RunExpiryWorker(ctx, cfg.DocumentExpiryInterval, cfg.DocumentExpiryBatchSize)
		}()
		logging.Success(config.ServiceName, "document expiry worker started")
	}

	if cfg.DocumentDeletionEnabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_ = service.RunDeletionWorker(ctx, owner, cfg.DocumentDeletionInterval,
				cfg.DocumentDeletionLease, cfg.DocumentDeletionBatchSize,
				cfg.DocumentDeletionBackoff, cfg.DocumentDeletionMaxAttempts)
		}()
		logging.Success(config.ServiceName, "document deletion worker started")
	}
}
