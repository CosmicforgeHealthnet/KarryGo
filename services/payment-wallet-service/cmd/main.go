package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/payment-wallet-service/internal/config"
	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	walletclients "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/clients"
	wallethttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/http"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("create payment wallet database pool: %v", err)
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	repository := walletrepositories.NewPostgresWalletRepository(db)
	paystack := walletclients.HTTPPaystackClient{
		BaseURL:   cfg.Paystack.BaseURL,
		SecretKey: cfg.Paystack.SecretKey,
		HTTPClient: &http.Client{
			Timeout: cfg.HTTPRequestTimeout,
		},
	}
	walletService := walletusecases.NewWalletService(walletusecases.Options{
		Repository:        repository,
		Paystack:          paystack,
		DefaultCurrency:   cfg.DefaultCurrency,
		PlatformFeeBPS:    cfg.PlatformFeeBPS,
		CallbackBaseURL:   cfg.PublicCallbackBaseURL,
		WithdrawalMinKobo: cfg.WithdrawalMinKobo,
		WithdrawalMaxKobo: cfg.WithdrawalMaxKobo,
	})

	serviceapp.Run(serviceapp.Options{
		Name:        "payment-wallet-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/payment-wallet",
		Capabilities: []string{
			"customer wallets",
			"payments and refunds",
			"provider earnings",
			"withdrawals",
			"fleet settlement",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
			func(ctx context.Context) error {
				return redisClient.Ping(ctx).Err()
			},
		},
		Register: func(group *gin.RouterGroup) {
			wallethttp.RegisterRoutes(group, walletService, cfg.CustomerAccessTokenSecret, cfg.ProviderAccessTokenSecrets, cfg.ServiceSecrets)
		},
	})
}
