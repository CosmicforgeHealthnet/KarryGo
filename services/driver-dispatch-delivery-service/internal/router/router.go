package router

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authrepositories "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	bookingfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking"
	identityhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/identity/http"
	notificationhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/notifications/http"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/profile"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/vehicle"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/verification"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/middleware"
	sharedauth "cosmicforge/logistics/shared/go/auth"
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

func New(cfg config.Config, db *pgxpool.Pool, redisClient *redis.Client, authService *authusecases.AuthUsecase, requestService *requestfeature.Service, tripService *tripfeature.Service, bookingService *bookingfeature.Service) *gin.Engine {
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
	avatarUploader, err := profile.NewAvatarUploaderFromEnv(cfg.AppEnv)
	if err != nil {
		panic(fmt.Errorf("configure avatar storage: %w", err))
	}
	profile.RegisterRoutes(engine, authService.TokenUsecase(), profile.NewHandlerWithUploader(profileService, avatarUploader))
	profile.StartSubscribers(context.Background(), redisClient, profileRepository)

	verificationRepository := verification.NewPostgresRepository(db)
	verificationUploader, err := verification.NewUploaderFromEnv(cfg.AppEnv)
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

	vehicleUploader, err := vehicle.NewVehicleUploaderFromEnv(cfg.AppEnv)
	if err != nil {
		panic(fmt.Errorf("configure vehicle storage: %w", err))
	}
	vehicleHandler := vehicle.NewHandler(db, vehicleUploader, vehicle.NewRedisEventPublisher(redisClient))
	vehicle.RegisterRoutes(engine, authService.TokenUsecase(), vehicleHandler)
	vehicle.StartSubscribers(context.Background(), redisClient, vehicle.NewPostgresRepository(db))

	availabilityRepository := availability.NewPostgresRepository(db)
	availabilityLiveStore := availability.NewRedisLiveStore(redisClient)
	availabilityHandler := availability.NewHandlerWithService(
		availability.NewService(
			availabilityRepository,
			availabilityLiveStore,
			availability.WithEventPublisher(availability.NewRedisEventPublisher(redisClient)),
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
		bookingfeature.RegisterRoutes(
			engine,
			sharedauth.NewTokenSigner(cfg.CustomerTokenSecret),
			bookingfeature.NewHandler(bookingService, profileService),
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

	return engine
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
