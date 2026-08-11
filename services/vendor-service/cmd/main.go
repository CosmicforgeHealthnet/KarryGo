package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/vendor-service/internal/audit"
	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/jobs"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/vendor-service/internal/platform/ratelimit"

	vendorauthclients "cosmicforge/logistics/services/vendor-service/internal/features/auth/clients"
	vendorauthhttp "cosmicforge/logistics/services/vendor-service/internal/features/auth/http"
	vendorauthrepositories "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
	vendorauthusecases "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"

	staffhttp "cosmicforge/logistics/services/vendor-service/internal/features/staff/http"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	vendors "cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	mediaintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/media"
	verificationintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/verification"
	walletintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/wallet"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logging.Fatal("configuration", "%v", err)
	}
	logging.Notice("vendor-service", "migration=%t vendor_service_enabled=%t redis=%s",
		cfg.Migration, cfg.Flags.VendorServiceEnabled, cfg.Redis.Addr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create vendor-service database pool: %v", err)
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	logConnectivity(ctx, cfg.Redis.Addr, db, redisClient)

	if cfg.Migration {
		logging.Notice("migration", "mode enabled")
		migrationCtx, mcancel := context.WithTimeout(ctx, 30*time.Second)
		if err := database.ApplyMigrations(migrationCtx, db, "migrations"); err != nil {
			mcancel()
			logging.Fatal("migration", "apply vendor-service migrations: %v", err)
		}
		mcancel()
		logging.Success("migration", "applied successfully")
	}

	// idempotencyStore is wired for the first command handler that needs it;
	// no route uses it yet (Section 5's OTP verify/session-create is
	// naturally idempotent-safe via its own atomic consume/version guards —
	// see internal/features/auth/usecases — rather than the Idempotency-Key
	// header, matching hauling/dispatch's identical precedent for login
	// flows).
	idempotencyStore := idempotency.NewStore(db, idempotency.DefaultExpiry)

	// outbox is wired but has no configured Publisher yet: DisabledPublisher
	// keeps any future Append()'d event durably PENDING (never silently
	// dropped) until a real destination is chosen for that event type. See
	// internal/events and docs/architecture/08_EVENT_OWNERSHIP.md. Section 5's
	// staff lifecycle events are appended to this same outbox (see
	// staffEventAdapter in adapters.go) — durable now, delivered once a real
	// destination/consumer exists.
	outboxRepo := events.NewOutboxRepository(db)
	outboxWorker := events.NewWorker(outboxRepo, events.DisabledPublisher{}, events.WorkerConfig{})
	_ = outboxWorker // started only once a real publisher and producer exist

	// inboxRepo dedupes events vendor-service consumes from other services;
	// wired for the first real event consumer to use.
	_ = events.NewInboxRepository(db)

	// ─── Section 5: Authentication, Sessions and Staff IAM ──────────────────

	// Repositories.
	userRepo := vendorauthrepositories.NewUserRepository(db)
	challengeRepo := vendorauthrepositories.NewOTPChallengeRepository(db)
	sessionRepo := vendorauthrepositories.NewSessionRepository(db)
	stepUpRepo := vendorauthrepositories.NewStepUpRepository(db)

	staffRepo := staffrepositories.NewStaffRepository(db)
	roleRepo := staffrepositories.NewRoleRepository(db)
	permissionRepo := staffrepositories.NewPermissionRepository(db)
	assignmentRepo := staffrepositories.NewAssignmentRepository(db)

	auditRecorder := audit.NewPostgresRecorder(db)
	evaluator := staffusecases.NewAuthorizationEvaluator(assignmentRepo)
	vendorService := vendors.NewService(db, vendorAuthorizationAdapter{evaluator: evaluator}, vendorAuditAdapter{recorder: auditRecorder}, events.NewDomainPublisher(outboxRepo))

	// notificationClient delivers AUTHENTICATION OTPs via notification-service
	// (email only today — see internal/features/auth/clients, phone has no
	// real delivery channel anywhere in the platform yet).
	notificationClient := notifications.Client{
		BaseURL: cfg.NotificationURL, ServiceName: "vendor-service", Secret: []byte(cfg.NotificationSecret),
	}
	var otpNotifier vendorauthusecases.NotificationSender
	if cfg.NotificationURL != "" {
		otpNotifier = vendorauthclients.NewOTPNotifier(notificationClient)
	}

	authService := vendorauthusecases.NewAuthService(vendorauthusecases.Options{
		Users: userRepo, Challenges: challengeRepo, Sessions: sessionRepo, StepUps: stepUpRepo,
		Staff: staffResolverAdapter{staff: staffRepo}, Notifier: otpNotifier,

		AccessTokenSecret: cfg.Auth.AccessTokenSecret, RefreshTokenSecret: cfg.Auth.RefreshTokenSecret, OTPSecret: cfg.Auth.OTPSecret,
		AccessTokenTTL: cfg.Auth.AccessTokenTTL, RefreshTokenTTL: cfg.Auth.RefreshTokenTTL,
		OTPTTL: cfg.Auth.OTPTTL, OTPMaxAttempts: cfg.Auth.OTPMaxAttempts, OTPMaxResends: cfg.Auth.OTPMaxResends,
		StepUpTTL: cfg.Auth.StepUpTTL, OTPDebug: cfg.Auth.OTPDebug,
	})
	var verificationAuthority vendors.VerificationAuthority
	if cfg.Flags.VendorVerificationIntegrationEnabled {
		verificationAuthority = verificationAuthorityAdapter{client: verificationintegration.NewClient(cfg.VerificationURL, cfg.VerificationSecret, cfg.InternalServiceCallTimeout)}
	}
	var bankAuthority vendors.BankAuthority
	if cfg.WalletURL != "" && cfg.WalletSecret != "" {
		bankAuthority = bankAuthorityAdapter{client: walletintegration.NewClient(cfg.WalletURL, cfg.WalletSecret, cfg.InternalServiceCallTimeout)}
	}
	var mediaAuthority vendors.MediaAuthority
	if cfg.MediaURL != "" && cfg.MediaSecret != "" {
		mediaAuthority = mediaAuthorityAdapter{client: mediaintegration.NewClient(cfg.MediaURL, cfg.MediaSecret, cfg.InternalServiceCallTimeout)}
	}
	vendorService.SetOnboardingDependencies(verificationAuthority, bankAuthority, mediaAuthority, authService)

	staffEvents := staffEventAdapter{outbox: outboxRepo, db: db}
	staffService := staffusecases.NewStaffService(staffusecases.StaffServiceOptions{
		Users: userResolverAdapter{users: userRepo}, Staff: staffRepo, Roles: roleRepo, Assignments: assignmentRepo,
		Evaluator: evaluator, Audit: auditAdapter{recorder: auditRecorder}, Events: staffEvents, Branches: vendorService,
	})
	roleService := staffusecases.NewRoleService(staffusecases.RoleServiceOptions{
		Roles: roleRepo, Permissions: permissionRepo, Assignments: assignmentRepo, Evaluator: evaluator,
		StepUp: authService, // *AuthService implements StepUpConsumer directly (ConsumeGrant)
		Audit:  auditAdapter{recorder: auditRecorder}, Events: staffEvents, Branches: vendorService,
	})

	limiter := ratelimit.New(redisClient, cfg.Auth.RedisKeyPrefix)

	authHandler := vendorauthhttp.NewHandler(authService, limiter, vendorauthhttp.RateLimits{
		StartMax: cfg.Auth.RateLimits.StartMax, StartWindowSeconds: cfg.Auth.RateLimits.StartWindowSeconds,
		ResendMax: cfg.Auth.RateLimits.ResendMax, ResendWindowSeconds: cfg.Auth.RateLimits.ResendWindowSeconds,
		VerifyMax: cfg.Auth.RateLimits.VerifyMax, VerifyWindowSeconds: cfg.Auth.RateLimits.VerifyWindowSeconds,
		RefreshMax: cfg.Auth.RateLimits.RefreshMax, RefreshWindowSeconds: cfg.Auth.RateLimits.RefreshWindowSeconds,
		StepUpStartMax: cfg.Auth.RateLimits.StepUpStartMax, StepUpStartWindowSeconds: cfg.Auth.RateLimits.StepUpStartWindowSeconds,
		StepUpVerifyMax: cfg.Auth.RateLimits.StepUpVerifyMax, StepUpVerifyWindowSeconds: cfg.Auth.RateLimits.StepUpVerifyWindowSeconds,
	})
	staffHandler := staffhttp.NewHandler(staffService, roleService, authService.RevokeAllSessionsForStaff)
	vendorHandler := vendors.NewHandler(vendorService, idempotencyStore, cfg.Flags.VendorOnboardingWritesEnabled, cfg.Flags.CommerceGlobalKillSwitch)

	accessSigner := sharedauth.NewTokenSigner(cfg.Auth.AccessTokenSecret)
	inboundVerifier := serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute)
	workers := []func(context.Context) error{}
	expiryJob := jobs.Job{Name: "verification-expiry", Interval: time.Minute, Pass: func(runCtx context.Context) error {
		_, err := vendorService.ExpireVerifications(runCtx, 100)
		logging.Notice("section7_metrics", "%s", vendorService.Section7MetricsJSON())
		return err
	}}
	workers = append(workers, func(runCtx context.Context) error {
		expiryJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
		return nil
	})
	if cfg.Flags.VendorVerificationIntegrationEnabled {
		reconcileJob := jobs.Job{Name: "verification-reconciliation", Interval: 15 * time.Second, Pass: func(runCtx context.Context) error {
			return vendorService.ReconcileVerifications(runCtx, "vendor-service-reconciler", 25)
		}}
		workers = append(workers, func(runCtx context.Context) error {
			reconcileJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
	}

	serviceapp.Run(serviceapp.Options{
		Name:        "vendor-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/vendor",
		Capabilities: []string{
			"vendor authentication: passwordless OTP login, sessions, step-up",
			"vendor staff IAM: staff provisioning/suspension, roles, permissions, assignments",
			"vendor business, brands, capabilities, branches and operating configuration",
			"restaurant/cloud-kitchen/pastry/shop catalogues (planned)",
			"CRM, inventory, purchasing, recipes/production (planned)",
			"commerce cart/checkout and vendor orders (planned)",
			"promotions and vendor-owned delivery (planned)",
			"commerce fulfilment planning (planned)",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
			func(ctx context.Context) error {
				return redisClient.Ping(ctx).Err()
			},
		},
		Register: func(group *gin.RouterGroup) {
			vendorauthhttp.RegisterRoutes(group, authHandler, accessSigner, sessionRepo, staffResolverAdapter{staff: staffRepo})

			protected := group.Group("")
			protected.Use(vendorauthhttp.RequireAuth(accessSigner, sessionRepo, staffResolverAdapter{staff: staffRepo}))
			staffhttp.RegisterRoutes(protected, staffHandler)
			vendors.RegisterRoutes(protected, vendorHandler)
			vendors.RegisterOnboardingRoutes(protected, vendorHandler)

			staffhttp.RegisterInternalRoutes(group, staffService, inboundVerifier)
		},
		RegisterRoot: func(router *gin.Engine) {
			vendors.RegisterInternalOnboardingRoutes(router, vendorHandler, inboundVerifier)
		},
		Workers: workers,
	})
}

func logConnectivity(ctx context.Context, redisAddr string, db interface{ Ping(context.Context) error }, redisClient interface {
	Ping(context.Context) *redis.StatusCmd
}) {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.Ping(pingCtx); err != nil {
		logging.Error("database", "connectivity check failed: %v", err)
	} else {
		logging.Success("database", "connected")
	}

	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		logging.Error("redis", "failed addr=%s err=%v", redisAddr, err)
	} else {
		logging.Success("redis", "connected addr=%s", redisAddr)
	}
}
