package main

import (
	"context"
	sharehttp "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/http"
	sharerepositories "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/repositories"
	shareusecases "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/usecases"
	shareworkers "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/workers"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/observability"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/customer-service/internal/config"
	"cosmicforge/logistics/services/customer-service/internal/database"
	"cosmicforge/logistics/services/customer-service/internal/features/admincommands"
	"cosmicforge/logistics/services/customer-service/internal/features/adminresources"
	"cosmicforge/logistics/services/customer-service/internal/features/adminreview"
	authclients "cosmicforge/logistics/services/customer-service/internal/features/auth/clients"
	customerhttp "cosmicforge/logistics/services/customer-service/internal/features/auth/http"
	authrepositories "cosmicforge/logistics/services/customer-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	contexthttp "cosmicforge/logistics/services/customer-service/internal/features/customer_context/http"
	contextusecases "cosmicforge/logistics/services/customer-service/internal/features/customer_context/usecases"
	identityhttp "cosmicforge/logistics/services/customer-service/internal/features/identity/http"
	mediauploadhttp "cosmicforge/logistics/services/customer-service/internal/features/media_upload/http"
	mobilityhttp "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/http"
	mobilityrepositories "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/repositories"
	mobilityusecases "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/usecases"
	movementhttp "cosmicforge/logistics/services/customer-service/internal/features/movement_history/http"
	movementrepositories "cosmicforge/logistics/services/customer-service/internal/features/movement_history/repositories"
	movementusecases "cosmicforge/logistics/services/customer-service/internal/features/movement_history/usecases"
	notificationhttp "cosmicforge/logistics/services/customer-service/internal/features/notifications/http"
	preferences "cosmicforge/logistics/services/customer-service/internal/features/preferences"
	profileclients "cosmicforge/logistics/services/customer-service/internal/features/profile/clients"
	profilehttp "cosmicforge/logistics/services/customer-service/internal/features/profile/http"
	profilerepositories "cosmicforge/logistics/services/customer-service/internal/features/profile/repositories"
	profileusecases "cosmicforge/logistics/services/customer-service/internal/features/profile/usecases"
	savedlocationhttp "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/http"
	savedlocationrepositories "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/repositories"
	savedlocationusecases "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/phonenumber"
	"cosmicforge/logistics/shared/go/ratelimit"
	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if err := cfg.ValidateJourneySharing(); err != nil {
		logging.Fatal("config", "%v", err)
	}
	if err := cfg.ValidateMediaUploadSessions(); err != nil {
		logging.Fatal("config", "%v", err)
	}
	logging.Notice("customer-service config", "migration=%t database_configured=%t redis_configured=%t", cfg.Migration, cfg.DatabaseURL != "", cfg.Redis.Addr != "")

	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create customer database pool: %v", err)
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	logConnectivity(ctx, cfg.DatabaseURL, cfg.Redis.Addr, db, redisClient)

	if cfg.Migration {
		logging.Notice("migration", "mode enabled")
		migrationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := database.ApplyMigrations(migrationCtx, db); err != nil {
			cancel()
			logging.Fatal("migration", "apply customer-service migrations: %v", err)
		}
		cancel()
		logging.Success("migration", "applied successfully")
	}

	var mediaClient *mediaclient.Client
	if cfg.MediaBaseURL != "" && cfg.MediaServiceToken != "" {
		mediaClient = mediaclient.New(mediaclient.Config{
			BaseURL:     cfg.MediaBaseURL,
			ServiceName: "customer-service",
			Token:       cfg.MediaServiceToken,
		})
		logging.Success("media-client", "configured base_url=%s", cfg.MediaBaseURL)
	} else {
		logging.Notice("media-client", "not configured; profile photo uploads are unavailable")
	}

	customerRepo := profilerepositories.NewPostgresCustomerRepository(db)
	sessionRepo := authrepositories.NewPostgresRefreshSessionRepository(db)
	challengeStore := authrepositories.NewRedisOTPChallengeRepository(redisClient)
	// notificationClient brokers customer-app notification access to
	// notification-service. Empty BaseURL means notification-service is not
	// configured; the OTP sender and proxy routes both fall back gracefully.
	notificationClient := notifications.Client{
		BaseURL:     cfg.NotificationBaseURL,
		ServiceName: "customer-service",
		Secret:      cfg.NotificationSecret,
	}
	var otpSender authclients.OTPSender = authclients.NewLoggingOTPSender(cfg.OTPDebug)
	if cfg.NotificationBaseURL != "" && len(cfg.NotificationSecret) > 0 {
		otpSender = authclients.NewNotificationEmailOTPSender(notificationClient, otpSender)
	}
	profileService := profileusecases.NewProfileService(profileusecases.Options{
		Customers:    customerRepo,
		MediaClient:  mediaClient,
		MediaBaseURL: cfg.MediaBaseURL,
	})
	savedLocationService := savedlocationusecases.NewSavedLocationService(
		savedlocationrepositories.NewPostgresSavedLocationRepository(db),
	)
	preferenceStore := preferences.NewStore(db)
	mobilityRepo := mobilityrepositories.NewPostgresRepository(db)
	callerScopes, err := contextusecases.ParseCallerScopes(cfg.ContextCallerScopes)
	if err != nil {
		logging.Fatal("configuration", "%v", err)
	}
	movementCallers, err := movementusecases.ParseCallerAuthorities(cfg.MovementEventCallers)
	if err != nil {
		logging.Fatal("configuration", "%v", err)
	}
	customerMetrics := observability.NewMetrics()
	movementService := movementusecases.NewService(movementusecases.Options{
		Metrics:         customerMetrics,
		Repository:      movementrepositories.NewPostgresRepository(db),
		Callers:         movementCallers,
		ConsumerEnabled: cfg.MovementEventsConsumerEnabled,
		ReadEnabled:     cfg.UnifiedMovementHistoryEnabled,
		GroupingCallers: strings.Split(cfg.MovementGroupingCallers, ","),
	})
	serviceVerifier := serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute)
	// Source-owned administrative account standing: login blocks are enforced
	// by auth, booking holds are published on the internal customer context.
	accountStanding := admincommands.StandingReader{DB: db}
	customerContextService := contextusecases.NewService(callerScopes, mobilityRepo, savedLocationService).WithGuests(mobilityRepo).WithStanding(accountStanding)
	mobilityService := mobilityusecases.NewService(mobilityRepo, mobilityusecases.Flags{
		MobilityPreferences: cfg.MobilityPreferencesEnabled,
		GuestTravellers:     cfg.GuestTravellersEnabled,
	})
	trustedContactService := profileusecases.NewTrustedContactService(customerRepo, cfg.TrustedContactsEnabled)

	var workers []func(context.Context) error
	if cfg.ReferralFeedEnabled {
		// New customers are announced to referral-service in the same
		// transaction that creates them; this relay delivers the announcements.
		feed, feedErr := referralcontract.NewFeed(db, "customer-service", cfg.ReferralIdentityPepper)
		if feedErr != nil {
			logging.Fatal("referral-feed", "configure (CUSTOMER_REFERRAL_IDENTITY_PEPPER): %v", feedErr)
		}
		relay, relayErr := feed.Relay(cfg.ReferralServiceURL, cfg.ReferralServiceSecret, 10*time.Second, func(err error) {
			logging.Error("referral-feed", "relay: %v", err)
		})
		if relayErr != nil {
			logging.Fatal("referral-feed", "configure relay (CUSTOMER_REFERRAL_SERVICE_URL / _SECRET): %v", relayErr)
		}
		customerRepo.SetSignupAnnouncer(profileclients.NewReferralAnnouncer(feed))
		workers = append(workers, relay.Run)
		logging.Success("referral-feed", "announcing new customers to %s", cfg.ReferralServiceURL)
	} else {
		logging.Notice("referral-feed", "off (CUSTOMER_REFERRAL_FEED_ENABLED=false): new customers are not announced to referral-service")
	}

	// Staging-only: reserved test numbers sign in with a fixed code.
	testLogin, err := sharedauth.ParseTestLogin(cfg.AppEnv, cfg.TestLoginPhones, cfg.TestLoginCode, phonenumber.NormalizeNigerianPhoneNumber)
	if err != nil {
		logging.Fatal("configuration", "CUSTOMER_TEST_LOGIN_PHONES: %v", err)
	}
	if testLogin != nil {
		logging.Notice("test-login", "%d test numbers sign in with the fixed code", testLogin.Count())
	}
	authService := authusecases.NewAuthService(authusecases.Options{
		Customers:          customerRepo,
		Sessions:           sessionRepo,
		Challenges:         challengeStore,
		OTPSender:          otpSender,
		AccessTokenSecret:  cfg.AccessTokenSecret,
		RefreshTokenSecret: cfg.RefreshTokenSecret,
		OTPSecret:          cfg.OTPSecret,
		AccessTokenTTL:     cfg.AccessTokenTTL,
		RefreshTokenTTL:    cfg.RefreshTokenTTL,
		OTPTTL:             cfg.OTPTTL,
		OTPRateWindow:      cfg.OTPRateWindow,
		OTPMaxRequests:     cfg.OTPMaxRequests,
		OTPMaxAttempts:     cfg.OTPMaxAttempts,
		OTPDebug:           cfg.OTPDebug,
		TestLogin:          testLogin,
		Standing:           accountStanding,
	})

	shareRepo := &sharerepositories.Postgres{DB: db}
	shareService := &shareusecases.Service{Repository: shareRepo, Enabled: cfg.ShareJourneyEnabled, TTL: cfg.ShareJourneyTTL, CompletedRetention: cfg.ShareCompletedRetention, Reader: &journeyshare.Client{Endpoints: map[string]journeyshare.Endpoint{
		"TAXI": {BaseURL: cfg.ShareTaxiURL, Secret: cfg.ShareTaxiSecret}, "DISPATCH": {BaseURL: cfg.ShareDispatchURL, Secret: cfg.ShareDispatchSecret}, "HAULING": {BaseURL: cfg.ShareHaulingURL, Secret: cfg.ShareHaulingSecret}, "VENDOR_COMMERCE": {BaseURL: cfg.ShareVendorURL, Secret: cfg.ShareVendorSecret},
	}}}
	ownerAdmin, err := adminresources.Runtime(ctx, db)
	if err != nil {
		logging.Fatal("configuration", "Customer owner admin configuration invalid")
	}
	reviewAdmin, err := adminreview.Runtime(ownerAdmin)
	if err != nil {
		logging.Fatal("configuration", "Customer review capability configuration invalid")
	}
	commandAdmin, err := admincommands.Runtime(ownerAdmin)
	if err != nil {
		logging.Fatal("configuration", "Customer admin command capability configuration invalid")
	}
	serviceapp.Run(serviceapp.Options{
		Workers: append(workers,
			shareworkers.Expiry(shareService, cfg.ShareJourneyEnabled),
			shareworkers.Notifications(shareRepo, notificationClient, cfg.ShareJourneyEnabled),
		),
		Name:        "customer-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/customer",
		Capabilities: []string{
			"customer auth entry using shared auth helpers",
			"customer profiles and preferences",
			"saved locations",
			"customer-facing request history views",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
			func(ctx context.Context) error {
				return redisClient.Ping(ctx).Err()
			},
		},
		Register: func(group *gin.RouterGroup) {
			sharehttp.Register(group, shareService, authService.AccessSigner(), journeyshare.RedisLimiter{Client: redisClient}, cfg.ShareViewerOrigin, cfg.ShareTrustedGatewayCIDRs...)
			customerhttp.RegisterCustomerRoutes(group, authService)
			profilehttp.RegisterProfileRoutes(group, profileService, authService.AccessSigner(), ratelimit.New(redisClient, "customer-media"))
			mediauploadhttp.Register(group, authService.AccessSigner(), mediauploadhttp.Handler{
				Config: cfg, Limiter: ratelimit.New(redisClient, "customer-evidence-media"),
			})
			profilehttp.RegisterTrustedContactRoutes(group, trustedContactService, authService.AccessSigner())
			mobilityhttp.RegisterRoutes(group, mobilityService, authService.AccessSigner())
			contexthttp.RegisterInternalRoutes(group, customerContextService, serviceVerifier)
			movementhttp.RegisterCustomerRoutes(group, movementService, authService.AccessSigner())
			savedlocationhttp.RegisterSavedLocationRoutes(group, savedLocationService, authService.AccessSigner())
			savedlocationhttp.RegisterInternalRoutes(
				group,
				savedLocationService,
				serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
			)
			notificationhttp.RegisterNotificationRoutes(group, notificationClient, authService.AccessSigner())
			identityhttp.RegisterIdentityRoutes(group, profileService, serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute))
			preferences.RegisterRoutes(group, preferenceStore, authService.AccessSigner())
			preferences.RegisterInternalRoutes(group, preferenceStore, serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute))
		},
		RegisterRoot: func(engine *gin.Engine) {
			ownerAdmin.Register(engine)
			reviewAdmin.Register(engine)
			commandAdmin.Register(engine)
			engine.GET("/internal/customer/v1/journey-share-metrics", append(journeyshare.RedisBoundary(serviceVerifier, redisClient, "admin-backoffice-service"), journeyshare.Metrics)...)
			// Customer V4 §27.2 private scrape: movement ingestion, projection
			// age and Share counters. Same caller boundary as the Share metrics.
			engine.GET("/internal/customer/v1/metrics", append(journeyshare.RedisBoundary(serviceVerifier, redisClient, "admin-backoffice-service"), func(c *gin.Context) {
				_ = observability.CollectGauges(c.Request.Context(), observability.PoolQuerier(db), customerMetrics, customerGaugeQueries)
				c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(customerMetrics.Render()+journeyshare.Render()))
			})...)
			movementhttp.RegisterIngress(engine, movementService, serviceVerifier)
		},
	})
}

func logConnectivity(ctx context.Context, databaseURL string, redisAddr string, db interface{ Ping(context.Context) error }, redisClient interface {
	Ping(context.Context) *redis.StatusCmd
}) {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.Ping(pingCtx); err != nil {
		logging.Error("database", "connectivity check failed")
	} else {
		logging.Success("database", "connected")
	}

	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		logging.Error("redis", "failed addr=%s err=%v", redisAddr, err)
	} else {
		logging.Success("redis", "connected addr=%s", redisAddr)
	}
}
