package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/hauling-service/internal/config"
	"cosmicforge/logistics/services/hauling-service/internal/database"

	availabilityhttp "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/http"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	availabilityusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/usecases"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookinghttp "cosmicforge/logistics/services/hauling-service/internal/features/booking/http"
	bookingpayments "cosmicforge/logistics/services/hauling-service/internal/features/booking/payments"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"

	haulingcommercemission "cosmicforge/logistics/services/hauling-service/internal/features/commercemission"
	haulingcommercequote "cosmicforge/logistics/services/hauling-service/internal/features/commercequote"
	fleetauthority "cosmicforge/logistics/services/hauling-service/internal/features/fleetauthority"
	identityhttp "cosmicforge/logistics/services/hauling-service/internal/features/identity/http"
	notificationhttp "cosmicforge/logistics/services/hauling-service/internal/features/notifications/http"
	privateexecution "cosmicforge/logistics/services/hauling-service/internal/features/privateexecution"
	providerassignments "cosmicforge/logistics/services/hauling-service/internal/features/providerassignments"
	restrictions "cosmicforge/logistics/services/hauling-service/internal/features/restrictions"

	providerauthhttp "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/http"
	providerauthrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/repositories"
	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"

	providerprofilehttp "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/http"
	providerprofilerepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/repositories"
	providerprofileusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/usecases"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/providercompat"
	"cosmicforge/logistics/shared/go/routing"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logging.Fatal("configuration", "%v", err)
	}
	logging.Notice("hauling-service", "migration=%t redis=%s", cfg.Migration, cfg.Redis.Addr)

	// ctx is cancelled when main returns (after serviceapp.Run exits on SIGTERM).
	// Background goroutines (auto-complete worker) use this to stop cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create hauling database pool: %v", err)
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

	logConnectivity(ctx, cfg.DatabaseURL, cfg.Redis.Addr, db, redisClient)

	if cfg.Migration {
		logging.Notice("migration", "mode enabled")
		migrationCtx, mcancel := context.WithTimeout(ctx, 30*time.Second)
		if err := database.ApplyMigrations(migrationCtx, db); err != nil {
			mcancel()
			logging.Fatal("migration", "apply hauling-service migrations: %v", err)
		}
		mcancel()
		logging.Success("migration", "applied successfully")
	}

	// ─── Repositories ─────────────────────────────────────────────────────────
	providerRepo := providerauthrepositories.NewPostgresProviderRepository(db)
	sessionRepo := providerauthrepositories.NewPostgresRefreshSessionRepository(db)
	challengeStore := providerauthrepositories.NewRedisOTPChallengeRepository(redisClient)
	profileRepo := providerprofilerepositories.NewPostgresProfileRepository(db)
	truckRepo := providerprofilerepositories.NewPostgresTruckRepository(db)
	bookingRepo := bookingrepo.NewPostgresBookingRepository(db)

	// ─── Auth service ──────────────────────────────────────────────────────────
	var membershipReader providercompat.Reader
	if cfg.ProviderOrganizationModeEnabled {
		membershipReader = providercompat.NewClient(cfg.FleetServiceURL, cfg.FleetServiceCaller,
			cfg.FleetServiceSecret, cfg.FleetServiceTimeout)
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
		MembershipReader:   membershipReader,
		ProviderService:    cfg.FleetServiceCaller,
	})

	// customerSigner verifies customer bearer tokens (read-only; same secret as customer-service).
	customerSigner := sharedauth.NewTokenSigner(cfg.CustomerTokenSecret)

	// ─── Profile service ───────────────────────────────────────────────────────
	profileService := providerprofileusecases.NewProfileService(profileRepo, truckRepo)

	// ─── Availability service ──────────────────────────────────────────────────
	// routesClient is nil unless HAULING_ROUTES_API_KEY is set. Route geometry
	// is optional everywhere it is used.
	routesClient := routing.New(cfg.RoutesAPIKey)
	if routesClient == nil {
		logging.Notice("routing", "disabled (HAULING_ROUTES_API_KEY unset) - trip maps and progress fall back to straight lines")
	}

	onlineTTL := time.Duration(cfg.ProviderOnlineTTL) * time.Second
	// The live-tracking index expires fixes on the same clock the availability
	// keys use, so a provider can never be matchable from a position that has
	// already aged out of their status key.
	availabilityStore := availabilityrepositories.NewRedisAvailabilityStoreWithTTL(redisClient, onlineTTL)
	restrictionService := restrictions.New(db)
	availService := availabilityusecases.NewAvailabilityServiceWithAuthority(availabilityStore, truckRepo, onlineTTL, availabilityrepositories.NewPostgresAuthorityStore(db)).WithRestrictionGate(restrictionService)

	// ─── Booking service ───────────────────────────────────────────────────────
	// bookingNotifier sends booking-lifecycle notifications via notification-service.
	// Incomplete configuration produces a delivery error; it never reports a
	// fabricated notification success.
	bookingNotifier := bookingclients.NewDurableBookingNotifier(notificationRedis)

	// paymentClient binds booking fares to payment-wallet-service. When
	// HAULING_PAYMENT_URL/SECRET are unset payment-required paths fail closed.
	var paymentClient bookingusecases.PaymentClient
	var commerceSettlement bookingusecases.CommerceDeliverySettlementAuthority
	if cfg.PaymentURL != "" && len(cfg.PaymentSecret) > 0 {
		walletPayments := bookingpayments.NewWalletPaymentClient(cfg.PaymentURL, "driver-hauling-service", cfg.PaymentSecret)
		paymentClient = walletPayments
		commerceSettlement = walletPayments
	}

	bookingService := bookingusecases.NewBookingService(bookingusecases.Options{
		Bookings:            bookingRepo,
		Availability:        availabilityStore,
		Trucks:              truckLookup{trucks: truckRepo},
		Payments:            paymentClient,
		CommerceSettlement:  commerceSettlement,
		Notifier:            bookingNotifier,
		MatchTimeoutSeconds: cfg.BookingMatchTimeout,
		SearchWindowSeconds: cfg.BookingSearchWindow,
		MaxRadiusKm:         cfg.MatchMaxRadiusKm,
		HandoverSecret:      cfg.HandoverSecret,
		AllowCashPayments:   cfg.CashPaymentsEnabled,
		Restrictions:        restrictionService,
		// nil when HAULING_ROUTES_API_KEY is unset: routes are then never
		// fetched and the apps fall back to straight-line geometry.
		Routes: routesClient,
	})
	authorityHandler := fleetauthority.New(db, fleetauthority.Flags{
		Enabled: cfg.FleetIntegrationEnabled, KillSwitch: cfg.FleetIntegrationKillSwitch,
	})
	inboundVerifier := serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute)
	privateExecutionService := privateexecution.NewService(
		privateexecution.NewRepository(db),
		privateexecution.NewHTTPAcceptanceVerifier(cfg.FleetServiceURL, cfg.FleetServiceCaller,
			cfg.FleetServiceSecret, cfg.FleetServiceTimeout),
		cfg.PrivateExecutionEnabled, cfg.FleetIntegrationKillSwitch).WithRestrictionGate(restrictionService)
	providerAssignmentClient := providerassignments.NewClient(cfg.FleetServiceURL,
		cfg.FleetServiceCaller, cfg.FleetServiceSecret, cfg.FleetServiceTimeout)
	if cfg.OutboxWorkerEnabled && cfg.FleetIntegrationEnabled && !cfg.FleetIntegrationKillSwitch {
		go (&fleetauthority.OutboxWorker{DB: db, URL: cfg.FleetEventURL, Secret: cfg.FleetEventSecret}).Run(ctx)
	}

	// notificationClient brokers provider-app notification access to
	// notification-service (feed, realtime token, device registration). Empty
	// BaseURL skips the proxy routes (local dev without notification-service).
	notificationClient := notifications.Client{
		BaseURL:     cfg.NotificationURL,
		ServiceName: "driver-hauling-service",
		Secret:      cfg.NotificationSecret,
	}

	// Auto-complete worker: promotes delivered bookings to completed after the
	// grace period. Runs until ctx is cancelled (i.e. service shutdown).
	if !cfg.FleetIntegrationKillSwitch {
		go bookingService.RunAutoCompleteWorker(ctx)
	}

	// Match recovery worker: re-adopts bookings whose in-memory matcher was lost
	// (e.g. across a restart) so they resume matching or go unmatched instead of
	// stranding in pending_match forever.
	if !cfg.FleetIntegrationKillSwitch {
		go bookingService.RunMatchRecoveryWorker(ctx)
	}

	// ─── HTTP ──────────────────────────────────────────────────────────────────
	serviceapp.Run(serviceapp.Options{
		Name:        "hauling-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/hauling",
		Capabilities: []string{
			"truck provider auth (OTP phone)",
			"truck provider profiles and truck records",
			"provider availability management (online/offline via Redis)",
			"customer availability check",
			"haulage bookings with async provider matching",
			"booking lifecycle: accept -> pickup -> deliver -> complete",
			"fare estimation",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
			func(ctx context.Context) error {
				return redisClient.Ping(ctx).Err()
			},
		},
		Register: func(group *gin.RouterGroup) {
			group.Use(func(c *gin.Context) {
				if cfg.FleetIntegrationKillSwitch && c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
					c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{"code": "integration_kill_switch", "message": "Hauling writes are temporarily disabled."}})
					return
				}
				c.Next()
			})
			providerauthhttp.RegisterProviderAuthRoutes(group, authService)
			providerprofilehttp.RegisterProfileRoutes(group, profileService, authService)
			availabilityhttp.RegisterAvailabilityRoutes(group, availService, authService, customerSigner)
			bookinghttp.RegisterBookingRoutes(group, bookingService, authService, customerSigner, providerRepo, truckRepo, redisClient)
			notificationhttp.RegisterNotificationRoutes(group, notificationClient, authService.AccessSigner(), customerSigner)
			identityhttp.RegisterIdentityRoutes(group, authService, serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute))

			// The internal commerce delivery-quote contract (Vendor Service
			// Section 17C). It answers serviceability and price for a route;
			// it creates no booking and matches no provider.
			//
			// Registration is skipped entirely when this deployment holds no
			// secret for the vendor caller, so a deployment that does not
			// serve commerce quotes exposes no route to probe rather than one
			// that answers 401 and confirms it exists.
			if _, trusted := serviceauth.ParseSecrets(cfg.ServiceSecrets)[commercequote.SignatureCaller]; trusted {
				haulingcommercequote.Register(
					group,
					haulingcommercequote.NewHandler(haulingcommercequote.New(
						haulingcommercequote.Options{
							Store:        haulingcommercequote.NewRepository(db),
							Availability: availabilityStore,
							Trucks:       commerceTruckLookup{trucks: truckRepo},
							MaxRadiusKm:  cfg.MatchMaxRadiusKm,
							TTL:          cfg.CommerceQuoteTTL,
						},
					)),
					db,
					serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
					[]string{commercequote.SignatureCaller},
				)
			}

			// The internal commerce mission contract (Vendor Service Section
			// 17F). Unlike the quote surface above this one EXECUTES: it
			// creates a real booking and starts provider matching for a fee
			// the customer has already paid. It is skipped without a booking
			// service rather than accepting hauls nothing would ever match.
			if _, trusted := serviceauth.ParseSecrets(cfg.ServiceSecrets)[commercemission.SignatureCaller]; trusted &&
				bookingService != nil {
				haulingcommercemission.Register(
					group,
					haulingcommercemission.NewHandler(haulingcommercemission.New(
						haulingcommercemission.Options{
							Store:   haulingcommercemission.NewRepository(db),
							Matcher: bookingService,
						},
					)),
					db,
					serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
					[]string{commercemission.SignatureCaller},
				)
			}
			fleetauthority.Register(group, authorityHandler, inboundVerifier, splitCSV(cfg.FleetAllowedCallers))
			restrictions.Register(group, restrictionService, authorityHandler, inboundVerifier, splitCSV(cfg.FleetAllowedCallers))
			privateexecution.Register(group, privateexecution.NewHandler(privateExecutionService),
				authorityHandler, inboundVerifier, splitCSV(cfg.FleetAllowedCallers))
			providerassignments.Register(group, authService, providerAssignmentClient)
		},
	})
}

func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// truckLookup adapts the provider_profile truck repository to the booking
// usecase TruckLookup interface, so the matcher can filter providers by truck
// type and capacity without the usecase importing the provider_profile package.
type truckLookup struct {
	trucks providerprofilerepositories.TruckRepository
}

func (t truckLookup) GetTruck(ctx context.Context, truckID string) (bookingusecases.TruckInfo, error) {
	truck, err := t.trucks.GetByIDAnywhere(ctx, truckID)
	if err != nil {
		return bookingusecases.TruckInfo{}, err
	}
	return bookingusecases.TruckInfo{
		TruckType:  truck.TruckType,
		CapacityKg: truck.CapacityKg,
		Status:     truck.Status,
	}, nil
}

// commerceTruckLookup adapts the same truck repository to the commerce quote
// service's own narrow lookup, so a quote applies exactly the truck-capacity
// and truck-type filters the matcher will later apply. A quote issued on
// looser rules than the matcher enforces is a quote that cannot be filled.
type commerceTruckLookup struct {
	trucks providerprofilerepositories.TruckRepository
}

func (t commerceTruckLookup) GetTruck(ctx context.Context, truckID string) (haulingcommercequote.TruckInfo, error) {
	truck, err := t.trucks.GetByIDAnywhere(ctx, truckID)
	if err != nil {
		return haulingcommercequote.TruckInfo{}, err
	}
	return haulingcommercequote.TruckInfo{
		TruckType:  truck.TruckType,
		CapacityKg: truck.CapacityKg,
		Status:     truck.Status,
	}, nil
}

func logConnectivity(ctx context.Context, databaseURL, redisAddr string, db interface{ Ping(context.Context) error }, redisClient interface {
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
