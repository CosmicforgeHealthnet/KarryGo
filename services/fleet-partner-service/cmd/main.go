// fleet-partner-service HTTP API process.
//
// Loads and validates configuration (failing fast), connects the dedicated
// PostgreSQL database and optional Redis, applies migrations when
// MIGRATION=true, optionally runs the in-process worker loops
// (FLEET_PARTNER_WORKER_ENABLED), and serves the corporate, internal,
// health, readiness and protected metrics routes with graceful shutdown.
package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/redisx"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		logging.Fatal("config", "%v", err)
	}
	logging.Notice(config.ServiceName, "env=%s addr=%s worker=%t migration=%t", cfg.AppEnv, cfg.HTTPAddr, cfg.WorkerEnabled, cfg.Migration)

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

	var redisClient *redis.Client
	if cfg.Redis.Addr != "" {
		redisClient, err = redisx.NewClient(ctx, cfg.Redis)
		if err != nil {
			if cfg.RedisRequired {
				logging.Fatal("redis", "connect: %v", err)
			}
			logging.Notice("redis", "unavailable, continuing degraded: %v", err)
		} else {
			defer redisClient.Close()
			logging.Success("redis", "connected")
		}
	}

	metrics := observability.NewMetrics()
	clients := platformclients.NewSet(cfg, metrics)
	_ = clients // foundational transport adapters (Sections 3–7)

	// Sections 8–10 domain services: passwordless authentication, onboarding,
	// staff IAM and managed providers.
	components := app.BuildComponents(cfg, pool, redisClient, metrics)
	publicRoutes, identityRoutes, tenantRoutes := components.Registrars(cfg)

	var workers sync.WaitGroup
	var jobWorker *jobs.Worker
	if cfg.WorkerEnabled {
		jobWorker = &jobs.Worker{
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
		// Expiry sweeps for OTP challenges, sessions and invitations.
		maintenance := &jobs.MaintenanceWorker{
			Metrics:  metrics,
			Interval: time.Minute,
			Sweeps: map[string]Expirer{
				"otp_challenges":       components.Challenges,
				"sessions":             components.Sessions,
				"staff_invitations":    components.Invitations,
				"provider_invitations": providerInvitationSweep{repo: components.ProviderRepo},
			},
		}

		workers.Add(4)
		go func() { defer workers.Done(); jobWorker.Run(ctx) }()
		go func() { defer workers.Done(); outboxWorker.Run(ctx) }()
		go func() { defer workers.Done(); maintenance.Run(ctx) }()
		go func() { defer workers.Done(); runProviderReconciler(ctx, components, cfg.JobPollInterval) }()
		logging.Notice("worker", "in-process workers started (outbox publisher transport: disabled adapter)")
	}

	readyChecks := []app.ReadyCheck{
		{Name: "postgres", Check: pool.Ping},
		{Name: "migrations", Check: func(checkCtx context.Context) error {
			return database.MigrationsCurrent(checkCtx, pool, "migrations")
		}},
	}
	if cfg.RedisRequired && redisClient != nil {
		readyChecks = append(readyChecks, app.ReadyCheck{Name: "redis", Check: func(checkCtx context.Context) error {
			return redisClient.Ping(checkCtx).Err()
		}})
	}

	var workerReady func() bool
	if jobWorker != nil {
		workerReady = jobWorker.Ready
	}

	router := app.BuildRouter(app.Options{
		Config:      cfg,
		Metrics:     metrics,
		TokenSigner: auth.NewTokenSigner(cfg.AccessTokenSecret),
		Verifier:    serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
		// Real membership resolution replaces the Sections 3–7 fail-closed
		// placeholder now that staff memberships exist.
		Resolver:         components.Resolver,
		ReadyChecks:      readyChecks,
		WorkerReady:      workerReady,
		PublicAuthRoutes: publicRoutes,
		IdentityRoutes:   identityRoutes,
		TenantRoutes:     tenantRoutes,
		InternalRoutes:   buildMarketplaceInternalRoutes(cfg, pool, metrics),
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logging.Success(config.ServiceName, "listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Fatal(config.ServiceName, "server stopped: %v", err)
		}
	}()

	<-ctx.Done()
	logging.Notice(config.ServiceName, "shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logging.Error(config.ServiceName, "http shutdown: %v", err)
	}
	workers.Wait()
	logging.Success(config.ServiceName, "stopped")
}
