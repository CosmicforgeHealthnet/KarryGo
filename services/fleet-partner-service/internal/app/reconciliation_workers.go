package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/automation"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retention"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// RegisterAutomationRunner attaches the Section 27 automation-runner
// registration boundary (see internal/features/automation) to the shared
// durable queue when AUTOMATION_RUNNER_WORKER_ENABLED is set. Off by
// default, matching every other Section 27 worker flag; there is no
// automation domain to execute yet (Section 24 is not implemented), so
// enabling this only changes whether AUTOMATION_RULE_EXECUTE fails as
// "no handler registered" (disabled) or as a classified, dead-lettered
// "automation domain not implemented" (enabled) -- neither path performs
// any AI decision-making or arbitrary code execution.
func RegisterAutomationRunner(cfg config.Config, worker *jobs.Worker) {
	if !cfg.AutomationRunnerWorkerEnabled || worker == nil {
		return
	}
	automation.RegisterWorkers(worker)
}

// StartDataRetentionWorker runs the Section 27 "data retention" required
// worker on DATA_RETENTION_WORKER_ENABLED: registers the scan-and-report
// handler on the shared durable queue and periodically enqueues it. See
// internal/features/retention's package doc for why this is a report-only
// capability scoped to Section 27's own operational tables rather than a
// destructive cross-domain anonymization engine.
func StartDataRetentionWorker(ctx context.Context, cfg config.Config, pool *pgxpool.Pool,
	queue *jobs.Queue, jobWorker *jobs.Worker, metrics *observability.Metrics, wg *sync.WaitGroup) {
	if !cfg.DataRetentionWorkerEnabled {
		logging.Notice(config.ServiceName, "data retention worker disabled by DATA_RETENTION_WORKER_ENABLED")
		return
	}
	retentionWorker := &retention.Worker{
		Service:    retention.NewService(pool),
		Queue:      queue,
		DB:         pool,
		Metrics:    metrics,
		CutoffDays: 90,
		BatchLimit: cfg.DataRetentionBatchSize,
	}
	retentionWorker.RegisterWorkers(jobWorker)
	logging.Success(config.ServiceName, "data retention worker started (report-only)")
	wg.Add(1)
	go func() {
		defer wg.Done()
		retentionWorker.RunPeriodic(ctx, cfg.DataRetentionScanInterval)
	}()
}

// RunProviderReconciler re-drives incomplete provider provisioning sagas.
// Unknown outcomes are resolved by read-back inside Reconcile.
//
// Exported (rather than living privately in one binary's cmd package) so
// both the API binary and the dedicated worker binary run the identical
// reconciliation loop -- a deployment that splits into a dedicated worker
// process must not silently lose provider provisioning reconciliation.
func RunProviderReconciler(ctx context.Context, components *Components, interval time.Duration) {
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
			_, _ = components.Providers.Reconcile(ctx, 25)
		}
	}
}

// StartMarketplaceReconciliation runs marketplace projection reconciliation
// and the stale-trip sweep until ctx ends.
//
// Disabled by default. When off, gapped projections stay GAP_DETECTED and
// commands against them keep refusing -- degraded but safe -- which is the
// correct posture for a deployment that has not configured a dispatch
// client to read back from.
//
// Exported for the same reason as RunProviderReconciler: both binaries must
// run the same worker, built through the same composition root
// (BuildMarketplaceService), or they risk ending up with different
// repository sets -- exactly the defect BuildMarketplaceService's own doc
// comment records having happened once already.
func StartMarketplaceReconciliation(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, metrics *observability.Metrics) {
	if !cfg.MarketplaceReconciliationEnabled {
		logging.Notice(config.ServiceName,
			"marketplace reconciliation disabled by FLEET_MARKETPLACE_RECONCILIATION_ENABLED")
		return
	}
	service := BuildMarketplaceService(cfg, pool, metrics)
	logging.Success(config.ServiceName, "marketplace reconciliation worker started")
	// RunReconciliationLoop returns when ctx is cancelled, which is how the
	// composition root's shutdown path stops it; main waits on the
	// WaitGroup, so an in-flight pass finishes rather than being killed
	// mid-transaction.
	_ = service.RunReconciliationLoop(ctx, cfg.MarketplaceReconcileInterval,
		mktusecases.ReconciliationConfig{
			Owner:       config.ServiceName,
			BatchSize:   cfg.MarketplaceReconcileBatchSize,
			StaleAfter:  cfg.MarketplaceProjectionStaleAfter,
			Lease:       cfg.MarketplaceReconcileLease,
			MaxAttempts: cfg.MarketplaceReconcileMaxAttempts,
			BaseBackoff: cfg.MarketplaceReconcileBackoffBase,
			MaxBackoff:  cfg.MarketplaceReconcileBackoffMax,
			SweepLimit:  cfg.MarketplaceReconcileSweepLimit,
		})
}

// StartAssignmentExpiryWorker runs the Section 27 "assignment expiry"
// required worker: on ASSIGNMENT_EXPIRY_WORKER_ENABLED, periodically expires
// overdue pending assignments via the owning marketplace domain's own
// SKIP-LOCKED claim (ExpireAssignments). Expiry never auto-accepts or
// auto-reassigns -- it releases the provider/asset hold and opens a
// reassignment case for a human (or, once available, the reassignment
// coordinator below) to act on.
//
// Disabled by default -- off means overdue assignments simply stay
// PENDING_PROVIDER_RESPONSE past their deadline until a human intervenes or
// the worker is enabled, never silently auto-committing anything.
func StartAssignmentExpiryWorker(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, metrics *observability.Metrics) {
	if !cfg.AssignmentExpiryWorkerEnabled {
		logging.Notice(config.ServiceName, "assignment expiry worker disabled by ASSIGNMENT_EXPIRY_WORKER_ENABLED")
		return
	}
	service := BuildMarketplaceService(cfg, pool, metrics)
	interval := cfg.AssignmentExpiryInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	logging.Success(config.ServiceName, "assignment expiry worker started")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := service.ExpireAssignments(ctx, cfg.AssignmentExpiryBatchSize); err != nil {
				logging.Notice(config.ServiceName, "assignment expiry pass error: %v", err)
			}
		}
	}
}

// StartReassignmentStalenessWorker runs the Section 27 "reassignment
// coordinator" required worker: on REASSIGNMENT_WORKER_ENABLED, periodically
// scans OPEN reassignment cases older than ReassignmentStaleAfter and
// reports how many need dispatcher attention via metrics.
//
// Automatic candidate re-selection is deliberately NOT performed here: the
// marketplace domain has no "next eligible candidate" picker independent of
// the human/dispatcher-driven Assign flow (dispatch board candidates are
// Section 16's concern), and Section 27's own architecture rule is that a
// generic background worker must call an owning-domain capability, not
// invent domain business logic that capability does not yet expose. This
// worker owns the scheduling/visibility half of the requirement --
// coordinating attention to stuck cases -- while the actual reassignment
// decision stays exactly where ExpireAssignments's own doc comment already
// puts it: with a human dispatcher calling Reassign.
func StartReassignmentStalenessWorker(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, metrics *observability.Metrics) {
	if !cfg.ReassignmentWorkerEnabled {
		logging.Notice(config.ServiceName, "reassignment coordinator worker disabled by REASSIGNMENT_WORKER_ENABLED")
		return
	}
	service := BuildMarketplaceService(cfg, pool, metrics)
	interval := cfg.ReassignmentScanInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	staleAfter := cfg.ReassignmentStaleAfter
	if staleAfter <= 0 {
		staleAfter = 5 * time.Minute
	}
	automatic := cfg.MarketplaceAutomaticReassignmentEnabled
	logging.Success(config.ServiceName, "reassignment coordinator worker started (automatic=%v)", automatic)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runReassignmentPass(ctx, service, staleAfter, automatic, metrics)
		}
	}
}

// runReassignmentPass surfaces stale reassignment cases via a gauge, and --
// only when explicitly enabled -- attempts automatic reassignment for each
// one. An attempt that finds no eligible candidate (ErrNoEligibleCandidate)
// leaves the case open for a human, exactly the pre-existing safe fallback;
// only genuinely unexpected errors are logged as such.
func runReassignmentPass(ctx context.Context, service *mktusecases.Service, staleAfter time.Duration, automatic bool, metrics *observability.Metrics) {
	count, err := service.ScanStaleReassignmentCases(ctx, staleAfter)
	if err != nil {
		logging.Notice(config.ServiceName, "reassignment staleness scan error: %v", err)
		return
	}
	metrics.SetGauge("fleet_partner_reassignment_cases_stale", nil, float64(count))
	if !automatic || count == 0 {
		return
	}
	cases, err := service.ListStaleReassignmentCases(ctx, staleAfter, 50)
	if err != nil {
		logging.Notice(config.ServiceName, "reassignment staleness list error: %v", err)
		return
	}
	for _, reassignmentCase := range cases {
		t := tenant.Context{OrganizationID: reassignmentCase.OrganizationID,
			ActorType: tenant.ActorTypeSystem, ActorID: "reassignment-coordinator"}
		_, err := service.AutoReassignStaleCase(ctx, t, reassignmentCase, "automatic_reassignment")
		switch {
		case err == nil:
			metrics.IncCounter("fleet_partner_reassignment_auto_succeeded_total", nil, 1)
		case errors.Is(err, mktusecases.ErrNoEligibleCandidate):
			metrics.IncCounter("fleet_partner_reassignment_auto_no_candidate_total", nil, 1)
		default:
			metrics.IncCounter("fleet_partner_reassignment_auto_error_total", nil, 1)
			logging.Notice(config.ServiceName, "automatic reassignment error for case %s: %v", reassignmentCase.ID, err)
		}
	}
}
