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

	"github.com/joho/godotenv"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
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

	jobWorker := &jobs.Worker{
		Queue:             jobs.NewQueue(pool),
		Metrics:           metrics,
		Owner:             ids.New("wrk"),
		PollInterval:      cfg.JobPollInterval,
		LeaseDuration:     cfg.JobLeaseDuration,
		HeartbeatInterval: cfg.JobHeartbeatInterval,
	}
	outboxWorker := &events.OutboxWorker{
		Repository:   events.NewOutboxRepository(pool),
		Publisher:    events.DisabledPublisher{},
		Metrics:      metrics,
		Owner:        ids.New("pub"),
		PollInterval: cfg.OutboxPollInterval,
		Lease:        cfg.JobLeaseDuration,
	}

	// Execution-start worker: turns a provider's durable acceptance into the
	// canonical trip. Returns a no-op stopper when disabled or unconfigured.
	stopExecutionStart := startExecutionStartWorker(ctx, cfg, pool, metrics)

	var workers sync.WaitGroup
	workers.Add(2)
	go func() { defer workers.Done(); jobWorker.Run(ctx) }()
	go func() { defer workers.Done(); outboxWorker.Run(ctx) }()
	logging.Success(config.ServiceName+"-worker", "workers running (outbox publisher transport: disabled adapter)")

	<-ctx.Done()
	logging.Notice(config.ServiceName+"-worker", "shutting down")
	workers.Wait()
	stopExecutionStart()
	logging.Success(config.ServiceName+"-worker", "stopped")
}
