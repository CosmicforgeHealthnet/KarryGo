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
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/redisx"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/admininvestigations"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/adminresources"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/noncestore"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/objectauth"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/retentionexec"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/riskregistry"
	platformserviceauth "cosmicforge/logistics/services/fleet-partner-service/internal/platform/serviceauth"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

func main() {
	if err := godotenv.Load(); err != nil {
		logging.Fatal("config", "failed to load .env file: %v", err)
	}

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
	go observability.ObservePoolStats(ctx, metrics, pool, 15*time.Second)

	// Sections 8–10 domain services: passwordless authentication, onboarding,
	// staff IAM and managed providers.
	components := app.BuildComponents(cfg, pool, redisClient, metrics)
	publicRoutes, identityRoutes, tenantRoutes := components.Registrars(cfg)

	var workers sync.WaitGroup
	var jobWorker *jobs.Worker
	jobQueue := jobs.NewQueue(pool)
	if cfg.WorkerEnabled {
		jobWorker = &jobs.Worker{
			Queue:             jobQueue,
			Metrics:           metrics,
			Owner:             ids.New("wrk"),
			PollInterval:      cfg.JobPollInterval,
			LeaseDuration:     cfg.JobLeaseDuration,
			HeartbeatInterval: cfg.JobHeartbeatInterval,
			Environment:       cfg.AppEnv,
		}
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
		if cfg.RetentionExecutionEnabled && components.RetentionRunner != nil {
			jobWorker.Register(retentionexec.RetentionJobType, components.RetentionRunner.HandleJob)
			logging.Notice(config.ServiceName, "retention execution worker registered")
		}
		if components.Integrations != nil {
			components.Integrations.RegisterWorker(jobWorker)
		}
		app.StartDataRetentionWorker(ctx, cfg, pool, jobQueue, jobWorker, metrics, &workers)
		// A configured Redis client makes the outbox worker deliver to a real
		// stream; without one every event stays durably PENDING in the
		// outbox and is never lost, but also never delivered — the same
		// explicit-no-transport posture DisabledPublisher always documented.
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
		// Expiry sweeps for OTP challenges, sessions and invitations.
		maintenance := &jobs.MaintenanceWorker{
			Metrics:  metrics,
			Interval: time.Minute,
			Sweeps: map[string]Expirer{
				"otp_challenges":       components.Challenges,
				"sessions":             components.Sessions,
				"staff_invitations":    components.Invitations,
				"provider_invitations": providerInvitationSweep{repo: components.ProviderRepo},
				"service_nonces":       noncestore.New(pool),
			},
		}

		workers.Add(5)
		go func() { defer workers.Done(); jobWorker.Run(ctx) }()
		go func() { defer workers.Done(); maintenance.Run(ctx) }()
		go func() {
			defer workers.Done()
			startMarketplaceReconciliation(ctx, cfg, pool, metrics)
		}()
		go func() { defer workers.Done(); app.StartAssignmentExpiryWorker(ctx, cfg, pool, metrics) }()
		go func() { defer workers.Done(); app.StartReassignmentStalenessWorker(ctx, cfg, pool, metrics) }()
		app.StartSection16Workers(ctx, cfg, components, &workers)
		app.StartSection17And18Workers(ctx, cfg, components, &workers)
		app.StartSections19To21Workers(ctx, cfg, components, &workers)
		app.StartSections22And23Workers(ctx, cfg, components, &workers)
		if !cfg.IntegrationsKillSwitch {
			workers.Add(2)
			go func() { defer workers.Done(); outboxWorker.Run(ctx) }()
			go func() { defer workers.Done(); runProviderReconciler(ctx, components, cfg.JobPollInterval) }()
		} else {
			logging.Notice("worker", "integration outbox and provider reconciliation stopped by global kill switch")
		}
		// WP10 document workers: reconciliation, expiry and deletion. Each is
		// independently flagged and each returns when ctx is cancelled, so the
		// shutdown path below waits for an in-flight pass rather than killing
		// it mid-transaction.
		app.StartDocumentWorkers(ctx, cfg, pool, metrics, &workers)
		app.StartSection15Workers(ctx, cfg, components, &workers)
		app.StartSection25Workers(ctx, cfg, components, &workers)
		app.StartSection26Workers(ctx, cfg, components, redisClient, metrics, &workers)
		if components.Integrations != nil {
			components.Integrations.StartFanoutCoordinator(ctx, &workers)
		}
		if cfg.FinanceIntegrationEnabled && !cfg.FinanceExecutionKillSwitch && !cfg.IntegrationsKillSwitch && components.Finance != nil {
			workers.Add(1)
			go func() { defer workers.Done(); _ = components.Finance.Run(ctx, financeWorkerConfig(cfg)) }()
		}
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

	// Section 28 executable retention: a DISABLED feature (the default)
	// stays ready with zero optional dependencies -- these three checks
	// are only ever appended when cfg.RetentionExecutionEnabled is true,
	// matching every other opt-in feature's readiness posture in this
	// block. Once enabled, each of the three extra retention classes'
	// mandatory dependency is checked independently and by name, so an
	// operator sees exactly which one is missing (never a secret or an
	// internal error string -- see app.go's ReadyCheck contract, which
	// already reduces every failure to {dependency, "unavailable"}
	// before it reaches the response body). components.Retention*Ready
	// were computed once, at composition time (Registrars, called
	// above via components.Registrars(cfg)), from whether each real
	// client/authority was actually configured -- checking the same
	// already-computed boolean here rather than re-probing a live
	// dependency on every /ready poll.
	if cfg.RetentionExecutionEnabled {
		readyChecks = append(readyChecks,
			app.ReadyCheck{Name: "retention_expired_export", Check: func(context.Context) error {
				if !components.RetentionExpiredExportReady {
					return errors.New("expired-export retention enabled without a configured media-file-service client")
				}
				return nil
			}},
			app.ReadyCheck{Name: "retention_support_metadata", Check: func(context.Context) error {
				if !components.RetentionSupportMetadataReady {
					return errors.New("support-metadata retention enabled without a configured support-dispute-service client")
				}
				return nil
			}},
			app.ReadyCheck{Name: "retention_superseded_projection", Check: func(context.Context) error {
				if !components.RetentionSupersededProjectionReady {
					return errors.New("superseded-projection retention enabled without a configured synchronization authority")
				}
				return nil
			}},
		)
	}

	if cfg.OrganizationAPIClientsEnabled || cfg.WebhooksEnabled || cfg.WebhookDeliveryWorkerEnabled {
		readyChecks = append(readyChecks, app.ReadyCheck{Name: "organization_integrations", Check: func(context.Context) error {
			if components.Integrations == nil {
				return errors.New("organization integrations enabled without a composed service")
			}
			return components.Integrations.Ready()
		}})
	}

	// Section 28 object-authorization and route-risk registries: always
	// checked, regardless of any feature flag (both registries are
	// always-on platform infrastructure, not opt-in features) -- a
	// self-consistency check, not the full every-mounted-route
	// cross-reference (that remains internal/section28contract's own
	// TestRouteRiskRegistry_CoversEveryMountedRoute/
	// TestObjectAuthorizationRegistry_CoversEveryMountedObjectRoute,
	// a CI/build-time gate; see riskregistry.SelfCheck/objectauth.SelfCheck's
	// own doc comments for why that full cross-reference is not
	// duplicated into the production binary).
	readyChecks = append(readyChecks,
		app.ReadyCheck{Name: "risk_registry", Check: func(context.Context) error { return riskregistry.SelfCheck() }},
		app.ReadyCheck{Name: "object_authorization_registry", Check: func(context.Context) error { return objectauth.SelfCheck() }},
	)

	var workerReady func() bool
	if jobWorker != nil {
		workerReady = jobWorker.Ready
	}

	// See config.Config.ServiceCredentialScopingEnabled's doc comment for
	// why this is opt-in rather than always-on: a nil ScopeVerifier makes
	// app.BuildRouter fall back to exactly the previous single-secret
	// InternalServiceAuth behavior, unchanged.
	var scopeVerifier *platformserviceauth.ScopeVerifier
	if cfg.ServiceCredentialScopingEnabled {
		versionedSecrets := platformserviceauth.ParseVersionedSecrets(cfg.ServiceSecrets)
		// Enabled but nothing to verify against is a misconfiguration, not
		// a valid "no callers yet" state -- every other internal-route
		// deployment already requires ServiceSecrets to be non-empty (see
		// cfg's own production validation), so a scoped deployment that
		// somehow ends up with zero parsed credentials indicates
		// ServiceSecrets was set in a format ParseVersionedSecrets could
		// not read, not a genuinely empty caller list.
		readyChecks = append(readyChecks, app.ReadyCheck{Name: "service_credential_scoping", Check: func(context.Context) error {
			if len(versionedSecrets) == 0 {
				return errors.New("service credential scoping enabled without any parsed versioned credentials")
			}
			return nil
		}})
		scopeVerifier = platformserviceauth.NewScopeVerifier(
			platformserviceauth.NewStore(pool),
			versionedSecrets,
			5*time.Minute,
		)
		logging.Notice(config.ServiceName, "service credential route/method/event scoping enabled")
	}

	// Section 29: enabled-worker mandatory-dependency checks. Each mirrors
	// config.ExecutionCapabilities' own AND-with-configuration gate, but
	// independently: ExecutionCapabilities silently turns a capability off
	// when its client is unconfigured (correct for the *business* gate —
	// see its own doc comment), which is exactly why readiness must check
	// the raw enablement flag on its own here, so a deployment that turned
	// a feature on but forgot its client shows unready rather than quietly
	// running with that feature disabled.
	if cfg.DispatchExecutionStartEnabled && !cfg.IntegrationsKillSwitch {
		dispatchConfigured := cfg.DispatchServiceURL != "" && len(cfg.DispatchServiceSecret) > 0
		readyChecks = append(readyChecks, app.ReadyCheck{Name: "dispatch_execution_worker", Check: func(context.Context) error {
			if !dispatchConfigured {
				return errors.New("dispatch execution-start enabled without a configured dispatch service client")
			}
			return nil
		}})
	}
	if cfg.HaulingIntegrationEnabled && !cfg.IntegrationsKillSwitch {
		haulingConfigured := cfg.Clients["driver-hauling-service"].Configured()
		readyChecks = append(readyChecks, app.ReadyCheck{Name: "hauling_execution_worker", Check: func(context.Context) error {
			if !haulingConfigured {
				return errors.New("hauling integration enabled without a configured hauling service client")
			}
			return nil
		}})
	}

	// Section 29: audit persistence must be reachable — count(*) rather than
	// a plain SELECT so an empty (but reachable) table is not mistaken for
	// an outage (a bare SELECT ... LIMIT 1 would return no rows and fail
	// the check on a fresh, empty deployment).
	readyChecks = append(readyChecks, app.ReadyCheck{Name: "audit_persistence", Check: func(checkCtx context.Context) error {
		var count int64
		return pool.QueryRow(checkCtx, "SELECT count(*) FROM fleet_partner_audit_records").Scan(&count)
	}})

	// Section 29: worker-owned backlog/staleness thresholds — only mandatory
	// once the worker loop is actually enabled (an API-only process, or a
	// deployment with FLEET_PARTNER_WORKER_ENABLED=false, correctly has
	// nothing here to be unready about).
	if cfg.WorkerEnabled {
		outboxReadiness := events.NewOutboxRepository(pool)
		readyChecks = append(readyChecks,
			app.ReadyCheck{Name: "job_stale_leases", Check: func(checkCtx context.Context) error {
				count, err := jobQueue.StaleLeaseCount(checkCtx, cfg.ReadinessStaleLeaseThreshold)
				if err != nil {
					return err
				}
				if count > 0 {
					return fmt.Errorf("stale job leases past threshold")
				}
				return nil
			}},
			app.ReadyCheck{Name: "job_dead_letter_backlog", Check: func(checkCtx context.Context) error {
				count, err := jobQueue.DeadLetterCount(checkCtx)
				if err != nil {
					return err
				}
				if count >= cfg.ReadinessJobDeadLetterCritical {
					return fmt.Errorf("dead-lettered job backlog at or above the critical threshold")
				}
				return nil
			}},
			app.ReadyCheck{Name: "outbox_backlog", Check: func(checkCtx context.Context) error {
				pending, err := outboxReadiness.PendingCount(checkCtx)
				if err != nil {
					return err
				}
				if pending >= cfg.ReadinessOutboxBacklogCritical {
					return fmt.Errorf("outbox backlog at or above the critical threshold")
				}
				age, err := outboxReadiness.OldestPendingAge(checkCtx)
				if err != nil {
					return err
				}
				if age > cfg.ReadinessOutboxStaleAfter {
					return fmt.Errorf("oldest pending outbox event exceeds the staleness threshold")
				}
				return nil
			}},
		)
		// A deployment that configured Redis intends live outbox delivery —
		// outboxPublisher silently falling back to DisabledPublisher (see
		// its own construction above: only ever happens when redisClient is
		// nil despite RedisRequired=false letting startup continue) must
		// surface here rather than deliver nothing while reporting healthy.
		if cfg.Redis.Addr != "" {
			readyChecks = append(readyChecks, app.ReadyCheck{Name: "outbox_publisher", Check: func(context.Context) error {
				if redisClient == nil {
					return errors.New("outbox delivery expected (redis configured) but the redis client is unavailable")
				}
				return nil
			}})
		}
	}

	ownerAdmin, err := adminresources.Runtime(ctx, pool)
	if err != nil {
		logging.Fatal("configuration", "Fleet owner admin configuration invalid")
	}
	// Investigation command boundary; FLEET_PARTNER_ADMIN_COMMAND_* default off.
	fleetCommands, err := admincommand.Runtime(ownerAdmin, admininvestigations.Spec(), admininvestigations.Executor{}, "FLEET_PARTNER")
	if err != nil {
		logging.Fatal("configuration", "Fleet owner admin command configuration invalid")
	}
	router := app.BuildRouter(app.Options{
		Config:        cfg,
		Metrics:       metrics,
		TokenSigner:   auth.NewTokenSigner(cfg.AccessTokenSecret),
		Verifier:      serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
		ScopeVerifier: scopeVerifier,
		NonceStore:    noncestore.New(pool),
		Limiter:       components.Limiter,
		// Real membership resolution replaces the Sections 3–7 fail-closed
		// placeholder now that staff memberships exist.
		Resolver:              components.Resolver,
		SessionStatus:         components.Sessions,
		ReadyChecks:           readyChecks,
		WorkerReady:           workerReady,
		PublicAuthRoutes:      publicRoutes,
		IdentityRoutes:        identityRoutes,
		TenantRoutes:          tenantRoutes,
		OrganizationAPIAuth:   components.OrganizationAPIAuth(),
		OrganizationAPIRoutes: components.OrganizationAPIRoutes(),
		CustomerRoutes:        components.CustomerRoutes(),
		InternalRoutes: func(group *gin.RouterGroup) {
			ownerAdmin.RegisterOn(group)
			fleetCommands.RegisterOn(group)
			if routes := buildMarketplaceInternalRoutes(cfg, pool, metrics); routes != nil {
				routes(group)
			}
			// Section 14 SLA observation ingestion shares the marketplace
			// internal route class, so it inherits HMAC v2 verification,
			// timestamp freshness and nonce replay protection rather than
			// growing its own.
			components.InternalCRMRoutes()(group)
			components.InternalPrivateOperationsRoutes()(group)
			components.InternalSection16Routes()(group)
			components.InternalSection22To23Routes()(group)
			components.InternalFleetPartnerRoutes(cfg)(group)
			if routes := buildWorkerOperationsInternalRoutes(cfg, pool); routes != nil {
				routes(group)
			}
			if routes := buildProviderMembershipInternalRoutes(cfg, components.Providers); routes != nil {
				routes(group)
			}
		},
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
