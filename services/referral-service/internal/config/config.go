// Package config loads referral-service settings from the environment.
//
// Every integration is off (or refuses to start) rather than insecurely
// defaulted: the event feed needs caller secrets, wallet crediting needs a
// wallet URL and secret, and login-token checks need each issuer's secret.
// Programme amounts and thresholds default to the approved launch values.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/redisx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Config is the full service configuration.
type Config struct {
	AppEnv      string
	HTTPAddr    string
	Migration   bool
	DatabaseURL string
	// Redis is optional; when Addr is empty the public code-check endpoint is
	// not rate limited (acceptable only for local development).
	Redis redisx.Config

	// ServiceSecrets are the HMAC secrets of every service allowed to call
	// referral-service's internal routes ("caller=secret,...").
	ServiceSecrets serviceauth.Secrets
	// AdminCallers may use the admin routes (freeze, clear, reverse).
	AdminCallers []string
	// WithdrawalCheckCallers may ask whether a person may cash out.
	WithdrawalCheckCallers []string

	// Login-token secrets: each must equal the secret the issuing service signs
	// its access tokens with.
	CustomerTokenSecret  []byte
	ProviderTokenSecrets map[string][]byte // keyed by actor type: hauling, dispatch, taxi

	// ShareLinkBase is prepended to a code to build its share link.
	ShareLinkBase string

	// Wallet crediting. Both empty disables crediting (rewards accumulate
	// with wallet_state=pending and are posted once configured).
	WalletURL    string
	WalletSecret []byte

	Programme Programme
	Workers   Workers
}

// Programme holds the business rules' numbers. Amounts are kobo.
type Programme struct {
	CustomerReferralBonusKobo int64
	CustomerWelcomeCreditKobo int64
	// FirstBookingCreditKobo replaces "10% off the first booking" with a
	// one-time spend-only credit of this amount.
	FirstBookingCreditKobo    int64
	ProviderWelcomeCreditKobo int64
	// ProviderWelcomeWithdrawable pays the driver welcome credit as
	// withdrawable referral earnings instead of spend-only promotional
	// credit. Off by default, per the spec's "welcome credit is promotional"
	// rule.
	ProviderWelcomeWithdrawable bool
	ProviderCommissionBPS       int64
	ProviderActivationJobs      int
	ProviderWindow              time.Duration
	CustomerActivityThreshold   int
	ProviderActivityThreshold   int
	ActivityWindow              time.Duration
	// Milestone5CreditKobo funds the "free standard delivery" voucher at five
	// qualified customer referrals, as spend-only credit.
	Milestone5CreditKobo  int64
	Milestone10BonusKobo  int64
	ActivityDecisionDelay time.Duration
	// ReversalReviewThreshold puts a referral under review each time this
	// many more of its jobs have been reversed; 0 turns the check off.
	ReversalReviewThreshold int
}

// Workers tunes the background workers.
type Workers struct {
	DecisionInterval time.Duration
	WalletInterval   time.Duration
	ExpiryInterval   time.Duration
	// ReconciliationInterval is how often postings are checked against the
	// wallet's ledger (daily by default).
	ReconciliationInterval time.Duration
	BatchSize              int
}

// Load reads the environment.
func Load() Config {
	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8114"),
		Migration:   getEnvBool("REFERRAL_MIGRATION", false),
		DatabaseURL: getEnv("REFERRAL_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5450/referral_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     os.Getenv("REFERRAL_REDIS_ADDR"),
			Password: os.Getenv("REFERRAL_REDIS_PASSWORD"),
			DB:       getEnvInt("REFERRAL_REDIS_DB", 0),
		},

		ServiceSecrets:         serviceauth.ParseSecrets(os.Getenv("REFERRAL_SERVICE_SECRETS")),
		AdminCallers:           splitCSV(getEnv("REFERRAL_ADMIN_CALLERS", "admin-backoffice-service")),
		WithdrawalCheckCallers: splitCSV(getEnv("REFERRAL_WITHDRAWAL_CHECK_CALLERS", "payment-wallet-service")),

		CustomerTokenSecret:  []byte(os.Getenv("REFERRAL_CUSTOMER_TOKEN_SECRET")),
		ProviderTokenSecrets: parseSecretMap(os.Getenv("REFERRAL_PROVIDER_TOKEN_SECRETS")),

		ShareLinkBase: getEnv("REFERRAL_SHARE_LINK_BASE", "https://karry-go.vercel.app/r/"),

		WalletURL:    os.Getenv("REFERRAL_WALLET_URL"),
		WalletSecret: []byte(os.Getenv("REFERRAL_WALLET_SECRET")),

		Programme: Programme{
			CustomerReferralBonusKobo:   getEnvInt64("REFERRAL_CUSTOMER_BONUS_KOBO", 50_000),
			CustomerWelcomeCreditKobo:   getEnvInt64("REFERRAL_CUSTOMER_WELCOME_CREDIT_KOBO", 20_000),
			FirstBookingCreditKobo:      getEnvInt64("REFERRAL_FIRST_BOOKING_CREDIT_KOBO", 100_000),
			ProviderWelcomeCreditKobo:   getEnvInt64("REFERRAL_PROVIDER_WELCOME_CREDIT_KOBO", 50_000),
			ProviderWelcomeWithdrawable: getEnvBool("REFERRAL_PROVIDER_WELCOME_WITHDRAWABLE", false),
			ProviderCommissionBPS:       getEnvInt64("REFERRAL_PROVIDER_COMMISSION_BPS", 1_000),
			ProviderActivationJobs:      getEnvInt("REFERRAL_PROVIDER_ACTIVATION_JOBS", 5),
			ProviderWindow:              time.Duration(getEnvInt("REFERRAL_PROVIDER_WINDOW_DAYS", 90)) * 24 * time.Hour,
			CustomerActivityThreshold:   getEnvInt("REFERRAL_CUSTOMER_ACTIVITY_THRESHOLD", 5),
			ProviderActivityThreshold:   getEnvInt("REFERRAL_PROVIDER_ACTIVITY_THRESHOLD", 10),
			ActivityWindow:              time.Duration(getEnvInt("REFERRAL_ACTIVITY_WINDOW_DAYS", 30)) * 24 * time.Hour,
			Milestone5CreditKobo:        getEnvInt64("REFERRAL_MILESTONE_5_CREDIT_KOBO", 150_000),
			Milestone10BonusKobo:        getEnvInt64("REFERRAL_MILESTONE_10_BONUS_KOBO", 750_000),
			ActivityDecisionDelay:       time.Duration(getEnvInt("REFERRAL_ACTIVITY_DECISION_DELAY_MINUTES", 60)) * time.Minute,
			ReversalReviewThreshold:     getEnvInt("REFERRAL_REVERSAL_REVIEW_THRESHOLD", 3),
		},
		Workers: Workers{
			DecisionInterval:       time.Duration(getEnvInt("REFERRAL_DECISION_INTERVAL_SECONDS", 30)) * time.Second,
			WalletInterval:         time.Duration(getEnvInt("REFERRAL_WALLET_INTERVAL_SECONDS", 10)) * time.Second,
			ExpiryInterval:         time.Duration(getEnvInt("REFERRAL_EXPIRY_INTERVAL_SECONDS", 3600)) * time.Second,
			ReconciliationInterval: time.Duration(getEnvInt("REFERRAL_RECONCILIATION_INTERVAL_SECONDS", 86400)) * time.Second,
			BatchSize:              getEnvInt("REFERRAL_WORKER_BATCH_SIZE", 100),
		},
	}
}

// Validate reports every configuration problem at once.
func (c Config) Validate() error {
	var problems []string
	if strings.TrimSpace(c.DatabaseURL) == "" {
		problems = append(problems, "REFERRAL_DATABASE_URL is required")
	}
	if (c.WalletURL == "") != (len(c.WalletSecret) == 0) {
		problems = append(problems, "REFERRAL_WALLET_URL and REFERRAL_WALLET_SECRET must be set together")
	}
	if c.WalletURL != "" {
		if parsed, err := url.ParseRequestURI(c.WalletURL); err != nil || parsed.Host == "" {
			problems = append(problems, "REFERRAL_WALLET_URL must be an absolute URL (bare origin)")
		}
	}
	for actor := range c.ProviderTokenSecrets {
		switch actor {
		case "hauling", "dispatch", "taxi":
		default:
			problems = append(problems, fmt.Sprintf("REFERRAL_PROVIDER_TOKEN_SECRETS has unknown actor type %q (want hauling, dispatch or taxi)", actor))
		}
	}
	p := c.Programme
	for name, value := range map[string]int64{
		"REFERRAL_CUSTOMER_BONUS_KOBO":          p.CustomerReferralBonusKobo,
		"REFERRAL_CUSTOMER_WELCOME_CREDIT_KOBO": p.CustomerWelcomeCreditKobo,
		"REFERRAL_FIRST_BOOKING_CREDIT_KOBO":    p.FirstBookingCreditKobo,
		"REFERRAL_PROVIDER_WELCOME_CREDIT_KOBO": p.ProviderWelcomeCreditKobo,
		"REFERRAL_MILESTONE_5_CREDIT_KOBO":      p.Milestone5CreditKobo,
		"REFERRAL_MILESTONE_10_BONUS_KOBO":      p.Milestone10BonusKobo,
	} {
		if value < 0 {
			problems = append(problems, name+" cannot be negative")
		}
	}
	if p.ProviderCommissionBPS < 0 || p.ProviderCommissionBPS > 10_000 {
		problems = append(problems, "REFERRAL_PROVIDER_COMMISSION_BPS must be between 0 and 10000")
	}
	if p.ProviderActivationJobs < 1 || p.CustomerActivityThreshold < 1 || p.ProviderActivityThreshold < 1 {
		problems = append(problems, "activation and activity thresholds must be at least 1")
	}
	if p.ReversalReviewThreshold < 0 {
		problems = append(problems, "REFERRAL_REVERSAL_REVIEW_THRESHOLD cannot be negative")
	}
	if p.ProviderWindow <= 0 || p.ActivityWindow <= 0 || p.ActivityDecisionDelay < 0 {
		problems = append(problems, "programme windows must be positive and the decision delay non-negative")
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid referral-service configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

func splitCSV(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseSecretMap(value string) map[string][]byte {
	secrets := map[string][]byte{}
	for name, secret := range serviceauth.ParseSecrets(value) {
		secrets[name] = secret
	}
	return secrets
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
