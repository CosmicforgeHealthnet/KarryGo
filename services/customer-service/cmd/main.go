package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/customer-service/internal/config"
	"cosmicforge/logistics/services/customer-service/internal/database"
	authclients "cosmicforge/logistics/services/customer-service/internal/features/auth/clients"
	customerhttp "cosmicforge/logistics/services/customer-service/internal/features/auth/http"
	authrepositories "cosmicforge/logistics/services/customer-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	profilerepositories "cosmicforge/logistics/services/customer-service/internal/features/profile/repositories"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("create customer database pool: %v", err)
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	customerRepo := profilerepositories.NewPostgresCustomerRepository(db)
	sessionRepo := authrepositories.NewPostgresRefreshSessionRepository(db)
	challengeStore := authrepositories.NewRedisOTPChallengeRepository(redisClient)
	otpSender := authclients.NewLoggingOTPSender(cfg.OTPDebug)
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
		},
	})
}
