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
	"cosmicforge/logistics/services/payment-wallet-service/internal/events"
	financehttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/http"
	financeintegration "cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/integration"
	financerepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	financeusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/usecases"
	walletclients "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/clients"
	wallethttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/http"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
	"time"
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
	// walletNotifier emits financial notifications via notification-service.
	// No-op when PAYMENT_WALLET_NOTIFICATION_URL/SECRET are unset (local dev).
	walletNotifier := walletclients.NewWalletNotifier(cfg.NotificationURL, cfg.NotificationSecret)
	ledger := financerepositories.NewLedgerRepository(db)
	outboxRepository := events.NewOutboxRepository(db)
	financeService := financeusecases.NewService(financeusecases.Options{
		Pool:        db,
		Ledger:      ledger,
		Accounts:    financerepositories.NewAccountRepository(db, ledger),
		Billing:     financerepositories.NewBillingRepository(db),
		Marketplace: financerepositories.NewMarketplaceRepository(db),
		Holds:       financerepositories.NewHoldRepository(db),
		Operations:  financerepositories.NewOperationsRepository(db),
		Reversals:   financerepositories.NewReversalRepository(db),
		JobPayments: financerepositories.NewJobPaymentRepository(db),
		Authorizer:  financeauth.NewAuthorizer(db),
		Audit:       audit.NewRecorder(),
		Events:      events.NewDomainPublisher(outboxRepository),
	})

	// The payment flow mirrors every job payment transition into the
	// authoritative verification record other services read. Constructing the
	// finance service first is what lets that bridge exist at all.
	walletService := walletusecases.NewWalletService(walletusecases.Options{
		Repository:        repository,
		Paystack:          paystack,
		Notifier:          walletNotifier,
		DefaultCurrency:   cfg.DefaultCurrency,
		PlatformFeeBPS:    cfg.PlatformFeeBPS,
		CallbackBaseURL:   cfg.PublicCallbackBaseURL,
		WithdrawalMinKobo: cfg.WithdrawalMinKobo,
		WithdrawalMaxKobo: cfg.WithdrawalMaxKobo,
		JobPayments:       financeintegration.NewJobPaymentBridge(financeService),
	})
	var workers []func(context.Context) error
	if cfg.FinanceWorkersEnabled && !cfg.FinanceExecutionKillSwitch {
		gateway := financeintegration.PaystackGateway{Client: paystack}
		workerConfig := financeusecases.WorkerConfig{
			Owner: "payment-wallet-service", BatchSize: cfg.FinanceWorkerBatchSize,
			Interval: cfg.FinanceWorkerInterval, Lease: cfg.FinanceWorkerLease,
			MaxAttempts: cfg.FinanceWorkerMaxAttempts, BaseBackoff: cfg.FinanceWorkerBackoff,
		}
		workers = append(workers, func(workerCtx context.Context) error {
			return financeService.RunFinancialWorkers(workerCtx, gateway, workerConfig)
		}, func(workerCtx context.Context) error {
			return financeService.RunMaintenanceWorkers(workerCtx, workerConfig)
		})
	}
	if cfg.FinanceOutboxEnabled {
		publisher, publisherErr := events.NewHTTPPublisher(cfg.FinanceOutboxTargetURL, cfg.FinanceOutboxPath, "payment-wallet-service", cfg.FinanceOutboxSecret, cfg.HTTPRequestTimeout)
		if publisherErr != nil {
			log.Fatalf("configure finance outbox publisher: %v", publisherErr)
		}
		outboxWorker := events.NewWorker(outboxRepository, publisher, events.WorkerConfig{Owner: "payment-wallet-service-outbox", BatchSize: cfg.FinanceOutboxBatchSize, Interval: cfg.FinanceOutboxInterval, Lease: cfg.FinanceOutboxLease, MaxAttempts: cfg.FinanceOutboxMaxAttempts})
		workers = append(workers, outboxWorker.Run)
	}

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
			if cfg.FinancePrivateAPIEnabled {
				financehttp.RegisterPrivateRoutes(group.Group("/internal/finance"), financeService,
					serviceauth.NewVerifier(cfg.ServiceSecrets, 5*time.Minute), cfg.FinanceAllowedCallers)
			}
		},
		Workers: workers,
	})
}
