package main

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

	cloudkitchen "cosmicforge/logistics/services/vendor-service/internal/features/cloudkitchen"
	cost "cosmicforge/logistics/services/vendor-service/internal/features/cost"
	crm "cosmicforge/logistics/services/vendor-service/internal/features/crm"
	delivery "cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	food "cosmicforge/logistics/services/vendor-service/internal/features/food"
	inventory "cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	orders "cosmicforge/logistics/services/vendor-service/internal/features/orders"
	pastry "cosmicforge/logistics/services/vendor-service/internal/features/pastry"
	production "cosmicforge/logistics/services/vendor-service/internal/features/production"
	productionbatch "cosmicforge/logistics/services/vendor-service/internal/features/productionbatch"
	productionmaster "cosmicforge/logistics/services/vendor-service/internal/features/productionmaster"
	purchasing "cosmicforge/logistics/services/vendor-service/internal/features/purchasing"
	recipes "cosmicforge/logistics/services/vendor-service/internal/features/recipes"
	shop "cosmicforge/logistics/services/vendor-service/internal/features/shop"
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
	// naturally idempotent-safe via its own atomic consume/version guards -
	// see internal/features/auth/usecases - rather than the Idempotency-Key
	// header, matching hauling/dispatch's identical precedent for login
	// flows).
	idempotencyStore := idempotency.NewStore(db, idempotency.DefaultExpiry)

	// outbox is wired but has no configured Publisher yet: DisabledPublisher
	// keeps any future Append()'d event durably PENDING (never silently
	// dropped) until a real destination is chosen for that event type. See
	// internal/events and docs/architecture/08_EVENT_OWNERSHIP.md. Section 5's
	// staff lifecycle events are appended to this same outbox (see
	// staffEventAdapter in adapters.go) - durable now, delivered once a real
	// destination/consumer exists.
	outboxRepo := events.NewOutboxRepository(db)
	outboxWorker := events.NewWorker(outboxRepo, events.DisabledPublisher{}, events.WorkerConfig{})
	_ = outboxWorker // started only once a real publisher and producer exist

	// inboxRepo dedupes events vendor-service consumes from other services;
	// wired for the first real event consumer to use.
	_ = events.NewInboxRepository(db)

	// --- Section 5: Authentication, Sessions and Staff IAM ---

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
	// (email only today - see internal/features/auth/clients, phone has no
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

	// Section 8A CRM. Every external integration it can consume is disabled by
	// default because no authority publishes those events yet; with them off
	// the read surface still works and simply reports data as unavailable
	// rather than inventing it.
	crmFlags := crm.Flags{
		CRMEnabled:                    cfg.Flags.VendorCRMEnabled,
		WritesEnabled:                 cfg.Flags.VendorCRMWritesEnabled,
		CustomerProjectionEnabled:     cfg.Flags.VendorCRMCustomerProjectionEnabled,
		ConsentIntegrationEnabled:     cfg.Flags.VendorCRMConsentIntegrationEnabled,
		PrivacyIntegrationEnabled:     cfg.Flags.VendorCRMPrivacyIntegrationEnabled,
		SegmentationEnabled:           cfg.Flags.VendorCRMSegmentationEnabled,
		SegmentMaterializationEnabled: cfg.Flags.VendorCRMSegmentMaterializationEnabled,
		InteractionsEnabled:           cfg.Flags.VendorCRMInteractionsEnabled,
		TasksEnabled:                  cfg.Flags.VendorCRMTasksEnabled,
		LoyaltyEnabled:                cfg.Flags.VendorCRMLoyaltyEnabled,
		CampaignsEnabled:              cfg.Flags.VendorCRMCampaignsEnabled,
		CampaignPreparationEnabled:    cfg.Flags.VendorCRMCampaignPreparationEnabled,
		CampaignExecutionEnabled:      cfg.Flags.VendorCRMCampaignExecutionEnabled,
		CorporateEnabled:              cfg.Flags.VendorCRMCorporateEnabled,
		IntelligenceEnabled:           cfg.Flags.VendorCRMIntelligenceEnabled,
		IntelligenceWorkerEnabled:     cfg.Flags.VendorCRMIntelligenceWorkerEnabled,
		ExportsEnabled:                cfg.Flags.VendorCRMExportsEnabled,
		CommerceKillSwitch:            cfg.Flags.CommerceGlobalKillSwitch,
	}
	crmService := crm.NewService(crm.Options{
		DB:     db,
		Scopes: crmScopeAdapter{evaluator: evaluator},
		Audit:  crmAuditAdapter{recorder: auditRecorder},
		Events: events.NewDomainPublisher(outboxRepo),
		Flags:  crmFlags,
		Notifier: func() notifications.Notifier {
			if cfg.NotificationURL == "" || cfg.NotificationSecret == "" {
				return nil
			}
			return notificationClient
		}(),
		// Section 8H. A nil artifact store disables export generation and
		// download entirely rather than falling back to local files: the
		// alternative to a private store is no download at all.
		Artifacts: func() crm.ArtifactStore {
			if !cfg.Flags.VendorCRMExportsEnabled || cfg.MediaURL == "" || cfg.MediaSecret == "" {
				return nil
			}
			return mediaintegration.NewArtifactStore(cfg.MediaURL, cfg.MediaSecret,
				cfg.InternalServiceCallTimeout, 5*time.Minute)
		}(),
		// The same Section 5 step-up mechanism the IAM and onboarding surfaces
		// use. A nil consumer refuses export creation and download rather than
		// skipping the check.
		StepUp: authService,
	})
	crmHandler := crm.NewHandler(crmService)
	crmInternalHandler := crm.NewInternalHandler(crmService)
	// Section 8B commands reuse the same Idempotency-Key store as Sections 6/7,
	// so a retried bulk tag or segment creation replays rather than repeats.
	crmSegmentationHandler := crm.NewSegmentationHandler(crmService, idempotencyStore)
	crmActivityHandler := crm.NewActivityHandler(crmService, idempotencyStore, limiter)
	crmLoyaltyHandler := crm.NewLoyaltyHandler(crmService, idempotencyStore, limiter)
	crmCampaignHandler := crm.NewCampaignHandler(crmService, idempotencyStore, limiter)
	crmCorporateHandler := crm.NewCorporateHandler(crmService, idempotencyStore, limiter)
	crmIntelligenceHandler := crm.NewIntelligenceHandler(crmService, idempotencyStore)
	crmExportHandler := crm.NewExportHandler(crmService, idempotencyStore)

	// Section 9A food catalogue. Media validation reuses the same private-API
	// client Section 7 uses for onboarding documents; with no media credentials
	// configured the service refuses to attach a reference rather than storing
	// an unverified one.
	foodFlags := food.Flags{
		CatalogueEnabled:   cfg.Flags.CommerceFoodEnabled,
		WritesEnabled:      cfg.Flags.VendorFoodCatalogueWritesEnabled,
		CommerceKillSwitch: cfg.Flags.CommerceGlobalKillSwitch,
	}
	foodService := food.NewService(food.Options{
		DB:     db,
		Scopes: foodScopeAdapter{evaluator: evaluator},
		Audit:  foodAuditAdapter{recorder: auditRecorder},
		Events: events.NewDomainPublisher(outboxRepo),
		Flags:  foodFlags,
		Media: func() food.MediaAuthority {
			if cfg.MediaURL == "" || cfg.MediaSecret == "" {
				return nil
			}
			return foodMediaAdapter{client: mediaintegration.NewClient(cfg.MediaURL, cfg.MediaSecret, cfg.InternalServiceCallTimeout)}
		}(),
	})
	// Section 9A commands reuse the same Idempotency-Key store as Sections 6/7/8.
	foodHandler := food.NewHandler(foodService, idempotencyStore)
	productionFlags := production.Flags{ProductionEnabled: cfg.Flags.CommerceFoodEnabled, WritesEnabled: cfg.Flags.VendorFoodCatalogueWritesEnabled, CommerceKillSwitch: cfg.Flags.CommerceGlobalKillSwitch}
	productionService := production.NewService(production.Options{DB: db, Scopes: productionScopeAdapter{evaluator: evaluator}, Audit: productionAuditAdapter{recorder: auditRecorder}, Events: events.NewDomainPublisher(outboxRepo), Flags: productionFlags})
	productionHandler := production.NewHandler(productionService).UseIdempotency(idempotencyStore)
	cloudKitchenService := cloudkitchen.NewService(db, productionService)
	cloudKitchenHandler := cloudkitchen.NewHandler(cloudKitchenService).UseIdempotency(idempotencyStore)
	pastryService := pastry.NewService(db, productionService)
	pastryHandler := pastry.NewHandler(pastryService, authService).UseIdempotency(idempotencyStore)
	shopFlags := shop.Flags{
		ShopEnabled:        cfg.Flags.CommerceShopEnabled,
		WritesEnabled:      cfg.Flags.VendorShopCatalogueWritesEnabled,
		CommerceKillSwitch: cfg.Flags.CommerceGlobalKillSwitch,
	}
	shopService := shop.NewService(shop.Options{
		DB: db, Scopes: shopScopeAdapter{evaluator: evaluator},
		Audit: shopAuditAdapter{recorder: auditRecorder}, Events: events.NewDomainPublisher(outboxRepo),
		Flags: shopFlags,
		Media: func() shop.MediaAuthority {
			if cfg.MediaURL == "" || cfg.MediaSecret == "" {
				return nil
			}
			return shopMediaAdapter{client: mediaintegration.NewClient(cfg.MediaURL, cfg.MediaSecret, cfg.InternalServiceCallTimeout)}
		}(),
	})
	shopHandler := shop.NewHandler(shopService, idempotencyStore)
	inventoryFlags := inventory.Flags{
		Enabled:       cfg.Flags.VendorInventoryEnabled,
		WritesEnabled: cfg.Flags.VendorInventoryWritesEnabled,
	}

	inventoryFoundation := inventory.NewService(
		inventory.Dependencies{
			Flags: inventoryFlags,
		},
	)

	inventoryStockService := inventory.NewStockService(
		inventory.StockOptions{
			DB:         db,
			Foundation: inventoryFoundation,
			Transactions: inventory.NewPostgresTransactionBoundary(
				db,
			),
			Evidence: inventory.NewPostgresEvidenceRecorder(),
			Events: inventoryEventAdapter{
				outbox: outboxRepo,
			},
		},
	)

	inventoryHandler := inventory.NewHandler(
		inventoryStockService,
		inventoryScopeAdapter{
			evaluator: evaluator,
		},
	)

	purchasingService := purchasing.NewService(
		purchasing.Options{
			DB: db,

			Scopes: purchasingScopeAdapter{
				evaluator: evaluator,
			},

			Events: events.NewDomainPublisher(
				outboxRepo,
			),

			Enabled: cfg.Flags.VendorPurchasingEnabled,

			WritesEnabled: cfg.Flags.VendorPurchasingEnabled,
		},
	)

	purchasingHandler := purchasing.NewHandler(
		purchasingService,
		idempotencyStore,
	)
	purchaseOrderService :=
		purchasing.NewPurchaseOrderService(
			purchasingService,
			authService,
		)

	purchaseOrderHandler :=
		purchasing.NewPurchaseOrderHandler(
			purchaseOrderService,
			purchasingHandler,
			idempotencyStore,
		)
	// Section 13: canonical taxonomy and Food/Shop search. Assembled in
	// section13.go so the layering of the search stack is documented where it
	// is built rather than buried in this function.
	section13Components, err := buildSection13(cfg, db, redisClient, evaluator, outboxRepo, uuid.NewString())
	if err != nil {
		logging.Fatal("section13", "assemble taxonomy and search: %v", err)
	}

	goodsReceivingService := purchasing.NewGoodsReceivingService(
		db,
		purchaseOrderService,
		inventoryStockService,
		events.NewDomainPublisher(outboxRepo),
	)

	// Section 15D operational surface. It reuses the purchasing service's
	// own multi-branch scope resolver rather than the inventory one,
	// because a dashboard spans every branch the operator holds.
	purchasingOperationsHandler := purchasing.NewOperationsHandler(
		goodsReceivingService,
		purchasingService,
	)

	goodsReceivingHandler := purchasing.NewGoodsReceivingHandler(
		goodsReceivingService,
		inventoryScopeAdapter{
			evaluator: evaluator,
		},
		idempotencyStore,
	)
	productionMasterService := productionmaster.NewService(
		db,
		events.NewDomainPublisher(outboxRepo),
	)

	productionMasterHandler := productionmaster.NewHandler(
		productionMasterService,
		inventoryScopeAdapter{
			evaluator: evaluator,
		},
		idempotencyStore,
	)
	recipeService := recipes.NewService(
		db,
		events.NewDomainPublisher(outboxRepo),
	)

	recipeHandler := recipes.NewHandler(
		recipeService,
		inventoryScopeAdapter{
			evaluator: evaluator,
		},
		idempotencyStore,
	)

	productionBatchService := productionbatch.NewService(
		db,
		productionService,
		inventoryStockService,
	)
	productionBatchHandler := productionbatch.NewHandler(
		productionBatchService,
		idempotencyStore,
	)

	// Section 16D operational cost. The purchasing reader is the ONLY link to
	// Section 15: cost is read from vendor_purchase_cost_history, never copied
	// into a second ledger.
	purchasingCostReader := cost.NewPostgresPurchasingCostReader(db)
	costResolver := cost.NewResolver(db, purchasingCostReader)
	costEngine := cost.NewEngine(db, costResolver)
	costService := cost.NewService(
		db,
		productionService,
		costEngine,
		costResolver,
	)
	costHandler := cost.NewHandler(
		costService,
		idempotencyStore,
	)

	// Section 17C (slice 1) Vendor Own Delivery policy. It is deliberately
	// composed with no Dispatch/Hauling/Taxi dependency: vendor-owned delivery
	// is independent of platform logistics, and wiring a logistics client in
	// here would be the first step towards confusing the two.
	deliveryService := delivery.NewService(
		db,
		productionService,
		cfg.Flags.VendorOwnDeliveryEnabled,
	)
	deliveryHandler := delivery.NewHandler(deliveryService)
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
	durableInventoryCoordinator := jobs.NewDurableCoordinator(db)
	durableInventoryWorkerOwner := "vendor-service-inventory-" + uuid.NewString()

	if cfg.Flags.CommerceFulfilmentWorkersEnabled && cfg.Flags.VendorInventoryEnabled {
		genericInventoryJobs := []jobs.DurableJob{
			{
				Job: jobs.Job{
					Name:     "inventory-reservation-expiry",
					Interval: 30 * time.Second,
					Pass: func(runCtx context.Context) error {
						_, err := inventoryStockService.ExpireDueReservations(runCtx, 200)
						return err
					},
				},
				Coordinator: durableInventoryCoordinator,
				Owner:       durableInventoryWorkerOwner,
				LeaseFor:    2 * time.Minute,
				MaxAttempts: 5,
				RetryBase:   5 * time.Second,
			},
			{
				Job: jobs.Job{
					Name:     "inventory-lot-expiry",
					Interval: 30 * time.Second,
					Pass: func(runCtx context.Context) error {
						_, err := inventoryStockService.ExpireDueLots(runCtx, 200)
						return err
					},
				},
				Coordinator: durableInventoryCoordinator,
				Owner:       durableInventoryWorkerOwner,
				LeaseFor:    2 * time.Minute,
				MaxAttempts: 5,
				RetryBase:   5 * time.Second,
			},
			{
				Job: jobs.Job{
					Name:     "inventory-stock-alerts",
					Interval: time.Minute,
					Pass: func(runCtx context.Context) error {
						_, err := inventoryStockService.GenerateInventoryAlerts(runCtx, 200)
						return err
					},
				},
				Coordinator: durableInventoryCoordinator,
				Owner:       durableInventoryWorkerOwner,
				LeaseFor:    2 * time.Minute,
				MaxAttempts: 5,
				RetryBase:   5 * time.Second,
			},
			{
				Job: jobs.Job{
					Name:     "inventory-balance-reconciliation",
					Interval: time.Minute,
					Pass: func(runCtx context.Context) error {
						_, err := inventoryStockService.ReconcileBalances(runCtx, 200)
						return err
					},
				},
				Coordinator: durableInventoryCoordinator,
				Owner:       durableInventoryWorkerOwner,
				LeaseFor:    2 * time.Minute,
				MaxAttempts: 5,
				RetryBase:   5 * time.Second,
			},
			{
				Job: jobs.Job{
					Name:     "inventory-backorder-reconciliation",
					Interval: time.Minute,
					Pass: func(runCtx context.Context) error {
						_, err := inventoryStockService.ReconcileBackorders(runCtx, 200)
						return err
					},
				},
				Coordinator: durableInventoryCoordinator,
				Owner:       durableInventoryWorkerOwner,
				LeaseFor:    2 * time.Minute,
				MaxAttempts: 5,
				RetryBase:   5 * time.Second,
			},
		}

		// Section 15D purchasing sweeps. Same coordinator, same lease,
		// retry and dead-letter behaviour as every other durable job; both
		// sweeps are idempotent because the alert store is keyed on
		// (business_id, dedupe_key).
		if cfg.Flags.VendorPurchasingEnabled {
			genericInventoryJobs = append(
				genericInventoryJobs,
				jobs.DurableJob{
					Job: jobs.Job{
						Name:     "purchasing-delivery-alerts",
						Interval: 5 * time.Minute,
						Pass: func(runCtx context.Context) error {
							_, err := goodsReceivingService.SweepDeliveryAlerts(runCtx, 200)
							return err
						},
					},
					Coordinator: durableInventoryCoordinator,
					Owner:       durableInventoryWorkerOwner,
					LeaseFor:    2 * time.Minute,
					MaxAttempts: 5,
					RetryBase:   5 * time.Second,
				},
				jobs.DurableJob{
					Job: jobs.Job{
						Name:     "purchasing-discrepancy-escalation",
						Interval: 15 * time.Minute,
						Pass: func(runCtx context.Context) error {
							_, err := goodsReceivingService.SweepDiscrepancyEscalations(runCtx, 200)
							return err
						},
					},
					Coordinator: durableInventoryCoordinator,
					Owner:       durableInventoryWorkerOwner,
					LeaseFor:    5 * time.Minute,
					MaxAttempts: 5,
					RetryBase:   30 * time.Second,
				},
			)
		}

		for _, configured := range genericInventoryJobs {
			job := configured

			workers = append(
				workers,
				func(runCtx context.Context) error {
					job.Run(
						runCtx,
						func(name string, err error) {
							logging.Error("worker", "%s: %v", name, err)
						},
					)

					return nil
				},
			)
		}
	}

	// Section 12 Shop operational tables remain live during the compatibility
	// bridge, so their existing passes remain scheduled through the same
	// durable coordinator.
	if cfg.Flags.CommerceFulfilmentWorkersEnabled && cfg.Flags.CommerceShopEnabled {
		shopInventoryCompatibilityJobs := []jobs.DurableJob{
			{
				Job: jobs.Job{
					Name:     "shop-inventory-expiry-compat",
					Interval: 30 * time.Second,
					Pass: func(runCtx context.Context) error {
						if _, err := shopService.ReconcileExpiredReservations(runCtx, 200); err != nil {
							return err
						}

						_, err := shopService.ReconcileExpiredLots(runCtx, 200)
						return err
					},
				},
				Coordinator: durableInventoryCoordinator,
				Owner:       durableInventoryWorkerOwner,
				LeaseFor:    2 * time.Minute,
				MaxAttempts: 5,
				RetryBase:   5 * time.Second,
			},
			{
				Job: jobs.Job{
					Name:     "shop-inventory-alerts-compat",
					Interval: time.Minute,
					Pass: func(runCtx context.Context) error {
						_, err := shopService.GenerateInventoryAlerts(runCtx, 200)
						return err
					},
				},
				Coordinator: durableInventoryCoordinator,
				Owner:       durableInventoryWorkerOwner,
				LeaseFor:    2 * time.Minute,
				MaxAttempts: 5,
				RetryBase:   5 * time.Second,
			},
			{
				Job: jobs.Job{
					Name:     "shop-inventory-bulk-operations",
					Interval: 15 * time.Second,
					Pass: func(runCtx context.Context) error {
						_, err := shopService.RunBulkOperationPass(runCtx, 10)
						return err
					},
				},
				Coordinator: durableInventoryCoordinator,
				Owner:       durableInventoryWorkerOwner,
				LeaseFor:    2 * time.Minute,
				MaxAttempts: 5,
				RetryBase:   5 * time.Second,
			},
		}

		for _, configured := range shopInventoryCompatibilityJobs {
			job := configured

			workers = append(
				workers,
				func(runCtx context.Context) error {
					job.Run(
						runCtx,
						func(name string, err error) {
							logging.Error("worker", "%s: %v", name, err)
						},
					)

					return nil
				},
			)
		}
	}
	// Section 17B checkout recovery. It releases stock a lapsed or crashed
	// checkout is still holding and hands the cart back to its customer.
	// Without it, a process that died mid-reservation would cost a vendor
	// real sellable stock until the reservation's own TTL elapsed.
	checkoutService := buildSection17BCheckoutService(
		cfg,
		db,
		foodService,
		shopService,
		inventoryStockService,
	)

	// Section 17C fulfilment planning. It consumes validated Section 17B
	// groups and the vendor's own delivery policy, and calls the logistics
	// authorities for real quotes. Composed with no Wallet, order or payment
	// dependency: 17C plans and quotes, it moves no money.
	fulfilmentService := buildSection17CFulfilmentService(
		cfg,
		db,
		deliveryService,
	)

	// Section 17D immutable quote lock. It freezes current checkout, product,
	// fulfilment and funding evidence but performs no payment or order work.
	quoteService := buildSection17DQuoteService(cfg, db)

	// Section 17E order orchestration. It cuts one ParentOrder and its
	// isolated VendorOrders from an approved quote. Composed with no Wallet
	// or mission-execution dependency: it creates orders, not payments.
	orderService := buildSection17EOrderService(cfg, db, evaluator, inventoryStockService, quoteService, events.NewDomainPublisher(outboxRepo))

	// Section 17F durable payment recovery. Without it, a process that died
	// mid-authorization would leave a leg whose outcome nobody ever resolves,
	// which is how a paid checkout gets silently lost.
	if recoveryJob, ok := section17FRecoveryJob(
		cfg,
		orderService,
		durableInventoryCoordinator,
		durableInventoryWorkerOwner,
	); ok {
		job := recoveryJob

		workers = append(
			workers,
			func(runCtx context.Context) error {
				job.Run(
					runCtx,
					func(name string, err error) {
						logging.Error("worker", "%s: %v", name, err)
					},
				)

				return nil
			},
		)
	}

	if expiryJob, ok := section17CExpiryJob(
		cfg,
		fulfilmentService,
		durableInventoryCoordinator,
		durableInventoryWorkerOwner,
	); ok {
		job := expiryJob

		workers = append(
			workers,
			func(runCtx context.Context) error {
				job.Run(
					runCtx,
					func(name string, err error) {
						logging.Error("worker", "%s: %v", name, err)
					},
				)

				return nil
			},
		)
	}

	if recoveryJob, ok := section17BRecoveryJob(
		cfg,
		checkoutService,
		durableInventoryCoordinator,
		durableInventoryWorkerOwner,
	); ok {
		job := recoveryJob

		workers = append(
			workers,
			func(runCtx context.Context) error {
				job.Run(
					runCtx,
					func(name string, err error) {
						logging.Error("worker", "%s: %v", name, err)
					},
				)

				return nil
			},
		)
	}

	if crmFlags.SegmentationEnabled && crmFlags.SegmentMaterializationEnabled {
		// The segment materializer claims due evaluations under a lease, so
		// running several instances is safe: SKIP LOCKED means two workers never
		// evaluate the same segment, and a worker that dies simply lets its
		// lease lapse for another to pick the run up.
		workerID := "vendor-service-segment-materializer-" + uuid.NewString()
		materializeJob := jobs.Job{Name: "crm-segment-materializer", Interval: 10 * time.Second, Pass: func(runCtx context.Context) error {
			_, err := crmService.RunMaterializationPass(runCtx, workerID, 5)
			if depth, depthErr := crmService.StaleSegmentBacklog(runCtx); depthErr == nil {
				logging.Notice("crm_metrics", `{"crm_segment_materialization_queue_depth":%d}`, depth)
			}
			return err
		}}
		workers = append(workers, func(runCtx context.Context) error {
			materializeJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
	}

	if crmFlags.LoyaltyEnabled {
		// The loyalty expiry sweeper needs no queue table and no lease: a lot
		// with points left and a past expiry IS the durable job, and it stays
		// due until a committed transaction settles it. Several instances are
		// safe (SKIP LOCKED on the account row), a crash mid-batch simply
		// leaves the work due, and an outage delays bookkeeping without ever
		// letting an expired point be spent - every balance command settles
		// expiry itself first.
		loyaltyExpiryJob := jobs.Job{Name: "crm-loyalty-expiry", Interval: time.Minute, Pass: func(runCtx context.Context) error {
			_, err := crmService.RunLoyaltyExpiryPass(runCtx, crm.LoyaltyExpiryBatchSize)
			if due, dueErr := crmService.DueExpiryLotCount(runCtx); dueErr == nil {
				logging.Notice("crm_metrics", `{"crm_loyalty_expiration_due_lots":%d}`, due)
			}
			return err
		}}
		workers = append(workers, func(runCtx context.Context) error {
			loyaltyExpiryJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
	}

	if crmFlags.CampaignsEnabled && crmFlags.CampaignPreparationEnabled {
		// The audience preparer claims due snapshots under a lease, so running
		// several instances is safe: SKIP LOCKED means two workers never build
		// the same audience, and a worker that dies simply lets its lease lapse
		// for another to adopt. It prepares audiences and nothing else - it
		// never contacts notification-service, because Section 8E does not
		// deliver anything.
		workerID := "vendor-service-campaign-preparer-" + uuid.NewString()
		prepareJob := jobs.Job{Name: "crm-campaign-preparer", Interval: 10 * time.Second, Pass: func(runCtx context.Context) error {
			_, err := crmService.RunCampaignPreparationPass(runCtx, workerID, 5)
			if depth, depthErr := crmService.CampaignPreparationBacklog(runCtx); depthErr == nil {
				logging.Notice("crm_metrics", `{"crm_campaign_preparation_queue_depth":%d}`, depth)
			}
			return err
		}}
		workers = append(workers, func(runCtx context.Context) error {
			prepareJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
	}
	if crmFlags.IntelligenceEnabled && crmFlags.IntelligenceWorkerEnabled {
		// The materializer claims evaluations under a lease, so running several
		// instances is safe. It builds a generation alongside the live one and
		// swaps atomically: a worker that dies leaves the previous scores
		// exactly as they were rather than emptying a dashboard.
		workerID := "vendor-service-intelligence-materializer-" + uuid.NewString()
		materializer := jobs.Job{Name: "crm-intelligence-materializer", Interval: 15 * time.Second,
			Pass: func(runCtx context.Context) error {
				_, err := crmService.RunIntelligenceMaterializer(runCtx, workerID, 3)
				return err
			}}
		workers = append(workers, func(runCtx context.Context) error {
			materializer.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
	}
	if crmFlags.ExportsEnabled {
		// Two jobs: one generates artifacts, one retires them. Retirement is
		// separate and unconditional because an artifact whose retention has
		// elapsed must stop being downloadable even if generation is wedged.
		workerID := "vendor-service-export-generator-" + uuid.NewString()
		generator := jobs.Job{Name: "crm-export-generator", Interval: 5 * time.Second,
			Pass: func(runCtx context.Context) error {
				_, err := crmService.RunExportGenerator(runCtx, workerID, 2)
				return err
			}}
		retention := jobs.Job{Name: "crm-export-retention", Interval: time.Minute,
			Pass: func(runCtx context.Context) error {
				_, err := crmService.ExportRetentionPass(runCtx, 50)
				return err
			}}
		workers = append(workers, func(runCtx context.Context) error {
			generator.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
		workers = append(workers, func(runCtx context.Context) error {
			retention.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
	}
	if crmFlags.CampaignsEnabled && crmFlags.CampaignExecutionEnabled {
		scheduler := jobs.Job{Name: "crm-campaign-scheduler", Interval: 5 * time.Second, Pass: func(runCtx context.Context) error {
			_, err := crmService.RunCampaignSchedulerPass(runCtx, 20)
			return err
		}}
		dispatcher := jobs.Job{Name: "crm-campaign-dispatcher", Interval: time.Second, Pass: func(runCtx context.Context) error {
			_, err := crmService.RunCampaignDeliveryPass(runCtx, 100)
			return err
		}}
		workers = append(workers, func(runCtx context.Context) error {
			scheduler.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
		workers = append(workers, func(runCtx context.Context) error {
			dispatcher.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
	}
	if productionFlags.ProductionEnabled {
		// Both passes are safe to run on every instance. Corporate occurrence
		// uniqueness and quote status/expiry are durable idempotency boundaries.
		corporateWorkerID := "vendor-cloud-kitchen-materializer-" + uuid.NewString()
		corporateWorker := jobs.Job{Name: "cloud-kitchen-corporate-materializer", Interval: 30 * time.Second, Pass: func(runCtx context.Context) error {
			if _, err := cloudKitchenService.MaterializeAll(runCtx, 100); err != nil {
				return err
			}
			_, err := cloudKitchenService.RunOccurrenceRecoveryPass(runCtx, corporateWorkerID, 100)
			return err
		}}
		quoteExpiryWorker := jobs.Job{Name: "pastry-quote-expiry", Interval: time.Minute, Pass: func(runCtx context.Context) error {
			if _, err := pastryService.ExpireQuotes(runCtx, 100); err != nil {
				return err
			}
			_, err := productionService.ExpireHolds(runCtx, 100)
			return err
		}}
		productionAlertWorker := jobs.Job{Name: "production-operational-alerts", Interval: time.Minute, Pass: func(runCtx context.Context) error {
			_, err := productionService.GenerateOperationalAlerts(runCtx, 200)
			return err
		}}
		workers = append(workers,
			func(runCtx context.Context) error {
				corporateWorker.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
				return nil
			},
			func(runCtx context.Context) error {
				quoteExpiryWorker.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
				return nil
			},
			func(runCtx context.Context) error {
				productionAlertWorker.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
				return nil
			},
		)
	}

	// Section 13: the rebuild and validate/activate workers. They run through
	// the same jobs.Job runtime as every other Vendor worker - Section 13
	// introduces no second scheduler.
	for _, job := range section13Components.Workers() {
		searchJob := job
		workers = append(workers, func(runCtx context.Context) error {
			searchJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
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
			"restaurant, cloud-kitchen, pastry and Shop management catalogues",
			"CRM and shared production capacity; inventory, suppliers and purchase-order workflow active, recipes planned",
			"commerce cart/checkout and vendor orders (planned)",
			"promotions and vendor-owned delivery (planned)",
			"commerce fulfilment planning (planned)",
			"canonical Food/Shop taxonomy: versions, vocabulary, mapping and governance",
			"Food and Shop search projections: ranking, filters, facets, suggestions",
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
			crm.RegisterRoutes(protected, crmHandler, crmFlags)
			crm.RegisterSegmentationRoutes(protected, crmSegmentationHandler, crmFlags)
			crm.RegisterActivityRoutes(protected, crmActivityHandler, crmFlags)
			crm.RegisterLoyaltyRoutes(protected, crmLoyaltyHandler, crmFlags)
			crm.RegisterLoyaltyTierRoutes(protected, crmLoyaltyHandler, crmFlags)
			crm.RegisterCampaignRoutes(protected, crmCampaignHandler, crmFlags)
			crm.RegisterCampaignExecutionRoutes(protected, crmCampaignHandler, crmFlags)
			crm.RegisterCorporateRoutes(protected, crmCorporateHandler, crmFlags)
			crm.RegisterIntelligenceRoutes(protected, crmIntelligenceHandler, crmFlags)
			crm.RegisterExportRoutes(protected, crmExportHandler, crmFlags)
			food.RegisterRoutes(protected, foodHandler, foodFlags)
			food.RegisterOperationsRoutes(protected, foodHandler, foodFlags)
			production.RegisterRoutes(protected, productionHandler, productionFlags)
			cloudkitchen.RegisterRoutes(protected, cloudKitchenHandler, productionFlags)
			pastry.RegisterRoutes(protected, pastryHandler, productionFlags)
			shop.RegisterRoutes(protected, shopHandler, shopFlags)
			foodService.SetInventoryAvailabilityAuthority(
				foodInventoryAvailabilityAdapter{
					inventory: inventoryStockService,
				},
			)

			inventory.RegisterRoutes(protected, inventoryHandler, inventoryFlags)
			inventory.RegisterReservationRoutes(protected, inventoryHandler, inventoryFlags)
			inventory.RegisterOperationalRoutes(protected, inventoryHandler, inventoryFlags)
			purchasing.RegisterRoutes(
				protected,
				purchasingHandler,
				cfg.Flags.VendorPurchasingEnabled,
			)
			purchasing.RegisterPurchaseOrderRoutes(
				protected,
				purchaseOrderHandler,
				cfg.Flags.VendorPurchasingEnabled,
			)
			purchasing.RegisterGoodsReceivingRoutes(
				protected,
				goodsReceivingHandler,
				cfg.Flags.VendorPurchasingEnabled,
			)
			productionmaster.RegisterRoutes(
				protected,
				productionMasterHandler,
			)
			recipes.RegisterRoutes(
				protected,
				recipeHandler,
				cfg.Flags.VendorRecipesEnabled,
			)

			productionbatch.RegisterRoutes(
				protected,
				productionBatchHandler,
				cfg.Flags.VendorRecipesEnabled,
			)

			cost.RegisterRoutes(
				protected,
				costHandler,
				cfg.Flags.VendorCostEnabled,
			)

			delivery.RegisterRoutes(
				protected,
				deliveryHandler,
				cfg.Flags.VendorOwnDeliveryEnabled,
			)

			purchasing.RegisterOperationsRoutes(protected, purchasingOperationsHandler, cfg.Flags.VendorPurchasingEnabled)
			section13Components.registerVendorRoutes(protected)

			// Section 17E vendor order queue and lifecycle commands.
			orders.RegisterVendorRoutes(
				protected,
				orderService,
				cfg.Flags.CommerceOrdersEnabled,
				idempotencyStore,
			)

		},
		RegisterRoot: func(router *gin.Engine) {
			registerSection17ACustomerCartRoutes(
				router,
				cfg,
				db,
				foodService,
				shopService,
				idempotencyStore,
			)
			registerSection17BCheckoutRoutes(
				router,
				cfg,
				checkoutService,
				idempotencyStore,
			)
			registerSection17CFulfilmentRoutes(
				router,
				cfg,
				fulfilmentService,
			)
			registerSection17DQuoteRoutes(
				router,
				cfg,
				quoteService,
				idempotencyStore,
			)
			registerSection17ECustomerOrderRoutes(
				router,
				cfg,
				orderService,
				idempotencyStore,
			)
			// Every internal-authority ingress is mounted here, at the engine
			// root, never under the public vendor base the gateway forwards.
			staffhttp.RegisterInternalRoutes(router, staffService, inboundVerifier)
			vendors.RegisterInternalOnboardingRoutes(router, vendorHandler, inboundVerifier)
			// Root-mounted, never under /api/v1/vendor: the CRM event ingress
			// carries consent and privacy authority and must not be reachable
			// through the public gateway.
			crm.RegisterInternalRoutes(router, crmInternalHandler, inboundVerifier, crmFlags)
			food.RegisterInternalRoutes(router, food.NewInternalHandler(foodService), inboundVerifier, foodFlags)
			shop.RegisterInternalInventoryRoutes(router, shop.NewInventoryInternalHandler(shopService, strings.Split(cfg.ShopInternalCallers, ",")), inboundVerifier, shopFlags)
			inventory.RegisterInternalRoutes(router, inventory.NewInternalInventoryHandler(inventoryStockService, strings.Split(cfg.InventoryInternalCallers, ",")), inboundVerifier, inventoryFlags)
			orders.RegisterInternalLogisticsRoutes(router, orderService, inboundVerifier)
			// Section 13 mounts two more internal-authority surfaces here: platform
			// taxonomy administration and internal Food/Shop search. Each carries its
			// own caller allowlist.
			section13Components.registerInternalRoutes(router, inboundVerifier)
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
