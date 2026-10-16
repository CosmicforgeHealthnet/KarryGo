package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/database"
	authclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/clients"
	authrepositories "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	bookingfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking"
	bookingclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking/clients"
	dispatchcommercemission "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercemission"
	dispatchcommercequote "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/movement"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
	tripsettlement "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/tripsettlement"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/router"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/managedsettlement"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/providercompat"
	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/routing"
	"cosmicforge/logistics/shared/go/walletclient"
)

func main() {
	// Best-effort .env load for local development (same pattern as hauling).
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx := context.Background()
	observability.InstallDependencyTransport(map[string]string{"wallet": cfg.WalletServiceURL, "notification": cfg.NotificationURL, "media": cfg.MediaFileBaseURL, "vendor": cfg.VendorServiceURL, "fleet": cfg.FleetServiceURL, "analytics": cfg.AnalyticsURL, "routing": cfg.RouteServiceURL})
	db, err := connectPostgresWithRetry(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect dispatch delivery postgres: %v", err)
	}
	defer db.Close()

	redisClient, err := connectRedisWithRetry(ctx, cfg)
	if err != nil {
		log.Fatalf("connect dispatch delivery redis: %v", err)
	}
	defer redisClient.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	walletClient := walletclient.Client{
		BaseURL:     cfg.WalletServiceURL,
		ServiceName: cfg.WalletServiceSource,
		Secret:      cfg.WalletServiceSecret,
		HTTPClient:  &http.Client{Timeout: cfg.ExternalServiceTimeout},
	}

	// media-file-service client for *_STORAGE_MODE=media (production-ready
	// upload backend for trip proof, verification, vehicle, and avatar
	// uploads). ServiceName must be "dispatch-delivery-service" to match
	// media-file-service's MEDIA_FILE_SERVICE_TOKENS entry for this caller —
	// note this differs from the "driver-dispatch-delivery-service" identity
	// used with notification-service.
	mediaClient := mediaclient.New(mediaclient.Config{
		BaseURL:     cfg.MediaFileBaseURL,
		ServiceName: "dispatch-delivery-service",
		Token:       cfg.MediaFileServiceToken,
	})

	var emailClient authclients.EmailClient = authclients.NoopEmailClient{}
	if cfg.NotificationURL != "" && len(cfg.NotificationSecret) > 0 {
		emailClient = authclients.NewNotificationEmailClient(notifications.Client{
			BaseURL: cfg.NotificationURL, ServiceName: "driver-dispatch-delivery-service",
			Secret: cfg.NotificationSecret, HTTPClient: &http.Client{Timeout: cfg.ExternalServiceTimeout},
		})
		log.Print("dispatch email delivery uses notification-service")
	} else {
		log.Print("dispatch email delivery is unavailable: notification-service connection is not configured")
	}

	var membershipReader providercompat.Reader
	if cfg.ProviderOrganizationModeEnabled {
		membershipReader = providercompat.NewClient(cfg.FleetServiceURL, cfg.FleetServiceCaller,
			cfg.FleetServiceSecret, cfg.FleetServiceTimeout)
	}
	// Referral feed: new riders (and, from verification, completed onboarding)
	// are announced to referral-service in the transaction that records them.
	identityRepository := authrepositories.NewPostgresIdentityRepository(db)
	var referralFeed *referralcontract.Feed
	if cfg.ReferralFeedEnabled {
		feed, feedErr := referralcontract.NewFeed(db, "driver-dispatch-delivery-service", cfg.ReferralIdentityPepper)
		if feedErr != nil {
			log.Fatalf("configure referral feed (DISPATCH_REFERRAL_IDENTITY_PEPPER): %v", feedErr)
		}
		relay, relayErr := feed.Relay(cfg.ReferralServiceURL, cfg.ReferralServiceSecret, 10*time.Second, func(err error) {
			log.Printf("referral feed relay: %v", err)
		})
		if relayErr != nil {
			log.Fatalf("configure referral feed relay (DISPATCH_REFERRAL_SERVICE_URL / _SECRET): %v", relayErr)
		}
		referralFeed = feed
		identityRepository.SetSignupAnnouncer(authclients.NewReferralAnnouncer(feed))
		go func() { _ = relay.Run(ctx) }()
		// Announces each rider once they are fully approved.
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				if _, err := feed.AnnounceApprovals(ctx, db, authclients.DispatchApprovals, 100); err != nil {
					log.Printf("referral feed approval sweep: %v", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
		log.Printf("referral feed on: announcing new riders to %s", cfg.ReferralServiceURL)
	} else {
		log.Printf("referral feed off (DISPATCH_REFERRAL_FEED_ENABLED=false): new riders are not announced to referral-service")
	}
	_ = referralFeed

	authService := authusecases.NewAuthUsecase(authusecases.Options{
		Identities:           identityRepository,
		OTPs:                 authrepositories.NewPostgresOTPRepository(db),
		Sessions:             authrepositories.NewPostgresSessionRepository(db),
		Notifier:             authclients.NewLoggingNotificationClient(cfg.OTPDebug),
		EmailClient:          emailClient,
		Publisher:            authclients.NewRedisEventPublisher(redisClient),
		AccessTokenSecret:    cfg.AccessTokenSecret,
		RefreshTokenSecret:   cfg.RefreshTokenSecret,
		OTPSecret:            cfg.OTPSecret,
		AccessTokenTTL:       cfg.AccessTokenTTL,
		RefreshTokenTTL:      cfg.RefreshTokenTTL,
		OTPTTL:               cfg.OTPTTL,
		OTPRateWindow:        cfg.OTPRateWindow,
		OTPMaxRequests:       cfg.OTPMaxRequests,
		OTPMaxAttempts:       cfg.OTPMaxAttempts,
		OTPLockoutTTL:        cfg.OTPLockoutTTL,
		OTPDebug:             cfg.OTPDebug,
		RequireEmailDelivery: cfg.AppEnv != "development",
		Redis:                redisClient,
		MembershipReader:     membershipReader,
		ProviderService:      cfg.FleetServiceCaller,
	})

	asynqRedis := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	asynqClient := asynq.NewClient(asynqRedis)
	defer asynqClient.Close()

	// Lifecycle notifier: sends customer/rider notifications via
	// notification-service. No-op when DISPATCH_NOTIFICATION_URL/SECRET unset.
	bookingNotifier := bookingclients.NewBookingNotifier(cfg.NotificationURL, cfg.NotificationSecret).WithDurableLifecycle()
	if cfg.NotificationURL != "" {
		log.Printf("notification-service integration enabled url=%s", cfg.NotificationURL)
	} else {
		log.Printf("notification-service integration disabled (DISPATCH_NOTIFICATION_URL unset); apps fall back to polling")
	}

	proofStorage, err := tripfeature.NewProofStorageFromEnv(cfg.AppEnv, mediaClient)
	if err != nil {
		log.Fatalf("configure trip proof storage: %v", err)
	}
	// Canonical event emission for the request and trip transitions. Gated on
	// the outbox flag, and attached to both repositories from one instance so
	// the two aggregates cannot end up with different opinions about whether
	// events are being written.
	marketplaceEmitter := marketplace.NewLegacyEventEmitter(
		marketplace.NewRepository(db), cfg.MarketplaceOutboxEnabled)
	if marketplaceEmitter.Enabled() {
		log.Printf("marketplace: canonical event emission enabled for request and trip transitions")
	} else {
		log.Printf("marketplace: canonical event emission disabled by DISPATCH_MARKETPLACE_OUTBOX_ENABLED")
	}

	// Section 9 DSP-008/DSP-009: the canonical Fleet completed-work delivery
	// is enqueued on the EXISTING marketplace outbox, in the same transaction
	// as the mission's authoritative completion. Attached only when the flag
	// is on, so a deployment that has not certified Fleet ingestion behaves
	// exactly as before.
	tripRepository := tripfeature.NewPostgresRepository(db).WithEventEmitter(marketplaceEmitter)
	if cfg.DispatchFleetCompletedWorkEnabled {
		tripRepository = tripRepository.WithCompletedWorkEnqueuer(
			marketplace.NewCompletedWorkEnqueuer(marketplace.NewRepository(db)))
		log.Printf("marketplace: Fleet completed-work enqueue enabled for managed mission completion")
	}
	// Section 12 (WAL-012). Both of Dispatch's settlement paths -- the
	// synchronous completion and the reconciliation worker -- go through this
	// one handoff, so they address a mission's money under the same identity.
	// Without wallet credentials there is nothing to ask, so no handoff is
	// wired and corporate completions are left entirely to Fleet.
	var managedHandoff tripfeature.ManagedSettlementHandoff
	if cfg.WalletServiceURL != "" && len(cfg.WalletServiceSecret) > 0 {
		managedHandoff = tripsettlement.New(managedsettlement.Client{
			BaseURL:     cfg.WalletServiceURL,
			ServiceName: cfg.WalletServiceSource,
			Secret:      cfg.WalletServiceSecret,
			HTTPClient:  &http.Client{Timeout: cfg.ExternalServiceTimeout},
		})
	}

	tripService := tripfeature.NewService(tripRepository, proofStorage).
		WithEventPublisher(tripfeature.NewRedisEventPublisher(redisClient)).
		WithWalletClient(walletClient).
		WithNotifier(bookingNotifier).
		WithHandoverSecret(cfg.HandoverSecret).
		// Section 9 DSP-014 / Section 12 WAL-012: the corporate settlement
		// boundary. A corporate mission is never paid directly to the rider;
		// Fleet resolves the compensation and instructs Wallet, and the
		// handoff's job is only to find out whether that money actually moved
		// so an ambiguous completion is retried rather than assumed.
		WithManagedSettlement(
			tripfeature.ManagedSettlementConfig{Enabled: cfg.DispatchManagedSettlementEnabled},
			managedHandoff)
	tripfeature.StartSubscribers(ctx, redisClient, tripService)

	// Rider request push: real notification-service sender when configured,
	// logging stub otherwise (asynq send_push task retries on hard errors).
	var requestSender requestfeature.NotificationSender = requestfeature.LoggingNotificationSender{}
	if cfg.NotificationURL != "" && len(cfg.NotificationSecret) > 0 {
		requestSender = requestfeature.NewNotificationServiceSender(cfg.NotificationURL, cfg.NotificationSecret)
	}

	requestService := requestfeature.NewService(
		requestfeature.NewPostgresRepository(db).WithEventEmitter(marketplaceEmitter),
		redisClient,
		requestfeature.NewHTTPNearbyClient(cfg.AvailabilityServiceURL, cfg.InternalServiceKey),
		requestSender,
		requestfeature.NewRedisEventPublisher(redisClient),
		asynqClient,
		requestfeature.Config{
			InitialRadiusKM:   cfg.BroadcastInitialRadiusKM,
			RadiusIncrementKM: cfg.BroadcastRadiusIncrementKM,
			MaxAttempts:       cfg.BroadcastMaxAttempts,
			BroadcastWindow:   cfg.BroadcastWindow,
		},
	)
	requestfeature.StartSubscribers(ctx, redisClient, requestService)
	if len(cfg.VendorServiceSecret) > 0 {
		commerceRepository := dispatchcommercemission.NewRepository(db).WithTargetEventEmitter(
			marketplace.NewLegacyEventEmitter(marketplace.NewRepository(db), true),
		)
		commerceRuntime := dispatchcommercemission.NewMultiStopMission(
			dispatchcommercemission.MultiStopMissionOptions{
				Store: commerceRepository, Quotes: dispatchcommercequote.NewRepository(db),
				Broadcast: requestService, Notifier: bookingNotifier,
			},
		).WithHandoverSecret(cfg.HandoverSecret)
		go commerceRuntime.RunRecoveryWorker(ctx, 10*time.Second)
	}

	bookingService := bookingfeature.NewService(
		bookingfeature.NewPostgresRepository(db),
		requestService,
		bookingfeature.TripCancelAdapter{Trips: tripService},
		availability.NewRedisLiveStore(redisClient),
		bookingfeature.NewWalletPaymentClient(walletClient),
		bookingfeature.NewRedisEventPublisher(redisClient),
		cfg.AvailabilityRadiusKM,
		// routesClient is nil unless DISPATCH_ROUTES_API_KEY is set, in which
		// case routes are never fetched and the apps fall back to
		// straight-line geometry.
	).WithNotifier(bookingNotifier).WithRoutes(routing.New(cfg.RoutesAPIKey))
	bookingService.WithCommerceSettlement(bookingfeature.NewWalletCommerceSettlementClient(walletClient))
	tripService.WithSettlementReconciler(bookingService, tripfeature.SettlementRetryPolicy{
		Lease: cfg.ExternalOperationLease, MaxAttempts: cfg.ExternalOperationRetryLimit,
		BaseBackoff: cfg.ExternalOperationBackoffBase, MaxBackoff: cfg.ExternalOperationBackoffMax,
	})
	bookingfeature.StartSubscribers(ctx, redisClient, bookingService)
	go bookingService.RunHoldReconciliationWorker(ctx, cfg.RecoveryWorkerInterval, bookingfeature.HoldRetryPolicy{
		Lease: cfg.ExternalOperationLease, MaxAttempts: cfg.ExternalOperationRetryLimit,
		AbandonAfter: 2*cfg.ExternalServiceTimeout + cfg.ExternalOperationLease,
		BaseBackoff:  cfg.ExternalOperationBackoffBase, MaxBackoff: cfg.ExternalOperationBackoffMax,
	})

	asynqServer := asynq.NewServer(asynqRedis, asynq.Config{
		Concurrency: 10,
		Queues:      map[string]int{"critical": 6, "default": 3, "low": 1},
	})
	asynqMux := asynq.NewServeMux()
	requestfeature.RegisterWorkerHandlers(asynqMux, requestfeature.NewWorker(requestService))
	go func() {
		if err := asynqServer.Run(asynqMux); err != nil {
			log.Printf("request asynq server stopped: %v", err)
		}
	}()

	// Organization marketplace relay worker. Returns a no-op stopper when the
	// feature flag is off or the fleet client is unconfigured.
	stopMarketplaceWorkers := router.StartMarketplaceWorkersWithAssignment(ctx, cfg, db, availability.NewRedisLiveStore(redisClient), bookingService, tripService)
	// Re-adopt broadcasts stranded mid-window by a crash before their expire
	// task was scheduled (idempotent — the worker re-checks state).
	go func() {
		if err := requestService.RecoverStrandedBroadcasts(ctx); err != nil {
			log.Printf("request stranded-broadcast recovery failed: %v", err)
		}
	}()

	// Settle trips left stuck in 'proof_submitted' when the rider's app never
	// called complete (killed, crashed, lost network). Without this the
	// customer's escrowed fare stays held and the rider is never paid.
	go tripService.RunAutoCompleteWorker(ctx)

	// Customer movement events. The sweeper reads committed delivery state and
	// queues what changed; the publisher delivers it to the customer context
	// owner. Both are additive: neither can fail a delivery.
	if cfg.CustomerMovementEventsEnabled {
		go (&events.MovementReconciler{Pool: db, BaseURL: cfg.CustomerServiceURL, ServiceName: movement.SourceService, Secret: cfg.CustomerServiceSecret, Metrics: observability.Metrics}).Run(ctx)
		movementPublisher := movement.NewPublisher(db)
		go movementPublisher.Run(ctx, 10*time.Second)
		go (&events.MovementPublisher{
			Pool:  db,
			Owner: movement.SourceService,
			Deliverer: &events.HTTPDeliverer{
				BaseURL: cfg.CustomerServiceURL, Path: events.CustomerMovementIngressPath,
				ServiceName: movement.SourceService, Secret: cfg.CustomerServiceSecret,
			},
		}).Run(ctx, 5*time.Second)
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router.New(cfg, db, redisClient, authService, requestService, tripService, bookingService, mediaClient),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("%s listening on %s", cfg.ServiceName, cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("%s stopped: %v", cfg.ServiceName, err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("%s shutdown: %v", cfg.ServiceName, err)
	}
	asynqServer.Shutdown()
	// Stop after the HTTP server so an in-flight provider decision finishes
	// being recorded before the worker that delivers it goes away.
	stopMarketplaceWorkers()
}

func connectPostgresWithRetry(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	var lastErr error
	for attempt := 1; attempt <= 30; attempt++ {
		pool, err := database.NewPool(ctx, databaseURL)
		if err == nil {
			return pool, nil
		}

		lastErr = err
		log.Printf("waiting for dispatch delivery postgres attempt=%d error=%v", attempt, err)
		time.Sleep(2 * time.Second)
	}

	return nil, lastErr
}

func connectRedisWithRetry(ctx context.Context, cfg config.Config) (*redis.Client, error) {
	var lastErr error
	for attempt := 1; attempt <= 30; attempt++ {
		client := redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})

		if err := client.Ping(ctx).Err(); err == nil {
			return client, nil
		} else {
			lastErr = err
			_ = client.Close()
			log.Printf("waiting for dispatch delivery redis attempt=%d error=%v", attempt, err)
			time.Sleep(2 * time.Second)
		}
	}

	return nil, lastErr
}
