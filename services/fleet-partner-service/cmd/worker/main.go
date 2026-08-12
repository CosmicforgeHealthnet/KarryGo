// fleet-partner-service dedicated worker process.
//
// Runs the durable-job worker and the outbox publisher without the HTTP API
// (deployments may instead enable in-process workers on the API binary via
// FLEET_PARTNER_WORKER_ENABLED). Exposes nothing over HTTP; readiness of a
// dedicated worker deployment is observed through job/lease metrics on the
// API process and process supervision.
package main

import (
	"context"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"cosmicforge/logistics/shared/go/redisx"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	financefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/finance"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/retentionexec"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		logging.Fatal("config", "%v", err)
	}
	logging.Notice(config.ServiceName+"-worker", "env=%s", cfg.AppEnv)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL, database.PoolOptions{
		MaxConns:        cfg.DatabaseMaxOpenConnections,
		MinConns:        cfg.DatabaseMaxIdleConnections,
		MaxConnLifetime: cfg.DatabaseConnectionMaxLife,
	})
	if err != nil {
		logging.Fatal("database", "connect dedicated database: %v", err)
	}
	defer pool.Close()
	logging.Success("database", "connected")

	if cfg.Migration {
		migrationCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		err := database.ApplyMigrations(migrationCtx, pool, "migrations")
		cancel()
		if err != nil {
			logging.Fatal("migration", "%v", err)
		}
		logging.Success("migration", "applied")
	}

	metrics := observability.NewMetrics()
	go observability.ObservePoolStats(ctx, metrics, pool, 15*time.Second)
	var redisClient *redis.Client
	if cfg.Redis.Addr != "" {
		if client, redisErr := redisx.NewClient(ctx, cfg.Redis); redisErr == nil {
			redisClient = client
			defer redisClient.Close()
		}
	}

	jobQueue := jobs.NewQueue(pool)
	jobWorker := &jobs.Worker{
		Queue:             jobQueue,
		Metrics:           metrics,
		Owner:             ids.New("wrk"),
		PollInterval:      cfg.JobPollInterval,
		LeaseDuration:     cfg.JobLeaseDuration,
		HeartbeatInterval: cfg.JobHeartbeatInterval,
		Environment:       cfg.AppEnv,
	}
	// A configured Redis client makes the outbox worker deliver to a real
	// stream; without one every event stays durably PENDING in the outbox
	// and is never lost, but also never delivered -- the same
	// explicit-no-transport posture DisabledPublisher always documented.
	// Mirrors cmd/main.go exactly, so a deployment cannot end up with a
	// dedicated worker process silently dropping outbox delivery that the
	// in-process API-binary option would have delivered.
	var outboxPublisher events.Publisher = events.DisabledPublisher{}
	if redisClient != nil {
		outboxPublisher = events.NewRedisStreamPublisher(redisClient, cfg.OutboxStreamKey)
	}
	outboxWorker := &events.OutboxWorker{
		Repository:   events.NewOutboxRepository(pool),
		Publisher:    outboxPublisher,
		Metrics:      metrics,
		Owner:        ids.New("pub"),
		PollInterval: cfg.OutboxPollInterval,
		Lease:        cfg.JobLeaseDuration,
		Environment:  cfg.AppEnv,
	}

	// Execution-start worker: turns a provider's durable acceptance into the
	// canonical trip. Returns a no-op stopper when disabled or unconfigured.
	stopExecutionStart := startExecutionStartWorker(ctx, cfg, pool, metrics)

	// Section 13 allocation and withdrawal execution plus projection refresh.
	// Built through the same composition helper the API binary uses, so the
	// two processes cannot end up with different gates.
	components := app.BuildComponents(cfg, pool, nil, metrics)
	// Registrars populates components.RetentionRunner (and its three
	// dependency-readiness booleans) as a side effect of building the API
	// binary's route registrars -- a dedicated worker process never calls
	// Registrars otherwise (it has no HTTP routes to mount), so without
	// this call components.RetentionRunner stays nil here and this
	// process would never actually run a retention job despite the API
	// process enqueueing one. cfg is discarded on purpose: none of the
	// three RouteRegistrar return values are used by a worker-only binary.
	_, _, _ = components.Registrars(cfg)
	if cfg.RetentionExecutionEnabled && components.RetentionRunner != nil {
		jobWorker.Register(retentionexec.RetentionJobType, components.RetentionRunner.HandleJob)
		logging.Notice(config.ServiceName+"-worker", "retention execution worker registered")
	}
	if components.Integrations != nil {
		components.Integrations.RegisterWorker(jobWorker)
	}

	var workers sync.WaitGroup
	workers.Add(1)
	go func() { defer workers.Done(); jobWorker.Run(ctx) }()
	if !cfg.IntegrationsKillSwitch {
		workers.Add(2)
		go func() { defer workers.Done(); outboxWorker.Run(ctx) }()
		// Re-drives incomplete provider provisioning sagas. Shared with the
		// API binary through internal/app so a deployment that splits into a
		// dedicated worker process does not silently lose this worker.
		go func() { defer workers.Done(); app.RunProviderReconciler(ctx, components, cfg.JobPollInterval) }()
	} else {
		logging.Notice(config.ServiceName+"-worker", "outbox delivery and provider reconciliation stopped by global integration kill switch")
	}
	// Marketplace projection reconciliation, assignment expiry and
	// reassignment staleness -- each self-gated on its own flag, same as
	// the API binary's in-process option, so a deployment that splits into
	// a dedicated worker process does not silently lose these workers.
	workers.Add(3)
	go func() { defer workers.Done(); app.StartMarketplaceReconciliation(ctx, cfg, pool, metrics) }()
	go func() { defer workers.Done(); app.StartAssignmentExpiryWorker(ctx, cfg, pool, metrics) }()
	go func() { defer workers.Done(); app.StartReassignmentStalenessWorker(ctx, cfg, pool, metrics) }()
	if cfg.FinanceIntegrationEnabled && !cfg.FinanceExecutionKillSwitch && !cfg.IntegrationsKillSwitch {
		if financeService := app.BuildFinanceService(cfg, pool, metrics); financeService != nil {
			workers.Add(1)
			go func() {
				defer workers.Done()
				_ = financeService.Run(ctx, financefeature.WorkerConfig{Owner: config.ServiceName + "-finance", BatchSize: cfg.FinanceWorkerBatchSize, Interval: cfg.FinanceWorkerInterval, Lease: cfg.FinanceWorkerLease, MaxAttempts: cfg.FinanceWorkerMaxAttempts, BaseBackoff: cfg.FinanceWorkerBackoff, ProjectionStaleAfter: cfg.FinanceProjectionStaleAfter})
			}()
		}
	}
	// WP10 document workers. A deployment that runs the dedicated worker
	// process must get the same sweeps the in-process option provides;
	// otherwise expiries and deletions would silently stop happening the
	// moment somebody split the deployment.
	app.StartDocumentWorkers(ctx, cfg, pool, metrics, &workers)
	if cfg.Section16WorkersEnabled && components.DispatchControl != nil {
		components.DispatchControl.RegisterWorkers(jobWorker)
	}
	if cfg.Section17WorkersEnabled && components.Workforce != nil {
		components.Workforce.RegisterWorkers(jobWorker)
	}
	if cfg.Section18WorkersEnabled && components.Assets != nil {
		components.Assets.RegisterWorkers(jobWorker)
	}
	app.RegisterSections19To21Workers(cfg, components, jobWorker)
	app.RegisterSections22And23Workers(cfg, components, jobWorker)
	app.RegisterAutomationRunner(cfg, jobWorker)
	app.StartDataRetentionWorker(ctx, cfg, pool, jobQueue, jobWorker, metrics, &workers)
	app.StartCompanyWalletWorkers(ctx, cfg, pool, metrics, &workers, components.CompanyWallet)
	// Section 14 quotation expiry, contract expiry and renewal sweeps, SLA
	// evaluation and escalation, and the durable notification drain. Built
	// through the same composition helper the API binary uses, so a split
	// deployment cannot end up with different gates.
	app.StartCRMWorkers(ctx, cfg, &workers, components.CRM)
	app.StartSection15Workers(ctx, cfg, components, &workers)
	app.StartSection16Workers(ctx, cfg, components, &workers)
	app.StartSection17And18Workers(ctx, cfg, components, &workers)
	app.StartSections19To21Workers(ctx, cfg, components, &workers)
	app.StartSections22And23Workers(ctx, cfg, components, &workers)
	app.StartSection25Workers(ctx, cfg, components, &workers)
	app.StartSection26Workers(ctx, cfg, components, redisClient, metrics, &workers)
	if components.Integrations != nil {
		components.Integrations.StartFanoutCoordinator(ctx, &workers)
	}
	transportLabel := "disabled adapter"
	if redisClient != nil {
		transportLabel = "redis stream"
	}
	logging.Success(config.ServiceName+"-worker", "workers running (outbox publisher transport: %s)", transportLabel)

	<-ctx.Done()
	logging.Notice(config.ServiceName+"-worker", "shutting down")
	workers.Wait()
	stopExecutionStart()
	logging.Success(config.ServiceName+"-worker", "stopped")
}
