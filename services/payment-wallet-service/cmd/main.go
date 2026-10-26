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
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/admincommands"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/adminresources"
	financehttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/http"
	financeintegration "cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/integration"
	financerepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	financeusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/usecases"
	financereporthttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/financereports/http"
	financereportrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/financereports/repositories"
	financereportusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/financereports/usecases"
	walletclients "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/clients"
	wallethttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/http"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/outboxmetrics"
	"cosmicforge/logistics/shared/go/referralcontract"
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
	accounts := financerepositories.NewAccountRepository(db, ledger)
	operations := financerepositories.NewOperationsRepository(db)
	reversals := financerepositories.NewReversalRepository(db)
	providerOps := financerepositories.NewProviderOperationsRepository(db)
	marketplace := financerepositories.NewMarketplaceRepository(db)
	outboxRepository := events.NewOutboxRepository(db)
	financeService := financeusecases.NewService(financeusecases.Options{
		Pool:        db,
		Ledger:      ledger,
		Accounts:    accounts,
		Billing:     financerepositories.NewBillingRepository(db),
		Marketplace: marketplace,
		Holds:       financerepositories.NewHoldRepository(db),
		Operations:  operations,
		Reversals:   reversals,
		ProviderOps: providerOps,
		JobPayments: financerepositories.NewJobPaymentRepository(db),
		Authorizer:  financeauth.NewAuthorizer(db),
		Audit:       audit.NewRecorder(),
		Events:      events.NewDomainPublisher(outboxRepository),
	})
	walletAdmin, adminErr := adminresources.Runtime(ctx, db)
	if adminErr != nil {
		log.Fatalf("configure Wallet private Admin boundary: %v", adminErr)
	}
	walletCommands, adminErr := admincommand.Runtime(walletAdmin, admincommands.Spec(), admincommands.Executor{
		Accounts: accounts, Ledger: ledger, Operations: operations, Reversals: reversals, ProviderOps: providerOps, Marketplace: marketplace, Audit: audit.NewRecorder(),
		Events: events.NewDomainPublisher(outboxRepository),
	}, "PAYMENT_WALLET")
	if adminErr != nil {
		log.Fatalf("configure Wallet private Admin command boundary: %v", adminErr)
	}
	// Section 26: read-only group finance report for admin-backoffice-service.
	// Off by default (503); enabling it requires a dedicated secret file.
	financeReports, adminErr := financereporthttp.FromEnvironment(db, financereportusecases.Service{Reader: financereportrepositories.New(db)})
	if adminErr != nil {
		log.Fatalf("configure Wallet admin finance report boundary: %v", adminErr)
	}

	// The payment flow mirrors every job payment transition into the
	// authoritative verification record other services read. Constructing the
	// finance service first is what lets that bridge exist at all.
	// Referral cash-outs stay off unless referral-service can be asked first.
	var referralEligibility walletusecases.ReferralEligibilityChecker
	if cfg.ReferralWithdrawalsEnabled {
		if cfg.ReferralOutboxTargetURL == "" || len(cfg.ReferralOutboxSecret) == 0 || len(cfg.ReferralIdentityPepper) == 0 {
			log.Fatalf("PAYMENT_WALLET_REFERRAL_WITHDRAWALS_ENABLED needs PAYMENT_WALLET_REFERRAL_OUTBOX_TARGET_URL, PAYMENT_WALLET_REFERRAL_OUTBOX_SECRET and PAYMENT_WALLET_REFERRAL_IDENTITY_PEPPER")
		}
		referralEligibility = referralcontract.Client{
			BaseURL:     cfg.ReferralOutboxTargetURL,
			ServiceName: "payment-wallet-service",
			Secret:      cfg.ReferralOutboxSecret,
			HTTPClient:  &http.Client{Timeout: cfg.HTTPRequestTimeout},
		}
	}
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

		ReferralEligibility:       referralEligibility,
		ReferralPepper:            cfg.ReferralIdentityPepper,
		ReferralWithdrawalMinKobo: cfg.ReferralWithdrawalMinKobo,
	})
	var workers []func(context.Context) error
	if cfg.NotificationURL != "" && len(cfg.NotificationSecret) > 0 {
		notificationOutbox := walletclients.WalletNotificationOutbox{DB: db, Sender: notifications.Client{
			BaseURL: cfg.NotificationURL, ServiceName: "payment-wallet-service", Secret: cfg.NotificationSecret,
		}}
		workers = append(workers, notificationOutbox.Run)
	}
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
	// Section 12 WAL-017. A corporate job whose settlement Fleet never
	// instructs keeps the customer's money in escrow with no error anywhere --
	// correct, and invisible. This sweep is what makes it visible. It moves no
	// money and holds no authority to; entitlement stays Fleet's.
	if cfg.ManagedReconciliationEnabled && !cfg.FinanceExecutionKillSwitch {
		reconciliationWorker := &walletusecases.ManagedReconciliationWorker{
			Store: repository,
			Config: walletusecases.ManagedReconciliationConfig{
				Grace:     cfg.ManagedReconciliationGrace,
				Interval:  cfg.ManagedReconciliationInterval,
				BatchSize: cfg.ManagedReconciliationBatch,
			},
		}
		workers = append(workers, func(workerCtx context.Context) error {
			reconciliationWorker.Run(workerCtx)
			return nil
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

	if cfg.ReferralOutboxEnabled {
		// Every job settlement appends a finance.job.settled.v1 event in its own
		// transaction; this relay delivers them to referral-service.
		referralStore, storeErr := outbox.NewStore(db, referralcontract.OutboxTable)
		if storeErr != nil {
			log.Fatalf("configure referral outbox: %v", storeErr)
		}
		referralPublisher, publisherErr := outbox.NewHTTPPublisher(cfg.ReferralOutboxTargetURL, referralcontract.IngestPath, "payment-wallet-service", cfg.ReferralOutboxSecret, cfg.HTTPRequestTimeout)
		if publisherErr != nil {
			log.Fatalf("configure referral outbox publisher (PAYMENT_WALLET_REFERRAL_OUTBOX_TARGET_URL / _SECRET): %v", publisherErr)
		}
		repository.SetReferralOutbox(referralStore)
		referralRelay := outbox.NewWorker(referralStore, referralPublisher, outbox.WorkerConfig{
			Owner: "payment-wallet-service-referral-outbox", BatchSize: cfg.ReferralOutboxBatchSize,
			Interval: cfg.ReferralOutboxInterval, Lease: cfg.ReferralOutboxLease, MaxAttempts: cfg.ReferralOutboxMaxAttempts,
			OnError: func(err error) { log.Printf("referral outbox relay: %v", err) },
		})
		workers = append(workers, referralRelay.Run)
	}

	// Outbox and reconciliation health, in the same metric names every other
	// service reports (OBS-006, OBS-007).
	//
	// The source descriptions live with the tables' writer, where a test can
	// run them against the real migrated schema.
	walletMetrics := outboxmetrics.NewReporter(db, events.MetricsInterval,
		events.MetricsSources()...).WithAuthToken(cfg.MetricsAuthToken)
	workers = append(workers, walletMetrics.Run)

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
			walletMetrics.RegisterRoutes(group)
			wallethttp.RegisterRoutes(group, walletService, cfg.CustomerAccessTokenSecret, cfg.ProviderAccessTokenSecrets, cfg.ServiceSecrets)
			if cfg.FinancePrivateAPIEnabled {
				financehttp.RegisterPrivateRoutes(group.Group("/internal/finance"), financeService,
					serviceauth.NewVerifier(cfg.ServiceSecrets, 5*time.Minute), cfg.FinanceAllowedCallers)
			}
		},
		RegisterRoot: func(router *gin.Engine) {
			walletAdmin.RegisterOn(router.Group(""))
			walletCommands.Register(router)
			financeReports.Register(router)
		},
		Workers: workers,
	})
}
