package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/customer-service/internal/config"
	"cosmicforge/logistics/services/customer-service/internal/database"
	authclients "cosmicforge/logistics/services/customer-service/internal/features/auth/clients"
	customerhttp "cosmicforge/logistics/services/customer-service/internal/features/auth/http"
	authrepositories "cosmicforge/logistics/services/customer-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	notificationhttp "cosmicforge/logistics/services/customer-service/internal/features/notifications/http"
	profilehttp "cosmicforge/logistics/services/customer-service/internal/features/profile/http"
	profilerepositories "cosmicforge/logistics/services/customer-service/internal/features/profile/repositories"
	profileusecases "cosmicforge/logistics/services/customer-service/internal/features/profile/usecases"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	logging.Notice("customer-service config", "migration=%t database=%s redis=%s", cfg.Migration, cfg.DatabaseURL, cfg.Redis.Addr)

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
		logging.Notice("media-client", "not configured — profile photo uploads will store empty URL")
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
		Customers:   customerRepo,
		MediaClient: mediaClient,
	})

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
	})

	serviceapp.Run(serviceapp.Options{
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
			customerhttp.RegisterCustomerRoutes(group, authService)
			profilehttp.RegisterProfileRoutes(group, profileService, authService.AccessSigner())
			notificationhttp.RegisterNotificationRoutes(group, notificationClient, authService.AccessSigner())
		},
	})
}

func logConnectivity(ctx context.Context, databaseURL string, redisAddr string, db interface{ Ping(context.Context) error }, redisClient interface {
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
