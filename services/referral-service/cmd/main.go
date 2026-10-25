package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/referral-service/internal/config"
	"cosmicforge/logistics/services/referral-service/internal/database"
	"cosmicforge/logistics/services/referral-service/internal/events"
	"cosmicforge/logistics/services/referral-service/internal/features/admincommands"
	"cosmicforge/logistics/services/referral-service/internal/features/adminresources"
	referralhttp "cosmicforge/logistics/services/referral-service/internal/features/referrals/http"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	"cosmicforge/logistics/services/referral-service/internal/jobs"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/ratelimit"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/walletclient"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logging.Fatal("configuration", "%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create pool: %v", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		logging.Error("database", "ping failed (service will report not-ready): %v", err)
	} else {
		logging.Success("database", "connected")
	}

	if cfg.Migration {
		migrationCtx, migrationCancel := context.WithTimeout(ctx, 60*time.Second)
		if err := database.ApplyMigrations(migrationCtx, db, "migrations"); err != nil {
			migrationCancel()
			logging.Fatal("migration", "apply referral-service migrations: %v", err)
		}
		migrationCancel()
		logging.Success("migration", "applied successfully")
	}

	if len(cfg.ServiceSecrets) == 0 {
		logging.Notice("events", "REFERRAL_SERVICE_SECRETS is empty: no service can deliver events yet")
	}

	// Redis only throttles the public code check. Without it that endpoint is
	// unthrottled, which is acceptable for local development only.
	var limiter referralhttp.RateLimiter
	readyChecks := []func(context.Context) error{db.Ping}
	if cfg.Redis.Addr != "" {
		redisClient := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
		defer redisClient.Close()
		limiter = ratelimit.New(redisClient, "referral")
		readyChecks = append(readyChecks, func(ctx context.Context) error { return redisClient.Ping(ctx).Err() })
	} else {
		logging.Notice("redis", "REFERRAL_REDIS_ADDR is empty: the public code check is not rate limited")
	}

	// Approved rewards are posted to payment-wallet. Without a wallet they wait
	// durably (wallet_state=pending) and are posted once one is configured.
	var wallet referralusecases.WalletClient
	if cfg.WalletURL != "" {
		wallet = walletclient.Client{
			BaseURL:     cfg.WalletURL,
			ServiceName: "referral-service",
			Secret:      cfg.WalletSecret,
			HTTPClient:  &http.Client{Timeout: 15 * time.Second},
		}
		logging.Success("wallet", "posting rewards to %s", cfg.WalletURL)
	} else {
		logging.Notice("wallet", "REFERRAL_WALLET_URL is empty: approved rewards will queue until a wallet is configured")
	}

	store := referralrepositories.NewStore(db)
	service := referralusecases.NewService(referralusecases.Options{
		Store:         store,
		Rules:         rulesFrom(cfg.Programme),
		ShareLinkBase: cfg.ShareLinkBase,
		Wallet:        wallet,
	})

	onWorkerError := func(name string, err error) { logging.Error("worker", "%s: %v", name, err) }
	batch := cfg.Workers.BatchSize
	workers := []func(context.Context) error{
		func(runCtx context.Context) error {
			jobs.Job{Name: "nonce-prune", Interval: 10 * time.Minute, Pass: func(passCtx context.Context) error {
				return events.PruneNonces(passCtx, db)
			}}.Run(runCtx, onWorkerError)
			return nil
		},
		func(runCtx context.Context) error {
			// Decides "was the referrer active?" once each reward's wait is up.
			jobs.Job{Name: "reward-decisions", Interval: cfg.Workers.DecisionInterval, Pass: func(passCtx context.Context) error {
				_, err := service.DecidePendingRewards(passCtx, batch)
				return err
			}}.Run(runCtx, onWorkerError)
			return nil
		},
		func(runCtx context.Context) error {
			// Marks driver tracks whose 90-day earning window has ended.
			jobs.Job{Name: "window-expiry", Interval: cfg.Workers.ExpiryInterval, Pass: func(passCtx context.Context) error {
				_, err := service.ExpireDriverWindows(passCtx)
				return err
			}}.Run(runCtx, onWorkerError)
			return nil
		},
		func(runCtx context.Context) error {
			// Checks every account's postings against the wallet's ledger.
			jobs.Job{Name: "wallet-reconciliation", Interval: cfg.Workers.ReconciliationInterval, Pass: func(passCtx context.Context) error {
				result, err := service.ReconcileWithWallet(passCtx, 200)
				if err == nil && result.Mismatched > 0 {
					logging.Error("reconciliation", "%d of %d accounts disagree with the wallet", result.Mismatched, result.Checked)
				}
				return err
			}}.Run(runCtx, onWorkerError)
			return nil
		},
		func(runCtx context.Context) error {
			// Posts approved rewards and reversals to payment-wallet.
			jobs.Job{Name: "wallet-posting", Interval: cfg.Workers.WalletInterval, Pass: func(passCtx context.Context) error {
				_, err := service.PostRewardsToWallet(passCtx, batch)
				return err
			}}.Run(runCtx, onWorkerError)
			return nil
		},
	}

	referralAdmin, adminErr := adminresources.Runtime(ctx, db)
	if adminErr != nil {
		logging.Fatal("configuration", "Referral private Admin configuration invalid")
	}
	referralCommands, adminErr := admincommand.Runtime(referralAdmin, admincommands.Spec(), admincommands.Executor{Referral: service}, "REFERRAL")
	if adminErr != nil {
		logging.Fatal("configuration", "Referral private command configuration invalid")
	}

	serviceapp.Run(serviceapp.Options{
		Name:        "referral-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/referrals",
		Capabilities: []string{
			"referral codes and attribution",
			"customer and driver referral rewards",
			"referrer activity tracking",
			"milestones, reversals and fraud review",
			"driver cash-out checks and wallet reconciliation",
		},
		ReadyChecks: readyChecks,
		Register: func(group *gin.RouterGroup) {
			referralhttp.RegisterPublicRoutes(group, referralhttp.PublicOptions{
				Service:              service,
				CustomerTokenSecret:  cfg.CustomerTokenSecret,
				ProviderTokenSecrets: cfg.ProviderTokenSecrets,
				Limiter:              limiter,
			})
		},
		RegisterRoot: func(router *gin.Engine) {
			referralAdmin.RegisterOn(router.Group(""))
			referralCommands.Register(router)
			referralhttp.RegisterInternalRoutes(router, referralhttp.InternalOptions{
				Service:            service,
				Verifier:           serviceauth.NewVerifier(cfg.ServiceSecrets, 5*time.Minute),
				NonceStore:         db,
				AdminCallers:       cfg.AdminCallers,
				LegacyAdminEnabled: cfg.LegacyAdminEnabled,
				// Referral cash-outs are refused while this list is empty.
				WithdrawalCheckCallers: cfg.WithdrawalCheckCallers,
			})
		},
		Workers: workers,
	})
}

// rulesFrom maps configuration onto the programme rules.
func rulesFrom(p config.Programme) referralmodels.Rules {
	return referralmodels.Rules{
		Activity: referralmodels.ActivityRules{
			CustomerThreshold: p.CustomerActivityThreshold,
			ProviderThreshold: p.ProviderActivityThreshold,
			Window:            p.ActivityWindow,
		},
		CustomerReferralBonusKobo:   p.CustomerReferralBonusKobo,
		CustomerWelcomeCreditKobo:   p.CustomerWelcomeCreditKobo,
		FirstBookingCreditKobo:      p.FirstBookingCreditKobo,
		ProviderWelcomeCreditKobo:   p.ProviderWelcomeCreditKobo,
		ProviderWelcomeWithdrawable: p.ProviderWelcomeWithdrawable,
		ProviderCommissionBPS:       p.ProviderCommissionBPS,
		ProviderActivationJobs:      p.ProviderActivationJobs,
		ProviderWindow:              p.ProviderWindow,
		Milestone5CreditKobo:        p.Milestone5CreditKobo,
		Milestone10BonusKobo:        p.Milestone10BonusKobo,
		ActivityDecisionDelay:       p.ActivityDecisionDelay,
		ReversalReviewThreshold:     p.ReversalReviewThreshold,
	}
}
