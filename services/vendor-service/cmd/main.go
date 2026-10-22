package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/analyticsproducer"
	"cosmicforge/logistics/services/vendor-service/internal/features/assets"
	"cosmicforge/logistics/services/vendor-service/internal/features/customershare"
	"cosmicforge/logistics/shared/go/journeyshare"
	"os"
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
	"cosmicforge/logistics/services/vendor-service/internal/privacy"
	sharedevents "cosmicforge/logistics/shared/go/events"

	vendorauthclients "cosmicforge/logistics/services/vendor-service/internal/features/auth/clients"
	vendorauthhttp "cosmicforge/logistics/services/vendor-service/internal/features/auth/http"
	vendorauthrepositories "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
	vendorauthusecases "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"

	cloudkitchen "cosmicforge/logistics/services/vendor-service/internal/features/cloud_kitchen"
	cost "cosmicforge/logistics/services/vendor-service/internal/features/cost"
	crm "cosmicforge/logistics/services/vendor-service/internal/features/crm"
	delivery "cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	financeprojection "cosmicforge/logistics/services/vendor-service/internal/features/finance_projection"
	food "cosmicforge/logistics/services/vendor-service/internal/features/food"
	fulfilmentgroup "cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	inventory "cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/features/orchestration"
	orders "cosmicforge/logistics/services/vendor-service/internal/features/orders"
	pastry "cosmicforge/logistics/services/vendor-service/internal/features/pastry"
	production "cosmicforge/logistics/services/vendor-service/internal/features/production"
	productionbatch "cosmicforge/logistics/services/vendor-service/internal/features/productionbatch"
	productionmaster "cosmicforge/logistics/services/vendor-service/internal/features/productionmaster"
	promotions "cosmicforge/logistics/services/vendor-service/internal/features/promotions"
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

	"cosmicforge/logistics/services/vendor-service/internal/features/registration"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/walletclient"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logging.Fatal("configuration", "%v", err)
	}
	logging.Notice("vendor-service", "migration=%t vendor_service_enabled=%t redis=%s",
		cfg.Migration, cfg.Flags.VendorServiceEnabled, cfg.Redis.Addr)
	logging.Notice("vendor-service", "process_mode=%s workers_enabled=%t kill_switch=%t",
		cfg.ProcessMode, cfg.ProcessMode != "api", cfg.Flags.CommerceGlobalKillSwitch)

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
	if err := requireWorkerDatabase(ctx, cfg.ProcessMode, db); err != nil {
		logging.Fatal("database", "worker startup readiness failed: %v", err)
	}
	// Admin Section 6: owner-signed global readiness coverage manifests. Off
	// unless VENDOR_ANALYTICS_PRODUCER_ENABLED is true.
	if coverageProducer, err := analyticsproducer.Runtime(ctx, db); err != nil {
		logging.Fatal("configuration", "Vendor analytics coverage producer configuration invalid")
	} else if coverageProducer != nil {
		go coverageProducer.Run(ctx, func(step string, err error) { logging.Error("vendor-analytics-coverage", "%s: %v", step, err) })
	}

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
		Registration: registration.NewService(db, cfg.Flags.VendorOnboardingWritesEnabled && !cfg.Flags.CommerceGlobalKillSwitch),

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
		mediaAuthority = mediaAuthorityAdapter{client: mediaintegration.NewClient(cfg.MediaURL, cfg.MediaSecret, cfg.InternalServiceCallTimeout).WithPublicURL(cfg.MediaPublicURL)}
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
	abuseLimiter := ratelimit.New(redisClient, cfg.AbuseProtection.RedisKeyPrefix)

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
	financeAuthority := vendorFinanceAuthority{client: walletclient.Client{
		BaseURL: strings.TrimRight(cfg.WalletURL, "/"), ServiceName: orders.PaymentSourceService, Secret: []byte(cfg.WalletSecret),
	}}
	financeService := financeprojection.NewService(financeAuthority, vendorAuthorizationAdapter{evaluator: evaluator}, authService)
	financeHandler := financeprojection.NewHandler(financeService)

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
	cloudKitchenHandler := cloudkitchen.NewHandler(cloudKitchenService).UseIdempotency(idempotencyStore).UseCRM(crmService)
	pastryService := pastry.NewService(db, productionService)
	// Production owns job status while Pastry owns custom-cake milestones and
	// commercial gating. This same-service adapter lets only explicitly mapped
	// final production progress advance Pastry's READY milestone.
	productionService.SetPastryMilestoneBridge(pastryService)
	pastryHandler := pastry.NewHandler(pastryService, authService).UseIdempotency(idempotencyStore).UseCRM(crmService)
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
	var assetMedia *mediaintegration.Client
	if cfg.MediaURL != "" && cfg.MediaSecret != "" && cfg.MediaPublicURL != "" {
		assetMedia = mediaintegration.NewClient(cfg.MediaURL, cfg.MediaSecret, cfg.InternalServiceCallTimeout).WithPublicURL(cfg.MediaPublicURL)
	}
	if assetMedia != nil {
		vendorService.UseAssetValidator(assetMedia.ValidateAsset)
		pastryService.UseAssetValidator(assetMedia.ValidateAsset)
	}
	assetHandler := assets.NewHandler(assets.Options{DB: db, Food: foodService, Shop: shopService, Vendors: vendorService, Production: productionService, Media: assetMedia,
		Enabled:  map[string]bool{"FOOD_ITEM": cfg.Flags.CommerceFoodEnabled, "SHOP_PRODUCT": cfg.Flags.CommerceShopEnabled, "BRAND": true, "PASTRY_REQUEST": cfg.Flags.CommerceFoodEnabled},
		Writable: map[string]bool{"FOOD_ITEM": cfg.Flags.VendorFoodCatalogueWritesEnabled && !cfg.Flags.CommerceGlobalKillSwitch, "SHOP_PRODUCT": cfg.Flags.VendorShopCatalogueWritesEnabled && !cfg.Flags.CommerceGlobalKillSwitch, "BRAND": !cfg.Flags.CommerceGlobalKillSwitch, "PASTRY_REQUEST": cfg.Flags.VendorFoodCatalogueWritesEnabled && !cfg.Flags.CommerceGlobalKillSwitch}})

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
	promotionService := promotions.NewService(db, productionService, cfg.Auth.OTPSecret, cfg.Flags.CommerceQuoteLockEnabled)
	promotionService.SetEventPublisher(events.NewDomainPublisher(outboxRepo))
	promotionHandler := promotions.NewHandler(promotionService, idempotencyStore)
	accessSigner := sharedauth.NewTokenSigner(cfg.Auth.AccessTokenSecret)
	inboundVerifier := serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute)
	workers := []func(context.Context) error{}
	promotionLifecycleJob := jobs.Job{Name: "promotion-lifecycle", Interval: time.Minute, Pass: func(runCtx context.Context) error {
		_, err := promotionService.MaintainLifecycle(runCtx, 200)
		return err
	}}
	workers = append(workers, func(runCtx context.Context) error {
		promotionLifecycleJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
		return nil
	})
	// Commerce operational notices use the existing transactional outbox and
	// Notification Service authority. Each controlled event stream has an
	// independent worker so unrelated Vendor events remain pending for their
	// actual consumers.
	if cfg.NotificationURL != "" && cfg.NotificationSecret != "" {
		publisher := events.NewNotificationPublisher(notifications.Client{
			BaseURL: cfg.NotificationURL, ServiceName: "vendor-service", Secret: []byte(cfg.NotificationSecret),
		})
		for _, eventType := range events.NotificationEventTypes() {
			eventType := eventType
			worker := events.NewWorker(outboxRepo, publisher, events.WorkerConfig{
				Owner: "vendor-commerce-notifications-" + strings.ReplaceAll(eventType, ".", "-"), EventType: eventType,
			})
			workers = append(workers, func(runCtx context.Context) error { return worker.Run(runCtx) })
		}
	}
	// Commerce grouping delivery to the customer context owner. Its own
	// worker, so an unreachable customer-service never holds up unrelated
	// Vendor events.
	if cfg.Flags.CustomerMovementGroupingEnabled {
		publisher, err := events.NewHTTPPublisher(cfg.CustomerURL, sharedevents.CustomerMovementGroupingPath,
			"vendor-service", []byte(cfg.CustomerSecret), 10*time.Second)
		if err == nil {
			worker := events.NewWorker(outboxRepo, customerGroupingHTTPPublisher{inner: publisher}, events.WorkerConfig{
				Owner: "vendor-customer-movement-grouping", EventType: sharedevents.CommerceGroupingEventName,
			})
			workers = append(workers, func(runCtx context.Context) error { return worker.Run(runCtx) })
			repair := &customerGroupingReconciler{publisher: &customerGroupingPublisher{db: db, outbox: outboxRepo}, baseURL: cfg.CustomerURL, secret: []byte(cfg.CustomerSecret)}
			workers = append(workers, repair.Run)
		}
		// Customer V4 §22 purchase Activity row: same flag, same Customer
		// endpoint and secret, its own worker and its own reconciliation.
		movementPublisher, err := events.NewHTTPPublisher(cfg.CustomerURL, sharedevents.CustomerMovementIngressPath,
			"vendor-service", []byte(cfg.CustomerSecret), 10*time.Second)
		if err == nil {
			worker := events.NewWorker(outboxRepo, customerOrderMovementHTTPPublisher{inner: movementPublisher}, events.WorkerConfig{
				Owner: "vendor-customer-order-movement", EventType: orders.CustomerOrderMovementEvent,
			})
			workers = append(workers, func(runCtx context.Context) error { return worker.Run(runCtx) })
			orderRepair := &customerOrderMovementReconciler{db: db, outbox: outboxRepo, baseURL: cfg.CustomerURL, secret: []byte(cfg.CustomerSecret)}
			workers = append(workers, orderRepair.Run)
		}
	}
	if cfg.Flags.CustomerGroupingBackfillEnabled {
		backfill := &customerGroupingPublisher{db: db, outbox: outboxRepo}
		workers = append(workers, func(ctx context.Context) error {
			runID := os.Getenv("VENDOR_CUSTOMER_GROUPING_BACKFILL_RUN_ID")
			dryRun := os.Getenv("VENDOR_CUSTOMER_GROUPING_BACKFILL_APPLY") != "true"
			cursor := ""
			for {
				_, next, done, err := backfill.BackfillBatch(ctx, runID, cursor, dryRun)
				if err != nil {
					return err
				}
				cursor = next
				if done {
					return nil
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
				}
			}
		})
	}
	if cfg.AnalyticsURL != "" && cfg.AnalyticsSecret != "" {
		// Other downstreams keep their own independently leased workers.
		publisher, err := events.NewHTTPPublisher(
			cfg.AnalyticsURL,
			"/internal/analytics/v1/commerce-events",
			"vendor-service",
			[]byte(cfg.AnalyticsSecret),
			10*time.Second,
		)
		if err == nil {
			// Section 13: readiness facts go to the network ingest, which
			// accepts this same envelope.
			if network, networkErr := events.NewHTTPPublisher(cfg.AnalyticsURL, networkanalytics.IngestPath,
				"vendor-service", []byte(cfg.AnalyticsSecret), 10*time.Second); networkErr == nil {
				worker := events.NewWorker(outboxRepo, network, events.WorkerConfig{
					Owner: "vendor-network-analytics-readiness", EventType: networkanalytics.EventBranchReadiness,
				})
				workers = append(workers, func(runCtx context.Context) error { return worker.Run(runCtx) })
			}
			for _, eventType := range events.AnalyticsEventTypes() {
				eventType := eventType
				worker := events.NewWorker(outboxRepo, publisher, events.WorkerConfig{
					Owner: "vendor-commerce-analytics-" + strings.ReplaceAll(eventType, ".", "-"), EventType: eventType,
				})
				workers = append(workers, func(runCtx context.Context) error { return worker.Run(runCtx) })
			}
		}
	}
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

	// Customer shopping optimization: Optimize My Cart, budget shopping and
	// recurring baskets.
	//
	// It is handed the fulfilment service ONLY through a quote-only port, so
	// a hundred candidate baskets produce a hundred delivery quotes and zero
	// plans, zero missions and zero payment intents. Nothing here is wired to
	// Wallet or orders, which is what makes that structural rather than a
	// promise.
	basketService := buildBasketOptimizationService(cfg, db, fulfilmentService)

	// Section 17D immutable quote lock. It freezes current checkout, product,
	// fulfilment and funding evidence but performs no payment or order work.
	quoteService := buildSection17DQuoteService(cfg, db, checkoutPromotionAdapter{service: promotionService, secret: cfg.Auth.OTPSecret})
	if quoteService != nil && checkoutService != nil && fulfilmentService != nil {
		quoteService.SetReplacementSourceAuthority(customer14ReplacementSource{db: db, checkout: checkoutService, planning: fulfilmentService})
	}

	// Section 17E order orchestration. It cuts one ParentOrder and its
	// isolated VendorOrders from an approved quote. Composed with no Wallet
	// or mission-execution dependency: it creates orders, not payments.
	orderService := buildSection17EOrderService(cfg, db, evaluator, inventoryStockService, quoteService, productionService, abuseLimiter, events.NewDomainPublisher(outboxRepo))

	// Fulfilment readiness and just-in-time orchestration.
	//
	// Built here, after the order service, because the two are wired to each
	// other: the orchestration commits through the order service (so a
	// just-in-time mission is assembled by exactly the code that assembles a
	// legacy one), and the order service plans through the orchestration
	// instead of dispatching during activation.
	//
	// A nil committer is passed deliberately when there is no order service to
	// commit through, so the fail-closed check inside sees the truth rather
	// than a non-nil adapter wrapping nothing.
	var missionCommitter orchestration.MissionCommitter
	if orderService != nil {
		missionCommitter = productionMissionCommitter{orders: orderService}
	}
	readinessOrchestration, err := buildReadinessOrchestration(cfg, db, evaluator, missionCommitter)
	if err != nil {
		logging.Fatal("orchestration", "assemble just-in-time orchestration: %v", err)
	}
	// The other direction. Wiring it only when just-in-time commitment is
	// actually enabled is what makes legacy and JIT mutually exclusive: with
	// the flag off the order service has no planner, and activation dispatches
	// synchronously exactly as it always has.
	if readinessOrchestration.orchestrationEnabled && orderService != nil {
		orderService.SetLogisticsPlanner(orchestrationPlanner{
			service: readinessOrchestration.orchestrationService,
		})
	}
	if orderService != nil {
		productionService.SetETAReadinessProjector(orderService)
	}

	// The operational fulfilment aggregate: the group rooted on one commerce
	// order, its pickup obligations, its parcels, its versioned plan history,
	// the projections of the logistics missions executing it, and the vendor's
	// own delivery missions.
	//
	// It is gated on commerce orders because it is post-order by definition.
	// Wired onto the order service as a PROJECTION rather than composed into
	// it: orders decides what is being fulfilled, and the aggregate records
	// what the executing authorities then say about it.
	fulfilmentOpsService := fulfilmentgroup.NewService(fulfilmentgroup.Options{
		Store:      fulfilmentgroup.NewStore(db, events.NewDomainPublisher(outboxRepo)),
		Production: productionService,
		Enabled:    cfg.Flags.CommerceOrdersEnabled,
	})
	fulfilmentOpsHandler := fulfilmentgroup.NewHandler(fulfilmentOpsService)
	var fulfilmentOpsProjection *fulfilmentOperationsProjection
	if orderService != nil {
		if projection := newFulfilmentOperationsProjection(fulfilmentOpsService, db); projection != nil {
			fulfilmentOpsProjection = projection
			orderService.SetFulfilmentProjection(projection)
		}
		// Post-pick physical re-evaluation (Karryam V4 §14.6) uses THE same
		// Section 17C planner checkout uses, so a packed load meets exactly the
		// policy, capabilities and authorities its quote was built on.
		if cfg.Flags.CommerceFulfilmentPlanningEnabled {
			if planner, plannerErr := newSection17CPlanner(cfg, db, deliveryService); plannerErr == nil {
				if evaluator := newPhysicalLoadEvaluator(db, planner); evaluator != nil {
					orderService.SetPhysicalLoadEvaluator(evaluator)
				}
			}
		}
	}
	fulfilmentOpsService.SetHybridFallbackAuthority(newVendorDeliveryRuntimeAuthority(
		db, fulfilmentOpsService, fulfilmentOpsProjection, orderService,
		cfg.WalletURL, cfg.WalletSecret, cfg.Fulfilment.QuoteTimeout,
	))
	fulfilmentOpsService.SetVendorMissionOrderAuthority(orderService)
	fulfilmentOpsService.SetVendorHandoverAuthority(newVendorOwnedRuntimeHandover(
		db, cfg.DispatchURL, cfg.DispatchSecret, config.Section32VendorOwnedHandoverEnabled(),
	))

	// Section 21 Prompt 1 commerce foundations. Orders remains the conversion
	// authority and Production remains the capacity authority.
	section21Service := orders.NewSection21Service(orders.Section21Options{
		DB: db, Orders: orderService, Checkout: quoteService, Production: productionService,
		Payments: newSection17FPaymentAuthority(cfg.Flags.PaymentWalletCommerceEnabled, cfg.WalletURL, cfg.WalletSecret, cfg.Fulfilment.QuoteTimeout),
		Scopes:   section17EScopeAdapter{evaluator: evaluator},
		Events:   events.NewOutboxRepository(db),
		Enabled:  cfg.Flags.CommerceBulkRFQEnabled,
	})
	if cfg.Flags.CommerceBulkRFQEnabled {
		cutoffJob := jobs.Job{Name: "commerce-scheduled-cutoff-notifications", Interval: time.Minute, Pass: func(runCtx context.Context) error {
			_, err := section21Service.EmitScheduledCutoffNotifications(runCtx, 100)
			return err
		}}
		workers = append(workers, func(runCtx context.Context) error {
			cutoffJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
		section21ExpiryJob := jobs.Job{Name: "commerce-rfq-expiry", Interval: time.Minute, Pass: func(runCtx context.Context) error {
			_, err := section21Service.ExpireRFQs(runCtx, 100)
			return err
		}}
		workers = append(workers, func(runCtx context.Context) error {
			section21ExpiryJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
		section21RecurringJob := jobs.DurableJob{
			Job: jobs.Job{Name: "commerce-recurring-feeding", Interval: time.Minute, Pass: func(runCtx context.Context) error {
				_, err := section21Service.GenerateRecurringOccurrences(runCtx, 500)
				return err
			}},
			Coordinator: durableInventoryCoordinator,
			Owner:       durableInventoryWorkerOwner,
			LeaseFor:    2 * time.Minute,
			MaxAttempts: 5,
			RetryBase:   5 * time.Second,
		}
		workers = append(workers, func(runCtx context.Context) error {
			section21RecurringJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
		section21PaymentRecoveryJob := jobs.DurableJob{
			Job: jobs.Job{Name: "commerce-rfq-payment-recovery", Interval: time.Minute, Pass: func(runCtx context.Context) error {
				_, err := section21Service.RecoverRFQPayments(runCtx, 100)
				return err
			}},
			Coordinator: durableInventoryCoordinator,
			Owner:       durableInventoryWorkerOwner,
			LeaseFor:    2 * time.Minute,
			MaxAttempts: 5,
			RetryBase:   5 * time.Second,
		}
		workers = append(workers, func(runCtx context.Context) error {
			section21PaymentRecoveryJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
	}

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

	for _, build := range []func() (jobs.DurableJob, bool){
		func() (jobs.DurableJob, bool) {
			return basketProposalExpiryJob(
				cfg, basketService,
				durableInventoryCoordinator, durableInventoryWorkerOwner,
			)
		},
		func() (jobs.DurableJob, bool) {
			return basketRecurrenceJob(
				cfg, basketService,
				durableInventoryCoordinator, durableInventoryWorkerOwner,
			)
		},
	} {
		basketJob, ok := build()

		if !ok {
			continue
		}

		job := basketJob

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
	if cfg.Flags.VendorPrivacyRetentionEnabled {
		// The flag is a claim kill switch only. Pending work remains durable and
		// becomes eligible again when the worker is re-enabled.
		retentionWorker := privacy.NewRetentionWorker(db, privacy.DefaultMaxAttempts)
		retentionJob := jobs.Job{Name: "vendor-privacy-retention", Interval: time.Minute,
			Pass: func(runCtx context.Context) error {
				_, err := retentionWorker.RunPass(runCtx, privacy.DefaultBatchSize)
				return err
			}}
		workers = append(workers, func(runCtx context.Context) error {
			retentionJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
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

	// Readiness/orchestration passes. Both are empty unless just-in-time
	// orchestration is enabled, so a deployment that has not certified it adds
	// no background work at all.
	for _, job := range readinessOrchestration.Workers() {
		orchestrationJob := job
		workers = append(workers, func(runCtx context.Context) error {
			orchestrationJob.Run(runCtx, func(name string, err error) { logging.Error("worker", "%s: %v", name, err) })
			return nil
		})
	}

	// Deployment topology is decided after workers are assembled so the same
	// domain wiring is used by every mode. API instances never claim work. The
	// global kill switch also stops new claims while leaving database-backed
	// pending jobs, outbox rows, audit and reads untouched.
	workers = workersForProcess(cfg.ProcessMode, cfg.Flags.CommerceGlobalKillSwitch, workers)

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
		ReadyChecks: []func(context.Context) error{db.Ping},
		Register: func(group *gin.RouterGroup) {
			vendorauthhttp.RegisterRoutes(group, authHandler, accessSigner, sessionRepo, staffResolverAdapter{staff: staffRepo})

			protected := group.Group("")
			protected.Use(vendorauthhttp.RequireAuth(accessSigner, sessionRepo, staffResolverAdapter{staff: staffRepo}))
			staffhttp.RegisterRoutes(protected, staffHandler)
			vendors.RegisterRoutes(protected, vendorHandler)
			vendors.RegisterOnboardingRoutes(protected, vendorHandler)
			financeprojection.RegisterRoutes(protected, financeHandler, cfg.Flags.PaymentWalletCommerceEnabled, idempotencyStore)
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
			assets.RegisterRoutes(protected, assetHandler)
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
			promotions.RegisterRoutes(protected, promotionHandler, cfg.Flags.CommerceQuoteLockEnabled)

			fulfilmentgroup.RegisterRoutes(
				protected,
				fulfilmentOpsHandler,
				cfg.Flags.CommerceOrdersEnabled,
			)

			purchasing.RegisterOperationsRoutes(protected, purchasingOperationsHandler, cfg.Flags.VendorPurchasingEnabled)
			section13Components.registerVendorRoutes(protected)
			readinessOrchestration.registerVendorRoutes(protected)

			// Section 17E vendor order queue and lifecycle commands.
			orders.RegisterVendorRoutes(
				protected,
				orderService,
				cfg.Flags.CommerceOrdersEnabled,
				idempotencyStore,
			)

			orders.RegisterSection21VendorRoutes(protected, section21Service, cfg.Flags.CommerceBulkRFQEnabled)

		},
		RegisterRoot: func(router *gin.Engine) {
			// Customer V4 §27.2: every customer commerce route below is
			// registered after this, so each is measured by family/outcome.
			router.Use(commerceRequestMetrics())
			registerCustomerDiscoveryRoutes(router, cfg, foodService, shopService, section13Components, abuseLimiter)
			shareService := &customershare.Service{DB: db, EnabledAuthorities: map[string]bool{"DISPATCH": cfg.Flags.DispatchCommerceIntegrationEnabled, "HAULING": cfg.Flags.HaulingCommerceIntegrationEnabled, "TAXI_GOODS": cfg.Flags.TaxiGoodsDeliveryEnabled, "VENDOR_OWN_DELIVERY": cfg.Flags.VendorOwnDeliveryEnabled}, Enabled: cfg.Flags.DispatchCommerceIntegrationEnabled || cfg.Flags.HaulingCommerceIntegrationEnabled || cfg.Flags.TaxiGoodsDeliveryEnabled || cfg.Flags.VendorOwnDeliveryEnabled, Logistics: &journeyshare.Client{Caller: "vendor-service", Endpoints: map[string]journeyshare.Endpoint{"DISPATCH": {BaseURL: cfg.DispatchURL, Secret: []byte(cfg.DispatchSecret)}, "HAULING": {BaseURL: cfg.HaulingURL, Secret: []byte(cfg.HaulingSecret)}, "TAXI_GOODS": {BaseURL: cfg.TaxiURL, Secret: []byte(cfg.TaxiSecret)}}, Paths: map[string]string{"DISPATCH": "/internal/dispatch/v1/commerce-share/missions/", "HAULING": "/internal/hauling/v1/commerce-share/missions/", "TAXI_GOODS": "/internal/taxi/v1/commerce-share/missions/"}}}
			customershare.Register(router, shareService, journeyshare.RedisBoundary(inboundVerifier, redisClient, "customer-service"))
			customerCartService := registerSection17ACustomerCartRoutes(
				router,
				cfg,
				db,
				foodService,
				shopService,
				idempotencyStore,
			)
			registerCartDeliveryPreview(router, cfg, customerCartService, newSection17BFoodAuthority(db, foodService), newSection17BShopAuthority(db, shopService, inventoryStockService), fulfilmentService, abuseLimiter)
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
			registerBasketOptimizationRoutes(
				router,
				cfg,
				basketService,
			)
			registerSection17DQuoteRoutes(
				router,
				cfg,
				quoteService,
				idempotencyStore,
				abuseLimiter,
			)
			registerSection17ECustomerOrderRoutes(
				router,
				cfg,
				orderService,
				idempotencyStore,
			)
			orders.RegisterSection21CustomerRoutes(router, section21Service,
				sharedauth.NewTokenSigner(cfg.CustomerAccessTokenSecret), cfg.Flags.CommerceBulkRFQEnabled)
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
			// Operator-facing readiness for the logistics integrations
			// (VND-031). Root-mounted and HMAC-protected for the same reason
			// the logistics ingress is: which authorities a deployment can
			// reach is operational detail, not a customer surface.
			registerLogisticsReadinessRoutes(
				router,
				newLogisticsRegistry(cfg, cfg.Fulfilment.QuoteTimeout),
				inboundVerifier,
				map[string]bool{
					"admin-backoffice-service": true,
					"analytics-service":        true,
				},
			)
			registerCommerceMetricsRoute(router, db, inboundVerifier, map[string]bool{
				"admin-backoffice-service": true,
				"analytics-service":        true,
			})
			orders.RegisterDispatchCommerceEventRoutes(router, orderService, inboundVerifier)
			orders.RegisterSupportResolutionRoutes(router, orderService, inboundVerifier)
			// Section 13 mounts two more internal-authority surfaces here: platform
			// taxonomy administration and internal Food/Shop search. Each carries its
			// own caller allowlist.
			section13Components.registerInternalRoutes(router, inboundVerifier)
			// The read-only orchestration decision surface, with its own
			// caller allowlist: an authority reconciling its own work may ask
			// what Vendor planned, but none may tell Vendor what to plan.
			readinessOrchestration.registerInternalRoutes(router, inboundVerifier)
		},
		Workers:     workers,
		DisableHTTP: cfg.ProcessMode == "worker",
	})
}

func requireWorkerDatabase(ctx context.Context, mode string, db interface{ Ping(context.Context) error }) error {
	if mode != "worker" {
		return nil
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return db.Ping(pingCtx)
}

func workersForProcess(mode string, killSwitch bool, workers []func(context.Context) error) []func(context.Context) error {
	if mode == "api" || killSwitch {
		return nil
	}
	return workers
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
