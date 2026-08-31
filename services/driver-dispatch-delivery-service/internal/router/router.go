package router

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authrepositories "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	bookingfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking"
	identityhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/identity/http"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	notificationhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/notifications/http"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/privateexecution"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/profile"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/restrictions"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/vehicle"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/verification"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/middleware"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// dispatchSessionRevokerAdapter bridges the auth session repository to the
// profile.SessionRevoker interface. In this service, provider_id == dispatch_rider_id.
type dispatchSessionRevokerAdapter struct {
	repo *authrepositories.PostgresSessionRepository
}

func (a *dispatchSessionRevokerAdapter) RevokeAllByProviderID(ctx context.Context, providerID string) (int64, error) {
	return a.repo.RevokeAllByDispatchRiderID(ctx, providerID)
}

func New(cfg config.Config, db *pgxpool.Pool, redisClient *redis.Client, authService *authusecases.AuthUsecase, requestService *requestfeature.Service, tripService *tripfeature.Service, bookingService *bookingfeature.Service, mediaClient *mediaclient.Client) *gin.Engine {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(httpx.RequestID())
	engine.Use(middleware.RequestLogger())
	engine.Use(httpx.Recovery())
	engine.Use(httpx.ErrorHandler())

	registerHealthRoutes(engine, cfg, func(ctx context.Context) error {
		if err := db.Ping(ctx); err != nil {
			return authhttp.ServiceUnavailable("PostgreSQL is not ready.", err)
		}
		if err := redisClient.Ping(ctx).Err(); err != nil {
			return authhttp.ServiceUnavailable("Redis is not ready.", err)
		}
		return nil
	})

	authhttp.RegisterRoutes(engine.Group("/api/v1/auth"), authService)
	profileRepository := profile.NewPostgresRepository(db)
	profileEventPublisher := profile.NewRedisProfileEventPublisher(redisClient)
	profileService := profile.NewServiceWithSessionRevoker(
		profileRepository,
		profileEventPublisher,
		&dispatchSessionRevokerAdapter{repo: authrepositories.NewPostgresSessionRepository(db)},
	)
	avatarUploader, err := profile.NewAvatarUploaderFromEnv(cfg.AppEnv, mediaClient)
	if err != nil {
		panic(fmt.Errorf("configure avatar storage: %w", err))
	}
	profile.RegisterRoutes(engine, authService.TokenUsecase(), profile.NewHandlerWithUploader(profileService, avatarUploader))
	profile.StartSubscribers(context.Background(), redisClient, profileRepository)

	verificationRepository := verification.NewPostgresRepository(db)
	verificationUploader, err := verification.NewUploaderFromEnv(cfg.AppEnv, mediaClient)
	if err != nil {
		panic(fmt.Errorf("configure verification storage: %w", err))
	}
	verificationHandler := verification.NewHandlerWithService(
		verification.NewService(
			verificationRepository,
			verification.NewSmileIdentityClientFromEnv(cfg.AppEnv),
			verification.WithUploader(verificationUploader),
			verification.WithEventPublisher(verification.NewRedisEventPublisher(redisClient)),
		),
	)
	verification.RegisterRoutes(engine, authService.TokenUsecase(), verificationHandler)
	verification.StartSubscribers(context.Background(), redisClient, verificationRepository)

	vehicleUploader, err := vehicle.NewVehicleUploaderFromEnv(cfg.AppEnv, mediaClient)
	if err != nil {
		panic(fmt.Errorf("configure vehicle storage: %w", err))
	}
	vehicleHandler := vehicle.NewHandler(db, vehicleUploader, vehicle.NewRedisEventPublisher(redisClient))
	vehicle.RegisterRoutes(engine, authService.TokenUsecase(), vehicleHandler)
	vehicle.StartSubscribers(context.Background(), redisClient, vehicle.NewPostgresRepository(db))

	availabilityRepository := availability.NewPostgresRepository(db)
	availabilityLiveStore := availability.NewRedisLiveStore(redisClient)
	restrictionService := restrictions.New(db)
	availabilityHandler := availability.NewHandlerWithService(
		availability.NewService(
			availabilityRepository,
			availabilityLiveStore,
			availability.WithEventPublisher(availability.NewRedisEventPublisher(redisClient)),
			availability.WithRestrictionGate(restrictionService),
		),
		authService.TokenUsecase(),
		redisClient,
	)
	availability.RegisterRoutes(engine, authService.TokenUsecase(), cfg.InternalServiceKey, availabilityHandler)
	availability.StartSubscribers(context.Background(), redisClient, availabilityRepository, availabilityLiveStore)

	if requestService != nil {
		requestfeature.RegisterRoutes(engine, authService.TokenUsecase(), requestfeature.NewHandler(requestService))
	}
	if tripService != nil {
		tripfeature.RegisterRoutes(engine, authService.TokenUsecase(), tripfeature.NewHandler(tripService))
	}
	if bookingService != nil {
		customerSigner := sharedauth.NewTokenSigner(cfg.CustomerTokenSecret)
		bookingfeature.RegisterRoutes(
			engine,
			customerSigner,
			bookingfeature.NewHandler(bookingService, profileService),
		)
		// Live-track websocket for the customer's trip map. Needs Redis for
		// the rider location pub/sub; without it the app keeps polling
		// GET /bookings/:id/location.
		if redisClient != nil {
			bookingfeature.RegisterTrackRoute(
				engine,
				bookingfeature.NewTrackHandler(bookingService, customerSigner, redisClient),
			)
		}
	}

	// Internal identity lookup for support-dispute-service (HMAC service auth).
	// Skipped when no service secrets are configured.
	if secrets := serviceauth.ParseSecrets(cfg.ServiceSecrets); len(secrets) > 0 {
		identityhttp.RegisterIdentityRoutes(
			engine,
			profileRepository,
			serviceauth.NewVerifier(secrets, 5*time.Minute),
		)
	}

	// Notification proxy (feed / realtime token / device registration) for the
	// rider and customer apps. Skipped when DISPATCH_NOTIFICATION_URL is unset.
	notificationhttp.RegisterNotificationRoutes(
		engine,
		notifications.Client{
			BaseURL:     cfg.NotificationURL,
			ServiceName: "driver-dispatch-delivery-service",
			Secret:      cfg.NotificationSecret,
		},
		sharedauth.NewTokenSigner(cfg.AccessTokenSecret),
		sharedauth.NewTokenSigner(cfg.CustomerTokenSecret),
	)

	registerMarketplaceRoutes(engine, cfg, db, authService, tripService)

	return engine
}

// registerMarketplaceRoutes wires the organization marketplace surface.
//
// Every cross-service client is built from configuration and falls back to its
// fail-closed variant when that configuration is incomplete. A deployment
// missing the fleet secret therefore refuses provider actions and reads rather
// than half-working — it never fabricates success or an empty list.
func registerMarketplaceRoutes(engine *gin.Engine, cfg config.Config, db *pgxpool.Pool,
	authService *authusecases.AuthUsecase, tripService *tripfeature.Service) {
	flags := marketplace.Flags{
		InternalAPIEnabled:         cfg.MarketplaceInternalAPIEnabled,
		ReservationEnabled:         cfg.MarketplaceReservationEnabled,
		ExecutionEnabled:           cfg.MarketplaceExecutionEnabled,
		OutboxEnabled:              cfg.MarketplaceOutboxEnabled,
		ProviderEligibilityEnabled: cfg.MarketplaceProviderEligibilityEnabled,
		VehicleEligibilityEnabled:  cfg.MarketplaceVehicleEligibilityEnabled,
		ProviderOffersEnabled:      cfg.ProviderOrganizationOffersEnabled,
		CompanyAssignEnabled:       cfg.ProviderCompanyAssignmentsEnabled,
		FleetRelayEnabled:          cfg.FleetRelayEnabled,
	}

	fleetClientConfig := marketplace.ResponderConfig{
		BaseURL:     cfg.FleetServiceURL,
		ServiceName: cfg.FleetServiceCaller,
		Secret:      cfg.FleetServiceSecret,
		Timeout:     cfg.FleetServiceTimeout,
	}
	responder, responderReady := marketplace.NewHTTPFleetResponder(fleetClientConfig)
	reader, readerReady := marketplace.NewHTTPProviderReader(fleetClientConfig)
	verifier, verifierReady := marketplace.NewHTTPAcceptanceVerifier(marketplace.VerifierConfig{
		BaseURL:     cfg.FleetServiceURL,
		ServiceName: cfg.FleetServiceCaller,
		Secret:      cfg.FleetServiceSecret,
		Timeout:     cfg.FleetServiceTimeout,
	})
	if !responderReady || !readerReady || !verifierReady {
		log.Printf("marketplace: fleet client configuration incomplete; provider actions and reads fail closed")
	}

	repository := marketplace.NewRepository(db)
	restrictionService := restrictions.New(db)
	service := marketplace.NewService(repository, flags, verifier).
		WithResponder(responder, cfg.FleetRelayRetryLimit).
		WithProviderReader(reader)
	internalVerifier := serviceauth.NewVerifier(serviceauth.Secrets{
		"fleet-partner-service": cfg.FleetServiceSecret,
	}, 5*time.Minute)
	service.WithRestrictionGate(restrictionService)
	marketplace.RegisterInternalRoutes(engine.Group("/api/v1/internal/marketplace"),
		marketplace.NewHandler(service, flags), internalVerifier, []string{"fleet-partner-service"})

	// The private vehicle authority contract. Mounted unconditionally under the
	// same verified identity as the marketplace surface: this service owns
	// vehicles, and a deployment where fleet can reserve a request but cannot
	// ask whether the vehicle may be used would push fleet into guessing.
	authority.RegisterAuthorityRoutes(engine.Group("/api/v1/internal/authority"),
		authority.NewAuthorityHandler(authority.NewAuthorityReader(db)),
		internalVerifier, authority.NewPostgresNonceClaimer(db),
		[]string{"fleet-partner-service"})
	restrictions.Register(engine.Group("/api/v1/internal/restrictions"), restrictionService,
		internalVerifier, authority.NewPostgresNonceClaimer(db), []string{"fleet-partner-service"})

	privateReady := cfg.PrivateExecutionEnabled && cfg.MarketplaceInternalAPIEnabled &&
		cfg.MarketplaceReservationEnabled && cfg.MarketplaceExecutionEnabled &&
		cfg.MarketplaceProviderEligibilityEnabled && cfg.MarketplaceVehicleEligibilityEnabled &&
		cfg.MarketplaceOutboxEnabled && tripService != nil
	if cfg.PrivateExecutionEnabled && !privateReady {
		log.Printf("private execution: enabled with incomplete lifecycle flags; contract fails closed")
	}
	privateexecution.Register(engine.Group("/api/v1/internal/private-executions"),
		privateexecution.NewHandler(privateexecution.NewService(
			privateexecution.NewRepository(db), service, tripService,
			privateReady, cfg.PrivateExecutionKillSwitch)),
		internalVerifier, db, []string{"fleet-partner-service"})

	marketplace.RegisterProviderRoutes(engine, authService.TokenUsecase(),
		marketplace.NewProviderHandler(service, flags))
}

// StartMarketplaceWorkers launches the relay worker and returns a stop
// function. It starts nothing when the feature flag is off or the fleet client
// is unconfigured, so an incomplete deployment does not spin a worker that can
// only fail.
func StartMarketplaceWorkers(ctx context.Context, cfg config.Config, db *pgxpool.Pool) func() {
	stopOutbox := startOutboxWorker(ctx, cfg, db)

	if !cfg.FleetRelayEnabled {
		log.Printf("marketplace: relay worker disabled by DISPATCH_FLEET_RELAY_ENABLED")
		return stopOutbox
	}
	responder, ready := marketplace.NewHTTPFleetResponder(marketplace.ResponderConfig{
		BaseURL:     cfg.FleetServiceURL,
		ServiceName: cfg.FleetServiceCaller,
		Secret:      cfg.FleetServiceSecret,
		Timeout:     cfg.FleetServiceTimeout,
	})
	if !ready {
		// Provider decisions are still recorded durably by the HTTP layer and
		// will be delivered once configuration is corrected and the worker
		// starts; nothing is lost by not running it now.
		log.Printf("marketplace: relay worker not started; fleet client configuration incomplete")
		return stopOutbox
	}

	worker := marketplace.NewRelayWorker(marketplace.NewRepository(db), responder,
		marketplace.RelayWorkerConfig{
			Owner:       cfg.ServiceName,
			Lease:       cfg.FleetRelayLeaseDuration,
			BatchSize:   cfg.FleetRelayBatchSize,
			MaxAttempts: cfg.FleetRelayRetryLimit,
			BaseBackoff: cfg.FleetRelayBackoffBase,
		})

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Run returns on cancellation; lease recovery runs at the top of each
		// pass, so a previous process's abandoned work is picked up here.
		_ = worker.Run(workerCtx, cfg.FleetRelayInterval)
	}()

	log.Printf("marketplace: relay worker started")
	return func() {
		cancel()
		<-done
		log.Printf("marketplace: relay worker stopped")
		stopOutbox()
	}
}

// startOutboxWorker launches canonical event delivery and returns a stopper.
//
// Two conditions gate it, and they mean different things. The flag being off
// is a decision: events accumulate in the outbox and are delivered whenever it
// is turned on, losing nothing. Incomplete fleet configuration is a mistake:
// the worker would claim rows, fail every delivery and burn the attempt budget
// until they dead-letter, so it is better not to run at all and say why.
func startOutboxWorker(ctx context.Context, cfg config.Config, db *pgxpool.Pool) func() {
	if !cfg.EventDeliveryEnabled {
		log.Printf("marketplace: outbox delivery worker disabled by DISPATCH_EVENT_DELIVERY_ENABLED")
		return func() {}
	}

	deliverer, ready := marketplace.NewHTTPEventDeliverer(marketplace.DelivererConfig{
		BaseURL:     cfg.FleetServiceURL,
		Path:        cfg.EventDeliveryPath,
		ServiceName: cfg.FleetServiceCaller,
		Secret:      string(cfg.FleetServiceSecret),
		Timeout:     cfg.FleetServiceTimeout,
	})
	if !ready {
		log.Printf("marketplace: outbox delivery worker not started; fleet client configuration incomplete")
		return func() {}
	}

	worker := marketplace.NewOutboxWorker(marketplace.NewRepository(db), deliverer,
		marketplace.OutboxWorkerConfig{
			Owner:         cfg.ServiceName,
			SourceService: cfg.ServiceName,
			Target:        "fleet-partner-service",
			Lease:         cfg.EventDeliveryLease,
			BatchSize:     cfg.EventDeliveryBatchSize,
			MaxAttempts:   cfg.EventDeliveryRetryLimit,
			BaseBackoff:   cfg.EventDeliveryBackoffBase,
			MaxBackoff:    cfg.EventDeliveryBackoffMax,
		})

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = worker.Run(workerCtx, cfg.EventDeliveryInterval)
	}()

	log.Printf("marketplace: outbox delivery worker started")
	return func() {
		cancel()
		<-done
		log.Printf("marketplace: outbox delivery worker stopped")
	}
}

func registerHealthRoutes(engine *gin.Engine, cfg config.Config, ready func(context.Context) error) {
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"service": cfg.ServiceName,
				"status":  "ok",
			},
		})
	})

	engine.GET("/ready", func(c *gin.Context) {
		if err := ready(c.Request.Context()); err != nil {
			httpx.Abort(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"service": cfg.ServiceName,
				"status":  "ready",
			},
		})
	})
}
