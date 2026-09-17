package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/taxi-service/internal/adminaudit"
	"cosmicforge/logistics/services/taxi-service/internal/config"
	"cosmicforge/logistics/services/taxi-service/internal/database"

	providerauthhttp "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/http"
	providerauthrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/repositories"
	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"

	providerprofilehttp "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/http"
	providerprofilerepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/repositories"
	providerprofileusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/usecases"

	availabilityhttp "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/http"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	availabilityusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/usecases"

	marketrepositories "cosmicforge/logistics/services/taxi-service/internal/features/markets/repositories"

	bookingclients "cosmicforge/logistics/services/taxi-service/internal/features/booking/clients"
	bookinghttp "cosmicforge/logistics/services/taxi-service/internal/features/booking/http"
	bookingmatching "cosmicforge/logistics/services/taxi-service/internal/features/booking/matching"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	bookingreviews "cosmicforge/logistics/services/taxi-service/internal/features/booking/reviews"
	bookingtelemetry "cosmicforge/logistics/services/taxi-service/internal/features/booking/telemetry"
	bookingusecases "cosmicforge/logistics/services/taxi-service/internal/features/booking/usecases"
	bookingverification "cosmicforge/logistics/services/taxi-service/internal/features/booking/verification"
	capabilities "cosmicforge/logistics/services/taxi-service/internal/features/capabilities"
	fleetprovider "cosmicforge/logistics/services/taxi-service/internal/features/fleetprovider"
	goods "cosmicforge/logistics/services/taxi-service/internal/features/goods"
	"cosmicforge/logistics/services/taxi-service/internal/features/managedexec"
	"cosmicforge/logistics/services/taxi-service/internal/movement"

	identityhttp "cosmicforge/logistics/services/taxi-service/internal/features/identity/http"
	operationshttp "cosmicforge/logistics/services/taxi-service/internal/features/operations/http"

	"cosmicforge/logistics/services/taxi-service/internal/apiresponse"
	"cosmicforge/logistics/services/taxi-service/internal/idempotency"
	taxioutbox "cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/services/taxi-service/internal/pricing"
	"cosmicforge/logistics/services/taxi-service/internal/routing"

	taxisettlement "cosmicforge/logistics/services/taxi-service/internal/features/booking/settlement"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/managedsettlement"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/observability"
	"cosmicforge/logistics/shared/go/outboxmetrics"
	"cosmicforge/logistics/shared/go/providercompat"
	sharedrouting "cosmicforge/logistics/shared/go/routing"
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
	// Fail closed at startup, not at the first booking request: a malformed
	// TAXI_PROVISIONAL_RATES value must never let the service boot into a
	// state where every quote silently 500s.
	pricingRates, err := pricing.ParseRatesJSON(cfg.PricingRatesJSON)
	if err != nil {
		logging.Fatal("configuration", "%v", err)
	}
	var goodsPricing goods.PricingPolicy
	if cfg.TaxiGoodsDeliveryEnabled {
		goodsPricing, err = goods.ParsePricingPolicy(cfg.TaxiGoodsPricingJSON)
		if err != nil {
			logging.Fatal("configuration", "%v", err)
		}
	}
	logging.Notice("taxi-service", "migration=%t redis=%s", cfg.Migration, cfg.Redis.Addr)

	// ctx is cancelled when main returns (after serviceapp.Run exits on SIGTERM).
	// Background workers use this to stop cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create taxi database pool: %v", err)
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	// The notification queue is consumed by notification-service, so it lives on
	// whichever Redis that service reads — usually a different instance from our
	// own. Reuse the main client when both point at the same address so we do not
	// hold a second pool for nothing.
	notificationRedis := redisClient
	if cfg.NotificationRedis.Addr != cfg.Redis.Addr || cfg.NotificationRedis.DB != cfg.Redis.DB {
		notificationRedis = redis.NewClient(&redis.Options{
			Addr:     cfg.NotificationRedis.Addr,
			Password: cfg.NotificationRedis.Password,
			DB:       cfg.NotificationRedis.DB,
		})
		defer notificationRedis.Close()
		logging.Notice("notification-queue", "redis=%s", cfg.NotificationRedis.Addr)
	}
	_ = notificationRedis // wired to the ride notifier in A9

	logConnectivity(ctx, cfg.Redis.Addr, db, redisClient)

	if cfg.Migration {
		logging.Notice("migration", "mode enabled")
		migrationCtx, mcancel := context.WithTimeout(ctx, 30*time.Second)
		if err := database.ApplyMigrations(migrationCtx, db); err != nil {
			mcancel()
			logging.Fatal("migration", "apply taxi-service migrations: %v", err)
		}
		mcancel()
		logging.Success("migration", "applied successfully")
	}

	// ─── Repositories ─────────────────────────────────────────────────────────
	providerRepo := providerauthrepositories.NewPostgresProviderRepository(db)
	sessionRepo := providerauthrepositories.NewPostgresRefreshSessionRepository(db)
	challengeStore := providerauthrepositories.NewRedisOTPChallengeRepository(redisClient)
	profileRepo := providerprofilerepositories.NewPostgresProfileRepository(db)
	vehicleRepo := providerprofilerepositories.NewPostgresVehicleRepository(db)

	// ─── Services ─────────────────────────────────────────────────────────────
	var fleetProviderContext providercompat.CorporateContextReader
	var fleetProviderClient *fleetprovider.Client
	// fleetProviderCompat is the concrete client, kept alongside the
	// interface because completed-work submission needs the full client, not
	// just the corporate-context read.
	var fleetProviderCompat *providercompat.Client
	if strings.TrimSpace(cfg.FleetServiceURL) != "" && len(cfg.FleetServiceSecret) > 0 {
		fleetProviderCompat = providercompat.NewClient(cfg.FleetServiceURL, cfg.FleetServiceCaller, cfg.FleetServiceSecret, cfg.FleetServiceTimeout)
		fleetProviderContext = fleetProviderCompat
		fleetProviderClient, _ = fleetprovider.NewClient(fleetprovider.Config{BaseURL: cfg.FleetServiceURL, ServiceName: cfg.FleetServiceCaller, Secret: cfg.FleetServiceSecret, Timeout: cfg.FleetServiceTimeout})
		logging.Notice("taxi-fleet", "corporate context client enabled url=%s", cfg.FleetServiceURL)
	}

	authService := providerauthusecases.NewAuthService(providerauthusecases.Options{
		Providers:          providerRepo,
		Sessions:           sessionRepo,
		Challenges:         challengeStore,
		AccessTokenSecret:  cfg.ProviderTokenSecret,
		RefreshTokenSecret: cfg.ProviderRefreshSecret,
		OTPSecret:          cfg.ProviderOTPSecret,
		AccessTokenTTL:     cfg.AccessTokenTTL,
		RefreshTokenTTL:    cfg.RefreshTokenTTL,
		OTPTTL:             cfg.OTPTTL,
		OTPRateWindow:      cfg.OTPRateWindow,
		OTPMaxRequests:     cfg.OTPMaxRequests,
		OTPMaxAttempts:     cfg.OTPMaxAttempts,
		OTPDebug:           cfg.OTPDebug,
		CorporateContext:   fleetProviderContext,
	})

	profileService := providerprofileusecases.NewProfileService(profileRepo, vehicleRepo)

	// The tracking fix TTL is bound to the same clock as the online status key,
	// so a driver can never be matched from a position that has already aged out
	// of their availability.
	onlineTTL := time.Duration(cfg.ProviderOnlineTTL) * time.Second
	availabilityStore := availabilityrepositories.NewRedisAvailabilityStoreWithTTL(redisClient, onlineTTL)
	operationalStateRepo := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	marketRepo := marketrepositories.NewPostgresMarketRepository(db)
	availabilityService := availabilityusecases.NewAvailabilityService(
		availabilityStore, operationalStateRepo, providerRepo, vehicleRepo, marketRepo, onlineTTL)

	// Verify-only signer for passenger bearer tokens. customer-service mints
	// them; this service only checks them, so the secret must match
	// CUSTOMER_ACCESS_TOKEN_SECRET there.
	customerSigner := sharedauth.NewTokenSigner(cfg.CustomerTokenSecret)

	// routesFetcher is nil-safe: New reports ErrNotConfigured on every call
	// rather than panicking when TAXI_ROUTES_API_KEY is unset, so local dev
	// works without a Google Cloud key -- routingClient's haversine fallback
	// absorbs the resulting failures.
	routesFetcher := sharedrouting.New(cfg.RoutesAPIKey)
	googleRouting := routing.NewGoogleRoutingClient(routesFetcher)
	routingClient := routing.NewHaversineFallbackClient(googleRouting, 0, 0)
	pricingProvider := pricing.NewConfigPricingProvider(pricingRates, cfg.PricingRateVersion, routingClient)

	// 24h mirrors the platform convention already documented in
	// internal/idempotency's own tests; no caller needs a key to outlive one
	// working day.
	idempotencyStore := idempotency.NewStore(db, 24*time.Hour)

	// Section 7: movement summaries for customer-service are written through
	// the outbox in the same transaction as each change (dark by default).
	metrics := observability.NewMetrics()
	movementWriter := &movement.Writer{Enabled: cfg.CustomerMovementEventsEnabled}
	capabilityService := capabilities.NewService(capabilities.NewRepository(db), capabilities.Flags{
		ServiceProductsEnabled:    cfg.ServiceProductsEnabled,
		CapabilityMatchingEnabled: cfg.CapabilityMatchingEnabled,
	}, metrics)
	bookingRepo := bookingrepositories.NewPostgresBookingRepository(db).WithMovementEvents(movementWriter)
	var riskStore bookingusecases.ScheduledRiskStore
	if cfg.ScheduledRiskMonitorEnabled {
		riskStore = bookingRepo
	}
	matcherLease := bookingmatching.NewMatcherLease(redisClient)
	pickupVerificationRepo := bookingverification.NewPostgresRepository(db)
	pickupVerifyRateLimiter := bookingverification.NewRateLimiter(redisClient)
	telemetryRepo := bookingtelemetry.NewPostgresRepository(db)
	reviewsRepo := bookingreviews.NewPostgresRepository(db)
	adminAuditRepo := adminaudit.NewPostgresRepository(db)
	// InstanceID identifies this process for the booking-level matcher lease
	// -- generated once at startup, stable for the process's lifetime,
	// unique across horizontally-scaled instances by construction (a fresh
	// UUID per process).
	instanceID := uuid.NewString()

	// Payments is left nil when unconfigured (no TAXI_PAYMENT_URL/_SECRET),
	// matching the same "disabled integration is a safe no-op" contract this
	// service already documents for TAXI_NOTIFICATION_URL/TAXI_ROUTES_API_KEY
	// -- see Options.Payments' own doc comment. Config.Validate already
	// requires both in production, so this branch only ever taken in local
	// dev without payment-wallet-service running.
	var paymentClient bookingusecases.PaymentClient
	var walletAuthority walletclient.Client
	if strings.TrimSpace(cfg.PaymentURL) != "" && len(cfg.PaymentSecret) > 0 {
		walletAuthority = walletclient.Client{
			BaseURL: cfg.PaymentURL, HTTPClient: &http.Client{Timeout: 10 * time.Second},
			ServiceName: bookingusecases.PaymentServiceName, Secret: cfg.PaymentSecret,
		}
		paymentClient = walletAuthority
		logging.Notice("taxi-payments", "wallet integration enabled url=%s", cfg.PaymentURL)
	} else {
		logging.Notice("taxi-payments", "wallet integration disabled (no TAXI_PAYMENT_URL/_SECRET) -- rides will not be charged")
	}

	// Section 12 (WAL-011). Taxi does not settle corporate work -- Fleet
	// resolves the compensation and instructs Wallet. What Taxi needs is a
	// definite answer about whether a completed job's money moved, so an
	// ambiguous completion is retried instead of assumed either way. Without
	// wallet credentials there is no way to ask, so no handoff is wired and
	// the completion is simply left for Fleet; that is the honest behaviour,
	// and it still never falls through to paying the driver directly.
	var managedHandoff bookingusecases.ManagedSettlementHandoff
	if strings.TrimSpace(cfg.PaymentURL) != "" && len(cfg.PaymentSecret) > 0 {
		managedHandoff = taxisettlement.New(managedsettlement.Client{
			BaseURL:     cfg.PaymentURL,
			ServiceName: bookingusecases.PaymentServiceName,
			Secret:      cfg.PaymentSecret,
			HTTPClient:  &http.Client{Timeout: 10 * time.Second},
		})
		logging.Notice("taxi-settlement", "managed settlement confirmation enabled url=%s", cfg.PaymentURL)
	} else if cfg.TaxiManagedSettlementEnabled {
		logging.Notice("taxi-settlement",
			"managed settlement isolation is on but wallet credentials are missing -- corporate completions will be left entirely to Fleet")
	}

	var productPolicy bookingusecases.ProductPolicySource
	if cfg.ServiceProductsEnabled {
		productPolicy = capabilityService
	}
	var customerContext bookingusecases.CustomerContextSource
	if cfg.CustomerContextEnabled {
		customerContext = &bookingclients.CustomerContextClient{BaseURL: cfg.CustomerServiceURL, ServiceName: movement.SourceService, Secret: cfg.CustomerServiceSecret}
	}

	bookingService := bookingusecases.NewBookingService(bookingusecases.Options{
		Bookings: bookingRepo, Markets: marketRepo, Pricing: pricingProvider, OperationalState: operationalStateRepo,
		Availability: availabilityStore, Providers: providerRepo, Vehicles: vehicleRepo, Routes: routingClient,
		Lease: matcherLease,
		Matching: bookingusecases.MatchingConfig{
			MatchTimeout:     time.Duration(cfg.BookingMatchTimeout) * time.Second,
			SearchWindow:     time.Duration(cfg.BookingSearchWindow) * time.Second,
			BaseRadiusKm:     cfg.MatchMaxRadiusKm,
			ExpandedRadiusKm: cfg.MatchExpandedRadiusKm,
			PollInterval:     time.Duration(cfg.MatchPollIntervalSeconds) * time.Second,
			InstanceID:       instanceID,
		},
		PickupVerifications: pickupVerificationRepo,
		VerifyRateLimiter:   pickupVerifyRateLimiter,
		Verification: bookingusecases.VerificationConfig{
			HandoverSecret: cfg.HandoverSecret,
			RateWindow:     cfg.PickupVerifyRateWindow,
			MaxAttempts:    cfg.PickupVerifyMaxAttempts,
		},
		Payments: paymentClient,
		// Section 8 corporate completion. All three are off by default; see
		// each flag's own doc comment in internal/config for what turning it
		// on actually changes.
		ManagedSettlement:    bookingusecases.ManagedSettlementConfig{Enabled: cfg.TaxiManagedSettlementEnabled},
		ManagedHandoff:       managedHandoff,
		CompletedWorkEnabled: cfg.TaxiFleetCompletedWorkEnabled,
		// Positions is the same Redis store used for matching candidates --
		// its repository-level Heartbeat(ctx, ProviderStatus) already
		// satisfies bookingusecases.PositionUpdater with no adapter, per
		// that interface's own doc comment.
		Telemetry: telemetryRepo,
		Positions: availabilityStore,
		TelemetryConfig: bookingtelemetry.Config{
			MaxSpeedKph:    cfg.TelemetryMaxSpeedKph,
			StaleThreshold: time.Duration(cfg.TelemetryStaleSeconds) * time.Second,
		},
		// OperationalHistory reuses the same repository instance Options.
		// OperationalState above already holds -- its StateAsOf/ListHistory
		// methods satisfy bookingusecases.OperationalHistoryLookup with no
		// adapter, same convention as Positions/availabilityStore.
		Reviews:            reviewsRepo,
		ReviewRevealWindow: cfg.ReviewRevealWindow,
		OperationalHistory: operationalStateRepo,
		Scheduling: bookingusecases.SchedulingConfig{
			LeadTime: time.Duration(cfg.ScheduleLeadMinutes) * time.Minute,
		},
		AdminAudit:     adminAuditRepo,
		Movement:       movementWriter,
		CapabilityGate: capabilityService,
		RiskStore:      riskStore,

		ProductPolicy:   productPolicy,
		CustomerContext: customerContext,
	})

	// Internal ops caller verification (phase B10) is nil when no
	// TAXI_SERVICE_SECRETS are configured -- operationshttp.
	// RegisterOperationsRoutes skips mounting the /internal group entirely
	// in that case, the same "disabled integration is a safe no-op"
	// contract this service already documents for TAXI_NOTIFICATION_URL/
	// TAXI_PAYMENT_URL. 5-minute clock skew matches the convention already
	// established for this exact verifier shape in driver-hauling-service.
	var opsVerifier *serviceauth.Verifier
	if strings.TrimSpace(cfg.ServiceSecrets) != "" {
		opsVerifier = serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute)
	}

	goodsRepo := goods.NewRepository(db).WithMovementEvents(movementWriter)
	var goodsSettlement goods.SettlementAuthority
	if strings.TrimSpace(cfg.PaymentURL) != "" && len(cfg.PaymentSecret) > 0 {
		goodsSettlement = goods.NewWalletAdapter(walletAuthority)
	}
	var goodsMedia goods.MediaAuthority
	var taxiMediaClient *mediaclient.Client
	if strings.TrimSpace(cfg.MediaURL) != "" && strings.TrimSpace(cfg.MediaToken) != "" {
		taxiMediaClient = mediaclient.New(mediaclient.Config{
			BaseURL: cfg.MediaURL, ServiceName: "driver-taxi-service", Token: cfg.MediaToken,
			HTTPClient: &http.Client{Timeout: 10 * time.Second},
		})
		goodsMedia = goods.NewMediaAdapter(taxiMediaClient)
	}
	// Fleet's private-execution authority (Section 8 TAX-012/013/014). The
	// engine and binder are the SAME booking service and repository every
	// independent ride uses, which is what keeps corporate work on the normal
	// taxi trip engine rather than a parallel one.
	managedExecService := managedexec.NewService(managedexec.Options{
		Repo:             managedexec.NewRepository(db),
		Engine:           bookingService,
		Binder:           bookingRepo,
		OperationalState: operationalStateRepo,
		Vehicles:         vehicleRepo,
		Corporate:        fleetProviderContext,
		ReservationTTL:   cfg.TaxiFleetReservationTTL,
	})

	goodsNotifier := goods.NewRedisNotificationAdapter(notifications.RedisPublisher{Client: notificationRedis})
	goodsService := goods.NewService(goods.Options{
		Repository: goodsRepo, Markets: marketRepo, Candidates: availabilityStore,
		Routing: routingClient, Settlement: goodsSettlement, Media: goodsMedia,
		Notifier: goodsNotifier, Pricing: goodsPricing, Enabled: cfg.TaxiGoodsDeliveryEnabled,
		HandoverSecret: cfg.TaxiGoodsHandoverSecret, QuoteTTL: cfg.TaxiGoodsQuoteTTL,
		MatchRadiusKM: cfg.MatchMaxRadiusKm,
		Capabilities: goods.ExecutionFlags{
			CapabilityEnabled:     cfg.GoodsCapabilityEnabled,
			MultiPickupEnabled:    cfg.GoodsMultiPickupEnabled,
			SecureHandoverEnabled: cfg.GoodsSecureHandoverEnabled,
			CumulativeFitEnabled:  cfg.GoodsCumulativeFitEnabled,
			RecoveryEnabled:       cfg.RecoveryEnabled,
		},
		Policies: capabilityService,
		Metrics:  metrics,
	})
	// ONE durable outbox, one worker, two destinations (Section 8 TAX-016).
	// Vendor lifecycle rows and Fleet completed-work rows share the same
	// leasing, backoff, dead-lettering and reconciliation machinery; the
	// router only decides where a claimed row is delivered. Building a second
	// outbox framework for Fleet would have duplicated all of that.
	taxiOutboxStore := taxioutbox.NewStore(db)
	var fleetCompletedWorkClient taxioutbox.CompletedWorkSubmitter
	if fleetCompletedWork := fleetProviderCompat; fleetCompletedWork != nil {
		fleetCompletedWorkClient = fleetCompletedWork
	}
	outboxRouter := taxioutbox.NewRouter(map[string]taxioutbox.Destination{
		taxioutbox.TargetVendor: taxioutbox.VendorClient{
			BaseURL: cfg.VendorURL, Secret: cfg.VendorSecret,
			HTTPClient: &http.Client{Timeout: 10 * time.Second},
		},
		taxioutbox.TargetFleet: taxioutbox.FleetCompletedWorkDestination{
			Client:  fleetCompletedWorkClient,
			Enabled: cfg.TaxiFleetCompletedWorkEnabled && fleetCompletedWorkClient != nil,
		},
		// Customer movement rows share the same durable outbox, worker,
		// leasing and backoff as the Vendor lifecycle and Fleet completed
		// work; the target is what keeps the three from ever being delivered
		// to each other.
		taxioutbox.TargetCustomer: movement.Destination{Deliverer: events.HTTPDeliverer{
			BaseURL: cfg.CustomerServiceURL, Path: events.CustomerMovementIngressPath, ServiceName: movement.SourceService,
			Secret: cfg.CustomerServiceSecret, HTTPClient: &http.Client{Timeout: 10 * time.Second},
		}},
	})
	goodsOutboxPublisher := taxioutbox.NewPublisher(taxiOutboxStore, outboxRouter, "taxi-outbox-"+uuid.NewString())
	if err := goodsService.AssertDependencies(); err != nil {
		logging.Fatal("configuration", "%v", err)
	}

	// Outbox and reconciliation health, in the same metric names every other
	// service reports (OBS-006, OBS-007).
	//
	// Mounted at /outbox-metrics rather than /metrics because Taxi already
	// publishes booking and availability gauges at /metrics, and merging two
	// registries would mean one of them silently wins.
	//
	// The source descriptions live with the table's writer, where a test can
	// run them against the real migrated schema.
	taxiOutboxMetrics := outboxmetrics.NewReporter(db, taxioutbox.MetricsInterval,
		taxioutbox.MetricsSources()...).WithAuthToken(cfg.MetricsAuthToken)

	serviceapp.Run(serviceapp.Options{
		Name:        "taxi-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/taxi",
		Capabilities: []string{
			"taxi driver auth entry using shared auth helpers",
			"taxi driver profiles and vehicle records",
			"taxi ride bookings",
			"taxi driver matching",
			"taxi trip lifecycle",
			"taxi active-trip telemetry",
			"taxi two-way reviews and earnings insights",
			"feature-gated Vendor Commerce Taxi Goods delivery",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
			func(ctx context.Context) error { return redisClient.Ping(ctx).Err() },
		},
		Register: func(group *gin.RouterGroup) {
			taxiOutboxMetrics.RegisterRoutes(group)
			if opsVerifier != nil {
				custodyops.RegisterInternal(group, custodyops.NewRepository(db, "TAXI_GOODS"), opsVerifier,
					custodyops.MediaFileAuthorizer{Client: taxiMediaClient, AllowedOwners: map[string]struct{}{"driver-taxi-service": {}, "vendor-service": {}}},
					custodyops.Policy{}, []string{"vendor-service", "operations-service"})
			}
			custodyops.RegisterProvider(group, custodyops.NewRepository(db, "TAXI_GOODS"), authService.AccessSigner(), providerauthusecases.ProviderRole, providerauthusecases.ProviderService)
			providerauthhttp.RegisterProviderAuthRoutes(group, authService, cfg.OTPDebug)
			providerprofilehttp.RegisterProfileRoutes(group, profileService, authService.AccessSigner())
			availabilityhttp.RegisterAvailabilityRoutes(group, availabilityService,
				authService.AccessSigner(), customerSigner, cfg.MatchMaxRadiusKm)
			bookinghttp.RegisterBookingRoutes(group, bookingService, authService.AccessSigner(), customerSigner, idempotencyStore,
				apiresponse.ProviderFinancialPrivacy(fleetProviderContext, cfg.TaxiTypedFinancialVisibilityEnabled))
			fleetprovider.RegisterRoutes(group, authService.AccessSigner(), fleetprovider.NewHandler(fleetProviderClient, fleetProviderContext, cfg.TaxiFleetProviderWorkEnabled))
			operationshttp.RegisterOperationsRoutes(group, bookingService, opsVerifier)
			goods.RegisterRoutes(group, goodsService, opsVerifier, authService.AccessSigner(), cfg.TaxiFleetProviderWorkEnabled)
			// Fleet's private-execution authority calls INTO taxi here. Both
			// surfaces stay unmounted unless an internal caller is trusted
			// and the integration is switched on (Section 8 TAX-028).
			managedexec.RegisterRoutes(group,
				managedexec.NewHandler(managedExecService, cfg.TaxiFleetManagedExecutionEnabled, cfg.TaxiFleetExecutionCallers),
				opsVerifier)
			managedexec.RegisterReconciliationRoutes(group,
				managedexec.NewReconciliationHandler(taxiOutboxStore, cfg.TaxiFleetExecutionCallers),
				opsVerifier)
			// Closes baseline spec finding 3.3 (phase B11): the last piece
			// needed before support-dispute-service can resolve a taxi
			// driver's identity for a complaint/dispute, the same way it
			// already does for hauling/dispatch.
			identityhttp.RegisterIdentityRoutes(group, authService, opsVerifier)
			capabilities.RegisterRoutes(group, capabilityService, authService.AccessSigner(), customerSigner, opsVerifier, strings.Split(cfg.CapabilityInternalCallers, ","))
			if opsVerifier != nil {
				group.GET("/internal/metrics", serviceauth.Middleware(opsVerifier), func(c *gin.Context) {
					c.String(http.StatusOK, metrics.Render())
				})
			}
		},
		// Every worker below runs against serviceapp.Run's own
		// signal-derived context and is waited on before shutdown completes
		// -- see that package's own Workers doc comment. Each sweeps once
		// immediately on start, then on its own interval, matching the
		// "sweep-on-start, then tick" shape already established across this
		// platform (see project memory's Go goroutine patterns note).
		Workers: []func(context.Context) error{
			taxiOutboxMetrics.Run,
			func(ctx context.Context) error {
				bookingService.RunScheduledDispatchWorker(ctx, 30*time.Second)
				return nil
			},
			func(ctx context.Context) error {
				// staleAfter = SearchWindow + MatchTimeout + a buffer: a
				// booking still pending_match/awaiting_acceptance well past
				// its own full search cycle is more likely orphaned by a
				// crash than genuinely still being searched.
				staleAfter := time.Duration(cfg.BookingSearchWindow+cfg.BookingMatchTimeout+60) * time.Second
				bookingService.RunMatchRecoveryWorker(ctx, 30*time.Second, staleAfter)
				return nil
			},
			func(ctx context.Context) error {
				bookingService.RunAutoCompleteWorker(ctx, 5*time.Minute, 30*time.Minute)
				return nil
			},
			func(ctx context.Context) error {
				if cfg.TaxiGoodsDeliveryEnabled {
					goodsService.RunSettlementWorker(ctx, time.Minute)
				}
				return nil
			},
			func(ctx context.Context) error {
				if cfg.TaxiGoodsDeliveryEnabled {
					goodsService.RunRecoveryWorker(ctx, 30*time.Second)
				}
				return nil
			},
			func(ctx context.Context) error {
				// The single outbox worker now drains BOTH destinations, so
				// it must run whenever either is live -- gating it on Taxi
				// Goods alone would have left Fleet completed-work rows
				// undelivered in a deployment that runs corporate taxi work
				// without Commerce.
				if cfg.TaxiGoodsDeliveryEnabled || cfg.TaxiFleetCompletedWorkEnabled {
					goodsOutboxPublisher.Run(ctx, time.Second)
				}
				return nil
			},
			func(ctx context.Context) error {
				// Backfill and reconciliation. It republishes only movements
				// customer-service is missing or behind on, so a first run
				// backfills history and later runs are a safety net that
				// cannot flood the consumer.
				if cfg.CustomerMovementEventsEnabled {
					(&movement.Reconciler{
						Pool: db, Writer: movementWriter,
						Reader: &movement.CustomerProjectionClient{
							BaseURL: cfg.CustomerServiceURL, ServiceName: movement.SourceService, Secret: cfg.CustomerServiceSecret,
						},
					}).Run(ctx, 5*time.Minute)
				}
				return nil
			},
			func(ctx context.Context) error {
				if cfg.ScheduledRiskMonitorEnabled {
					bookingService.RunScheduledRiskMonitor(ctx, time.Minute, 2*time.Hour)
				}
				return nil
			},
		},
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
