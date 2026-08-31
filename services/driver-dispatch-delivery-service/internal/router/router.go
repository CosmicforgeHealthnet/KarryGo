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

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/apiresponse"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/assignment"
	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authrepositories "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	bookingfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking"
	bookingclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking/clients"
	dispatchcommercemission "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercemission"
	dispatchcommercequote "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/deliveryrecovery"
	identityhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/identity/http"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/missionplanning"
	notificationhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/notifications/http"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/privateexecution"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/profile"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/restrictions"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/routeintegrity"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/securehandover"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/serviceability"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/vehicle"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/verification"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/middleware"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/providercompat"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/walletclient"
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
	if cfg.ProviderOrganizationModeEnabled {
		engine.Use(apiresponse.ProviderPrivacy(providercompat.NewClient(cfg.FleetServiceURL, cfg.FleetServiceCaller, cfg.FleetServiceSecret, cfg.FleetServiceTimeout)))
	}

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

	// The internal commerce delivery-quote contract (Vendor Service Section
	// 17C). It answers serviceability and price for a route; it creates no
	// booking and reserves no rider, so it sits with the read-shaped private
	// surfaces rather than the execution ones.
	//
	// It gets its OWN verifier trusting exactly one caller with exactly one
	// secret. Sharing the fleet verifier would have made a second caller
	// presentable at every private surface's door, relying on each allowlist
	// to turn it away; a separate verifier means the commerce secret opens
	// nothing else even if an allowlist is later widened by mistake.
	//
	// Registration is skipped entirely when no vendor secret is configured, so
	// a deployment that does not serve commerce quotes exposes no route to
	// probe rather than one that answers 401 and confirms it exists.
	if len(cfg.VendorServiceSecret) > 0 {
		quoteRepository := dispatchcommercequote.NewRepository(db)
		dispatchcommercequote.Register(
			engine.Group("/api/v1/internal"),
			dispatchcommercequote.NewHandler(dispatchcommercequote.New(
				dispatchcommercequote.Options{
					Store:    quoteRepository,
					Live:     availabilityLiveStore,
					RadiusKM: cfg.AvailabilityRadiusKM,
					TTL:      cfg.CommerceQuoteTTL,
				},
			)),
			serviceauth.NewVerifier(serviceauth.Secrets{
				commercequote.SignatureCaller: cfg.VendorServiceSecret,
			}, 5*time.Minute),
			authority.NewPostgresNonceClaimer(db),
			[]string{commercequote.SignatureCaller},
		)
		dispatchcommercequote.RegisterMultiStop(
			engine.Group("/api/v1/internal"),
			dispatchcommercequote.NewMultiStopHandler(dispatchcommercequote.NewMultiStop(
				dispatchcommercequote.MultiStopOptions{
					Store:  quoteRepository,
					Routes: dispatchcommercequote.NewHTTPRoutePlanner(cfg.RouteServiceURL, cfg.RouteServiceTimeout),
					Live:   availabilityLiveStore, RadiusKM: cfg.AvailabilityRadiusKM,
					TTL: cfg.CommerceQuoteTTL,
				},
			)),
			serviceauth.NewVerifier(serviceauth.Secrets{dispatchcommerce.SignatureCaller: cfg.VendorServiceSecret}, 5*time.Minute),
			authority.NewPostgresNonceClaimer(db), []string{dispatchcommerce.SignatureCaller},
		)
	}

	// The internal commerce mission contract (Vendor Service Section 17F).
	// Unlike the quote surface above this one EXECUTES: it creates a real
	// booking and starts rider matching for a fee the customer has already
	// paid. It therefore needs the request service, and is skipped entirely
	// without it rather than accepting missions nothing would ever match.
	if len(cfg.VendorServiceSecret) > 0 && requestService != nil {
		commerceEvents := marketplace.NewLegacyEventEmitter(marketplace.NewRepository(db), true)
		missionRepository := dispatchcommercemission.NewRepository(db).WithTargetEventEmitter(commerceEvents)
		requestService.WithProviderAcceptanceGate(missionRepository).WithCommerceJobCards(missionRepository)
		dispatchcommercemission.Register(
			engine.Group("/api/v1/internal"),
			dispatchcommercemission.NewHandler(dispatchcommercemission.New(
				dispatchcommercemission.Options{
					Store:     missionRepository,
					Broadcast: requestService,
				},
			)),
			serviceauth.NewVerifier(serviceauth.Secrets{
				commercemission.SignatureCaller: cfg.VendorServiceSecret,
			}, 5*time.Minute),
			authority.NewPostgresNonceClaimer(db),
			[]string{commercemission.SignatureCaller},
		)
		multiStopMissionService := dispatchcommercemission.NewMultiStopMission(dispatchcommercemission.MultiStopMissionOptions{
			Store: missionRepository, Quotes: dispatchcommercequote.NewRepository(db), Broadcast: requestService,
			Notifier: bookingclients.NewBookingNotifier(cfg.NotificationURL, cfg.NotificationSecret),
		}).WithHandoverSecret(cfg.HandoverSecret).
			WithMediaAttachmentAuthorizer(newMediaAttachmentAuthorizer(mediaClient))
		multiStopMissionHandler := dispatchcommercemission.NewMultiStopMissionHandler(multiStopMissionService)
		operationalHandler := dispatchcommercemission.NewOperationalHandler(
			dispatchcommercemission.NewOperationalService(missionRepository, cfg.HandoverSecret, nil).
				WithReplacementAuthority(multiStopMissionService).
				WithMediaAttachmentAuthorizer(newMediaAttachmentAuthorizer(mediaClient)),
		)
		dispatchcommercemission.RegisterMultiStopMissions(
			engine.Group("/api/v1/internal"),
			multiStopMissionHandler,
			serviceauth.NewVerifier(serviceauth.Secrets{dispatchcommerce.SignatureCaller: cfg.VendorServiceSecret}, 5*time.Minute),
			authority.NewPostgresNonceClaimer(db), []string{dispatchcommerce.SignatureCaller},
		)
		dispatchcommercemission.RegisterProviderPickupRoutes(engine, authService.TokenUsecase(), multiStopMissionHandler)
		dispatchcommercemission.RegisterProviderOperationalRoutes(engine, authService.TokenUsecase(), operationalHandler)
		dispatchcommercemission.RegisterInternalOperationalRoutes(
			engine.Group("/api/v1/internal"), operationalHandler,
			serviceauth.NewVerifier(serviceauth.Secrets{dispatchcommerce.SignatureCaller: cfg.VendorServiceSecret}, 5*time.Minute),
			authority.NewPostgresNonceClaimer(db), []string{dispatchcommerce.SignatureCaller},
		)
	}

	// Sections 7-8 are independently gated and additive. The legacy request
	// routes above remain registered regardless of these flags.
	if len(cfg.VendorServiceSecret) > 0 && cfg.MultiVendorPlanningEnabled && !cfg.AssignmentKillSwitch {
		planningRoutes := missionplanning.NewSection6RouteAuthority(
			dispatchcommercequote.NewHTTPRoutePlanner(cfg.RouteServiceURL, cfg.RouteServiceTimeout),
			serviceability.NewCircularZone(serviceability.Point{Latitude: cfg.ServiceZoneCenterLat, Longitude: cfg.ServiceZoneCenterLng}, cfg.ServiceZoneRadiusKM, cfg.ServiceRestrictedZones),
			cfg.ServiceabilityDegradedMode,
		)
		missionplanning.Register(engine.Group("/api/v1/internal"),
			missionplanning.NewHandler(missionplanning.New(missionplanning.NewRepository(db), missionplanning.NewPlannerWithCapacity(cfg.PlanningMaxWeightGrams, cfg.PlanningMaxVolumeML, cfg.PlanningCapacityPolicyVersion), true).WithRouteAuthority(planningRoutes)),
			serviceauth.NewVerifier(serviceauth.Secrets{dispatchcommerce.SignatureCaller: cfg.VendorServiceSecret}, 5*time.Minute),
			db, []string{dispatchcommerce.SignatureCaller})
	}
	var assignmentRuntime *assignment.Service
	if (len(cfg.VendorServiceSecret) > 0 || cfg.ReplacementHandoverEnabled) && (cfg.SequentialAssignmentEnabled || cfg.OrganizationAssignmentEnabled || cfg.AdminAssistedAssignmentEnabled) && !cfg.AssignmentKillSwitch && tripService != nil {
		repository := assignment.NewRepository(db)
		service := assignment.New(repository, assignment.NewCanonicalTripAdapter(db, tripService, repository), assignment.Policy{
			SequentialEnabled: cfg.SequentialAssignmentEnabled, QueueEnabled: cfg.DispatchQueueEnabled,
			OrganizationEnabled: cfg.OrganizationAssignmentEnabled,
			AdminEnabled:        cfg.AdminAssistedAssignmentEnabled, OfferWindow: cfg.AssignmentOfferWindow,
			QueueRetryInterval: cfg.AssignmentQueueRetryInterval, InitialRadiusKM: cfg.AvailabilityRadiusKM, MaxRadiusKM: 50,
			ReconciliationLease: cfg.ExternalOperationLease, ReconciliationMaxAttempts: cfg.ExternalOperationRetryLimit,
			ReconciliationBackoffBase: cfg.ExternalOperationBackoffBase, ReconciliationBackoffMax: cfg.ExternalOperationBackoffMax,
		}).WithCandidateFinder(assignment.NewCandidateAuthority(db, availabilityLiveStore, cfg.AvailabilityRadiusKM)).WithQueueFinancialAuthority(assignment.NewBookingFinancialAdapter(db, bookingService))
		assignmentRuntime = service
		if cfg.OrganizationAssignmentEnabled && cfg.FleetServiceURL != "" && len(cfg.FleetServiceSecret) > 0 {
			service.WithOrganizationAuthority(assignment.NewFleetOrganizationAuthority(providercompat.NewClient(cfg.FleetServiceURL, cfg.FleetServiceCaller, cfg.FleetServiceSecret, cfg.FleetServiceTimeout)))
		}
		handler := assignment.NewHandler(service)
		if len(cfg.VendorServiceSecret) > 0 {
			assignment.RegisterInternal(engine.Group("/api/v1/internal"), handler,
				serviceauth.NewVerifier(serviceauth.Secrets{dispatchcommerce.SignatureCaller: cfg.VendorServiceSecret}, 5*time.Minute), db, []string{dispatchcommerce.SignatureCaller})
		}
		assignment.RegisterProvider(engine, authService.TokenUsecase(), handler)
		if cfg.DispatchQueueEnabled {
			assignment.RegisterCustomer(engine, sharedauth.NewTokenSigner(cfg.CustomerTokenSecret), handler)
			if secrets := serviceauth.ParseSecrets(cfg.ServiceSecrets); len(secrets) > 0 {
				if secret, ok := secrets["api-gateway"]; ok {
					assignment.RegisterPartnerInternal(engine.Group("/api/v1/internal"), handler, serviceauth.NewVerifier(serviceauth.Secrets{"api-gateway": secret}, 5*time.Minute), db, []string{"api-gateway"})
				}
			}
		}
		if cfg.AdminAssistedAssignmentEnabled {
			if secrets := serviceauth.ParseSecrets(cfg.ServiceSecrets); len(secrets) > 0 {
				if secret, ok := secrets["admin-backoffice-service"]; ok {
					assignment.RegisterAdminInternal(engine.Group("/api/v1/internal"), handler, serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": secret}, 5*time.Minute), db, []string{"admin-backoffice-service"})
				}
			}
		}
	}

	// Sections 9-10 are a separate, disabled-by-default security authority.
	// It binds the existing Trip to the Sections 7-8 mission/manifest/custody
	// model and replaces no legacy route while disabled.
	var handoverRuntime *securehandover.Service
	if cfg.HandoverChallengeEnabled && tripService != nil {
		handoverRepository, handoverErr := securehandover.NewRepository(db, cfg.HandoverSecret)
		if handoverErr != nil {
			panic(fmt.Errorf("configure secure handover repository: %w", handoverErr))
		}
		handoverService, handoverErr := securehandover.NewService(handoverRepository, cfg.HandoverSecret, securehandover.Policy{
			Enabled: cfg.HandoverChallengeEnabled, PickupEvidenceEnabled: cfg.PickupEvidenceGateEnabled,
			DeliveryEvidenceEnabled: cfg.DeliveryEvidenceGateEnabled, QREnabled: cfg.HandoverQREnabled,
			TTL: cfg.HandoverChallengeTTL, CaptureTTL: cfg.HandoverCaptureSessionTTL,
			ResendCooldown: cfg.HandoverResendCooldown, MaxAttempts: cfg.HandoverMaxAttempts,
			MaxResends: cfg.HandoverMaxResends, PolicyVersion: cfg.HandoverPolicyVersion,
			EmailAllowed: cfg.HandoverEmailAllowed, InAppAllowed: cfg.HandoverInAppAllowed,
			PushAllowed: cfg.HandoverPushAllowed, SMSAllowed: cfg.HandoverSMSAllowed,
			ReconciliationLease: cfg.ExternalOperationLease, ReconciliationMaxAttempts: cfg.ExternalOperationRetryLimit,
			ReconciliationBackoffBase: cfg.ExternalOperationBackoffBase, ReconciliationBackoffMax: cfg.ExternalOperationBackoffMax,
		})
		if handoverErr != nil {
			panic(fmt.Errorf("configure secure handover service: %w", handoverErr))
		}
		handoverService.WithRateLimiter(securehandover.NewRedisRateLimiter(redisClient)).
			WithMediaAuthorizer(newMediaAttachmentAuthorizer(mediaClient))
		if cfg.NotificationURL != "" && len(cfg.NotificationSecret) > 0 {
			handoverService.WithNotifier(securehandover.NewNotificationTransport(notifications.Client{
				BaseURL: cfg.NotificationURL, ServiceName: "driver-dispatch-delivery-service", Secret: cfg.NotificationSecret, HTTPClient: &http.Client{Timeout: cfg.ExternalServiceTimeout},
			}))
		}
		tripService.WithSecureHandover(handoverService)
		handoverRuntime = handoverService
		handoverHandler := securehandover.NewHandler(handoverService)
		securehandover.RegisterProvider(engine, authService.TokenUsecase(), handoverHandler)
		securehandover.RegisterCustomer(engine, sharedauth.NewTokenSigner(cfg.CustomerTokenSecret), handoverHandler)
		internalSecrets := serviceauth.Secrets{}
		if len(cfg.VendorServiceSecret) > 0 {
			internalSecrets[dispatchcommerce.SignatureCaller] = cfg.VendorServiceSecret
		}
		if secret, ok := serviceauth.ParseSecrets(cfg.ServiceSecrets)["api-gateway"]; ok {
			internalSecrets["api-gateway"] = secret
		}
		if len(internalSecrets) > 0 {
			allowed := make([]string, 0, len(internalSecrets))
			for caller := range internalSecrets {
				allowed = append(allowed, caller)
			}
			securehandover.RegisterInternal(engine.Group("/api/v1/internal"), handoverHandler,
				serviceauth.NewVerifier(internalSecrets, 5*time.Minute), db, allowed)
		}
	}

	// Delivery recovery extends the canonical Trip and existing handover,
	// assignment, route, Wallet and commerce-mission authorities. Registration
	// is feature-gated and therefore cannot alter legacy failed-delivery flows.
	if cfg.DeliveryRecoveryEnabled && tripService != nil {
		recovery := deliveryrecovery.New(deliveryrecovery.NewRepository(db), deliveryrecovery.Policy{
			Enabled: true, ReturnEnabled: cfg.ReturnWorkflowEnabled, HubHoldEnabled: cfg.HubHoldEnabled,
			ReplacementEnabled: cfg.ReplacementHandoverEnabled, GracePeriod: cfg.ReceiverGracePeriod,
			MaximumSameDayWindow: cfg.RecoverySameDayWindow,
		}).WithHandover(deliveryrecovery.NewTripHandoverInvalidator(db, handoverRuntime)).
			WithRoutes(deliveryrecovery.NewSection6ReturnRouter(
				dispatchcommercequote.NewHTTPRoutePlanner(cfg.RouteServiceURL, cfg.RouteServiceTimeout),
				serviceability.NewCircularZone(serviceability.Point{Latitude: cfg.ServiceZoneCenterLat, Longitude: cfg.ServiceZoneCenterLng}, cfg.ServiceZoneRadiusKM, cfg.ServiceRestrictedZones),
			)).
			WithHubAuthority(deliveryrecovery.NewConfiguredHubAuthority(cfg.ApprovedHoldHubs)).
			WithHubEvidenceAuthority(newMediaAttachmentAuthorizer(mediaClient)).
			WithFinancialAuthority(deliveryrecovery.NewWalletRecoveryAuthority(walletclient.Client{BaseURL: cfg.WalletServiceURL, ServiceName: cfg.WalletServiceSource, Secret: cfg.WalletServiceSecret, HTTPClient: &http.Client{Timeout: cfg.ExternalServiceTimeout}}))
		if assignmentRuntime != nil {
			recovery.WithReplacement(deliveryrecovery.NewAssignmentReplacementAdapter(assignmentRuntime))
		}
		recoveryHandler := deliveryrecovery.NewHandler(recovery)
		deliveryrecovery.RegisterProvider(engine, authService.TokenUsecase(), recoveryHandler)
		deliveryrecovery.RegisterCustomer(engine, sharedauth.NewTokenSigner(cfg.CustomerTokenSecret), recoveryHandler)
		if cfg.ReplacementHandoverEnabled {
			if secret, ok := serviceauth.ParseSecrets(cfg.ServiceSecrets)["admin-backoffice-service"]; ok {
				deliveryrecovery.RegisterEmergencyInternal(engine.Group("/api/v1/internal"), recoveryHandler,
					serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": secret}, 5*time.Minute), db, []string{"admin-backoffice-service"})
			}
		}
	}

	if cfg.RouteAlertsEnabled && tripService != nil {
		routeService := routeintegrity.New(routeintegrity.NewRepository(db),
			routeintegrity.NewSection6RouteAuthority(dispatchcommercequote.NewHTTPRoutePlanner(cfg.RouteServiceURL, cfg.RouteServiceTimeout)),
			routeintegrity.Policy{Enabled: true, OfflineReplayEnabled: cfg.OfflineReplayEnabled, CorridorMeters: cfg.RouteDeviationMeters,
				WrongWayToleranceDegrees: cfg.RouteWrongWayToleranceDegrees, StationaryThreshold: cfg.RouteStationaryThreshold,
				StaleThreshold: cfg.RouteStaleThreshold, HighRiskCorridorMeters: cfg.RouteHighRiskDeviationMeters,
				HighRiskStaleThreshold: cfg.RouteHighRiskStaleThreshold, MaximumAccuracyMeters: cfg.RouteMaximumAccuracyMeters,
				MaximumClientClockSkew: cfg.OfflineMaximumClientClockSkew})
		routeHandler := routeintegrity.NewHandler(routeService)
		routeintegrity.RegisterProvider(engine, authService.TokenUsecase(), routeHandler)
		routeintegrity.RegisterCustomer(engine, sharedauth.NewTokenSigner(cfg.CustomerTokenSecret), routeHandler)
		routeintegrity.StartSubscribers(context.Background(), redisClient, routeService)
	}

	if requestService != nil {
		requestfeature.RegisterRoutes(engine, authService.TokenUsecase(), requestfeature.NewHandler(requestService))
	}
	if tripService != nil {
		tripfeature.RegisterRoutes(engine, authService.TokenUsecase(), tripfeature.NewHandler(tripService))
	}
	if bookingService != nil {
		bookingService.WithServiceabilityQuoteAuthority(
			serviceability.NewRepository(db),
			cfg.RouteServiceabilityEnabled && cfg.PackagePolicyEnabled,
		)
		bookingfeature.RegisterRoutes(
			engine,
			sharedauth.NewTokenSigner(cfg.CustomerTokenSecret),
			bookingfeature.NewHandler(bookingService, profileService),
		)
	}

	// Sections 4-6 are a gated pre-booking decision surface. Registration
	// requires both policies, so merely deploying the additive schema cannot
	// alter or expose an unfinished path to legacy customers.
	if cfg.RouteServiceabilityEnabled && cfg.PackagePolicyEnabled {
		serviceability.RegisterRoutes(
			engine,
			sharedauth.NewTokenSigner(cfg.CustomerTokenSecret),
			serviceability.NewHandler(serviceability.New(serviceability.Options{
				Store: serviceability.NewRepository(db),
				Routes: serviceability.NewRoutePlannerAdapter(
					dispatchcommercequote.NewHTTPRoutePlanner(cfg.RouteServiceURL, cfg.RouteServiceTimeout),
				),
				Zones: serviceability.NewCircularZone(
					serviceability.Point{Latitude: cfg.ServiceZoneCenterLat, Longitude: cfg.ServiceZoneCenterLng},
					cfg.ServiceZoneRadiusKM,
					cfg.ServiceRestrictedZones,
				),
				TTL:          cfg.ServiceabilityTTL,
				MaxRouteKM:   cfg.ServiceabilityMaxRouteKM,
				DegradedMode: cfg.ServiceabilityDegradedMode,
			})),
		)
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
func StartMarketplaceWorkers(ctx context.Context, cfg config.Config, db *pgxpool.Pool, trips ...*tripfeature.Service) func() {
	return startMarketplaceWorkers(ctx, cfg, db, nil, nil, trips...)
}

func StartMarketplaceWorkersWithAssignment(ctx context.Context, cfg config.Config, db *pgxpool.Pool, live availability.LiveStore, bookings *bookingfeature.Service, trips ...*tripfeature.Service) func() {
	return startMarketplaceWorkers(ctx, cfg, db, live, bookings, trips...)
}

func startMarketplaceWorkers(ctx context.Context, cfg config.Config, db *pgxpool.Pool, live availability.LiveStore, bookings *bookingfeature.Service, trips ...*tripfeature.Service) func() {
	stopTelemetry := observability.StartCollector(ctx, db, cfg.MetricsInterval)
	stopOutbox := startOutboxWorker(ctx, cfg, db)
	stopCommerceOutbox := startCommerceOutboxWorker(ctx, cfg, db)
	stopAnalytics := startAnalyticsOutboxWorker(ctx, cfg, db)
	stopAssignment := startAssignmentWorker(ctx, cfg, db, live, bookings, trips...)
	stopHandover := startSecureHandoverWorker(ctx, cfg, db)
	stopRecovery := startDeliveryRecoveryWorker(ctx, cfg, db, live, bookings, trips...)
	stopRecoveryNotifications := startRecoveryNotificationWorker(ctx, cfg, db)
	stopEventWorkers := func() {
		stopTelemetry()
		stopAnalytics()
		stopRecoveryNotifications()
		stopRecovery()
		stopHandover()
		stopAssignment()
		stopCommerceOutbox()
		stopOutbox()
	}

	if !cfg.FleetRelayEnabled {
		log.Printf("marketplace: relay worker disabled by DISPATCH_FLEET_RELAY_ENABLED")
		return stopEventWorkers
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
		return stopEventWorkers
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
		stopEventWorkers()
	}
}

func startDeliveryRecoveryWorker(ctx context.Context, cfg config.Config, db *pgxpool.Pool, live availability.LiveStore, bookings *bookingfeature.Service, trips ...*tripfeature.Service) func() {
	if !cfg.DeliveryRecoveryEnabled || db == nil {
		return func() {}
	}
	recovery := deliveryrecovery.New(deliveryrecovery.NewRepository(db), deliveryrecovery.Policy{
		Enabled: true, ReturnEnabled: cfg.ReturnWorkflowEnabled, HubHoldEnabled: cfg.HubHoldEnabled,
		ReplacementEnabled: cfg.ReplacementHandoverEnabled, GracePeriod: cfg.ReceiverGracePeriod,
		MaximumSameDayWindow: cfg.RecoverySameDayWindow,
	}).WithReliabilityPolicy(cfg.ServiceName+"-recovery", cfg.ExternalOperationLease, cfg.ExternalOperationRetryLimit, cfg.ExternalOperationBackoffBase, cfg.ExternalOperationBackoffMax)
	if (cfg.SequentialAssignmentEnabled || cfg.OrganizationAssignmentEnabled || cfg.AdminAssistedAssignmentEnabled) && !cfg.AssignmentKillSwitch && len(trips) > 0 && trips[0] != nil {
		repository := assignment.NewRepository(db)
		assignmentService := assignment.New(repository, assignment.NewCanonicalTripAdapter(db, trips[0], repository), assignment.Policy{
			SequentialEnabled: cfg.SequentialAssignmentEnabled, OrganizationEnabled: cfg.OrganizationAssignmentEnabled, AdminEnabled: cfg.AdminAssistedAssignmentEnabled, QueueEnabled: cfg.DispatchQueueEnabled, OfferWindow: cfg.AssignmentOfferWindow,
			QueueRetryInterval: cfg.AssignmentQueueRetryInterval, InitialRadiusKM: cfg.AvailabilityRadiusKM, MaxRadiusKM: 50,
			ReconciliationLease: cfg.ExternalOperationLease, ReconciliationMaxAttempts: cfg.ExternalOperationRetryLimit,
			ReconciliationBackoffBase: cfg.ExternalOperationBackoffBase, ReconciliationBackoffMax: cfg.ExternalOperationBackoffMax,
		}).WithCandidateFinder(assignment.NewCandidateAuthority(db, live, cfg.AvailabilityRadiusKM)).WithQueueFinancialAuthority(assignment.NewBookingFinancialAdapter(db, bookings))
		if cfg.OrganizationAssignmentEnabled && cfg.FleetServiceURL != "" && len(cfg.FleetServiceSecret) > 0 {
			assignmentService.WithOrganizationAuthority(assignment.NewFleetOrganizationAuthority(providercompat.NewClient(cfg.FleetServiceURL, cfg.FleetServiceCaller, cfg.FleetServiceSecret, cfg.FleetServiceTimeout)))
		}
		recovery.WithReplacement(deliveryrecovery.NewAssignmentReplacementAdapter(assignmentService))
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); recovery.RunWorker(workerCtx, cfg.RecoveryWorkerInterval) }()
	return func() { cancel(); <-done }
}

func startRecoveryNotificationWorker(ctx context.Context, cfg config.Config, db *pgxpool.Pool) func() {
	if db == nil || cfg.NotificationURL == "" || len(cfg.NotificationSecret) == 0 {
		return func() {}
	}
	notifier := notifications.Client{
		BaseURL: cfg.NotificationURL, ServiceName: "driver-dispatch-delivery-service", Secret: cfg.NotificationSecret, HTTPClient: &http.Client{Timeout: cfg.ExternalServiceTimeout},
	}
	deliverer := bookingfeature.LifecycleNotificationDeliverer{Notifier: notifier, Other: deliveryrecovery.NewRecoveryNotificationDeliverer(deliveryrecovery.NewRepository(db), notifier)}
	worker := marketplace.NewOutboxWorker(marketplace.NewRepository(db), deliverer, marketplace.OutboxWorkerConfig{
		Owner: cfg.ServiceName + "-recovery-notifications", SourceService: "driver-dispatch-delivery-service", Target: "notification-service",
		Lease: cfg.EventDeliveryLease, BatchSize: cfg.EventDeliveryBatchSize, MaxAttempts: cfg.EventDeliveryRetryLimit,
		BaseBackoff: cfg.EventDeliveryBackoffBase, MaxBackoff: cfg.EventDeliveryBackoffMax,
	})
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); _ = worker.Run(workerCtx, cfg.EventDeliveryInterval) }()
	return func() { cancel(); <-done }
}

func startSecureHandoverWorker(ctx context.Context, cfg config.Config, db *pgxpool.Pool) func() {
	if !cfg.HandoverChallengeEnabled || db == nil {
		return func() {}
	}
	repository, err := securehandover.NewRepository(db, cfg.HandoverSecret)
	if err != nil {
		log.Printf("secure handover: expiry worker not started: %v", err)
		return func() {}
	}
	service, err := securehandover.NewService(repository, cfg.HandoverSecret, securehandover.Policy{
		Enabled: true, EmailAllowed: cfg.HandoverEmailAllowed, InAppAllowed: cfg.HandoverInAppAllowed,
		PushAllowed: cfg.HandoverPushAllowed, SMSAllowed: cfg.HandoverSMSAllowed,
		ReconciliationLease: cfg.ExternalOperationLease, ReconciliationMaxAttempts: cfg.ExternalOperationRetryLimit,
		ReconciliationBackoffBase: cfg.ExternalOperationBackoffBase, ReconciliationBackoffMax: cfg.ExternalOperationBackoffMax,
	})
	if err != nil {
		log.Printf("secure handover: expiry worker not started: %v", err)
		return func() {}
	}
	if cfg.NotificationURL != "" && len(cfg.NotificationSecret) > 0 {
		service.WithNotifier(securehandover.NewNotificationTransport(notifications.Client{
			BaseURL: cfg.NotificationURL, ServiceName: "driver-dispatch-delivery-service", Secret: cfg.NotificationSecret, HTTPClient: &http.Client{Timeout: cfg.ExternalServiceTimeout},
		}))
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.RunExpiryWorker(workerCtx, cfg.HandoverWorkerInterval)
	}()
	return func() { cancel(); <-done }
}

func startAssignmentWorker(ctx context.Context, cfg config.Config, db *pgxpool.Pool, live availability.LiveStore, bookings *bookingfeature.Service, trips ...*tripfeature.Service) func() {
	if (!cfg.SequentialAssignmentEnabled && !cfg.OrganizationAssignmentEnabled && !cfg.AdminAssistedAssignmentEnabled) || cfg.AssignmentKillSwitch || db == nil {
		return func() {}
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	repository := assignment.NewRepository(db)
	var tripAuthority assignment.TripEnsurer
	if len(trips) > 0 && trips[0] != nil {
		tripAuthority = assignment.NewCanonicalTripAdapter(db, trips[0], repository)
	}
	service := assignment.New(repository, tripAuthority, assignment.Policy{
		SequentialEnabled: cfg.SequentialAssignmentEnabled, QueueEnabled: cfg.DispatchQueueEnabled,
		OrganizationEnabled: cfg.OrganizationAssignmentEnabled, AdminEnabled: cfg.AdminAssistedAssignmentEnabled,
		OfferWindow: cfg.AssignmentOfferWindow, QueueRetryInterval: cfg.AssignmentQueueRetryInterval,
		ReconciliationLease: cfg.ExternalOperationLease, ReconciliationMaxAttempts: cfg.ExternalOperationRetryLimit,
		ReconciliationBackoffBase: cfg.ExternalOperationBackoffBase, ReconciliationBackoffMax: cfg.ExternalOperationBackoffMax,
	}).WithCandidateFinder(assignment.NewCandidateAuthority(db, live, cfg.AvailabilityRadiusKM)).WithQueueFinancialAuthority(assignment.NewBookingFinancialAdapter(db, bookings))
	go func() { defer close(done); service.RunWorker(workerCtx, cfg.AssignmentWorkerInterval) }()
	return func() { cancel(); <-done }
}

func startCommerceOutboxWorker(ctx context.Context, cfg config.Config, db *pgxpool.Pool) func() {
	if !cfg.CommerceEventDeliveryEnabled {
		log.Printf("commerce: lifecycle outbox worker disabled by DISPATCH_COMMERCE_EVENT_DELIVERY_ENABLED")
		return func() {}
	}
	deliverer, ready := marketplace.NewHTTPEventDeliverer(marketplace.DelivererConfig{
		BaseURL: cfg.VendorServiceURL, Path: cfg.CommerceEventDeliveryPath,
		ServiceName: cfg.ServiceName, Secret: string(cfg.VendorServiceSecret),
		Timeout: cfg.FleetServiceTimeout,
	})
	if !ready {
		log.Printf("commerce: lifecycle outbox worker not started; Vendor callback configuration incomplete")
		return func() {}
	}
	worker := marketplace.NewOutboxWorker(marketplace.NewRepository(db), marketplace.VendorLifecycleDeliverer{Transport: deliverer},
		marketplace.OutboxWorkerConfig{Owner: cfg.ServiceName + "-commerce-events",
			SourceService: cfg.ServiceName, Target: "vendor-service",
			Lease: cfg.EventDeliveryLease, BatchSize: cfg.EventDeliveryBatchSize,
			MaxAttempts: cfg.EventDeliveryRetryLimit, BaseBackoff: cfg.EventDeliveryBackoffBase,
			MaxBackoff: cfg.EventDeliveryBackoffMax})
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); _ = worker.Run(workerCtx, cfg.EventDeliveryInterval) }()
	log.Printf("commerce: lifecycle outbox worker started")
	return func() { cancel(); <-done; log.Printf("commerce: lifecycle outbox worker stopped") }
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
