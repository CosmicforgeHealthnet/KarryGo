package main

import (
	"context"
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

	identityhttp "cosmicforge/logistics/services/hauling-service/internal/features/identity/http"
	notificationhttp "cosmicforge/logistics/services/hauling-service/internal/features/notifications/http"

	providerauthhttp "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/http"
	providerauthrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/repositories"
	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"

	providerprofilehttp "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/http"
	providerprofilerepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/repositories"
	providerprofileusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/usecases"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	logging.Notice("hauling-service", "migration=%t database=%s redis=%s", cfg.Migration, cfg.DatabaseURL, cfg.Redis.Addr)

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
	availabilityStore := availabilityrepositories.NewRedisAvailabilityStore(redisClient)
	bookingRepo := bookingrepo.NewPostgresBookingRepository(db)

	// ─── Auth service ──────────────────────────────────────────────────────────
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
	})

	// customerSigner verifies customer bearer tokens (read-only; same secret as customer-service).
	customerSigner := sharedauth.NewTokenSigner(cfg.CustomerTokenSecret)

	// ─── Profile service ───────────────────────────────────────────────────────
	profileService := providerprofileusecases.NewProfileService(profileRepo, truckRepo)

	// ─── Availability service ──────────────────────────────────────────────────
	onlineTTL := time.Duration(cfg.ProviderOnlineTTL) * time.Second
	availService := availabilityusecases.NewAvailabilityService(availabilityStore, truckRepo, onlineTTL)

	// ─── Booking service ───────────────────────────────────────────────────────
	// bookingNotifier sends booking-lifecycle notifications via notification-service.
	// When HAULING_NOTIFICATION_URL/SECRET are unset it is a no-op (local dev).
	bookingNotifier := bookingclients.NewBookingNotifier(cfg.NotificationURL, cfg.NotificationSecret)

	// paymentClient binds booking fares to payment-wallet-service. When
	// HAULING_PAYMENT_URL/SECRET are unset (local dev) payment is disabled and the
	// booking flow settles without charging.
	var paymentClient bookingusecases.PaymentClient
	if cfg.PaymentURL != "" && len(cfg.PaymentSecret) > 0 {
		paymentClient = bookingpayments.NewWalletPaymentClient(cfg.PaymentURL, "driver-hauling-service", cfg.PaymentSecret)
	}

	bookingService := bookingusecases.NewBookingService(bookingusecases.Options{
		Bookings:            bookingRepo,
		Availability:        availabilityStore,
		Trucks:              truckLookup{trucks: truckRepo},
		Payments:            paymentClient,
		Notifier:            bookingNotifier,
		MatchTimeoutSeconds: cfg.BookingMatchTimeout,
		SearchWindowSeconds: cfg.BookingSearchWindow,
		MaxRadiusKm:         cfg.MatchMaxRadiusKm,
	})

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
	go bookingService.RunAutoCompleteWorker(ctx)

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
			providerauthhttp.RegisterProviderAuthRoutes(group, authService)
			providerprofilehttp.RegisterProfileRoutes(group, profileService, authService)
			availabilityhttp.RegisterAvailabilityRoutes(group, availService, authService, customerSigner)
			bookinghttp.RegisterBookingRoutes(group, bookingService, authService, customerSigner, providerRepo, truckRepo)
			notificationhttp.RegisterNotificationRoutes(group, notificationClient, authService.AccessSigner(), customerSigner)
			identityhttp.RegisterIdentityRoutes(group, authService, serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute))
		},
	})
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

func logConnectivity(ctx context.Context, databaseURL, redisAddr string, db interface{ Ping(context.Context) error }, redisClient interface {
	Ping(context.Context) *redis.StatusCmd
}) {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.Ping(pingCtx); err != nil {
		logging.Error("database", "failed url=%s err=%v", databaseURL, err)
	} else {
		logging.Success("database", "connected url=%s", databaseURL)
	}

	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		logging.Error("redis", "failed addr=%s err=%v", redisAddr, err)
	} else {
		logging.Success("redis", "connected addr=%s", redisAddr)
	}
}
