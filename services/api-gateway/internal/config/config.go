package config

import (
	"os"
	"strconv"
	"time"

	"cosmicforge/logistics/shared/go/redisx"
)

type Config struct {
	AppEnv        string
	HTTPAddr      string
	DatabaseURL   string
	Redis         redisx.Config
	Wallet        WalletConfig
	WebhookSecret string // WEBHOOK_SIGNING_SECRET: key material for encrypting webhook secrets at rest

	// Webhook delivery worker settings.
	WebhookMaxRetries      int           // WEBHOOK_MAX_RETRIES (default 5)
	WebhookDeliveryTimeout time.Duration // WEBHOOK_DELIVERY_TIMEOUT_SECONDS (default 30s)

	// PartnerAuth holds partner dashboard session settings (separate from any
	// dispatch/customer/provider JWT secret).
	PartnerAuth PartnerAuthConfig
	// Pricing holds the fare engine parameters.
	Pricing PricingConfig
	// Billing holds the partner Billing & Usage parameters.
	Billing BillingConfig
}

// WalletConfig holds the payment-wallet client settings. The URL is a bare
// origin (e.g. http://localhost:8105); the shared walletclient appends the path.
type WalletConfig struct {
	URL         string
	ServiceName string
	Secret      string
}

// PartnerAuthConfig holds partner dashboard auth + internal service settings.
type PartnerAuthConfig struct {
	JWTSecret          string        // PARTNER_JWT_SECRET (dashboard sessions only)
	JWTTTL             time.Duration // derived from PARTNER_JWT_TTL_HOURS (default 24h)
	InternalServiceKey string        // PARTNER_INTERNAL_SERVICE_KEY (internal routes only)
}

// PricingConfig holds the fare engine parameters (all kobo except the multiplier).
type PricingConfig struct {
	BaseKobo          int64
	StandardPerKmKobo int64
	ExpressPerKmKobo  int64
	LargeMultiplier   float64
}

// BillingConfig holds the partner Billing & Usage parameters. RatePerDeliveryKobo
// is a single fixed rate applied to every partner (not per-partner pricing) -
// this is a placeholder default (100 NGN) and MUST be set to the real agreed
// price via BILLING_RATE_PER_DELIVERY_KOBO before this is used for anything
// real; nothing in this codebase currently validates or challenges that value.
type BillingConfig struct {
	RatePerDeliveryKobo int64
}

func Load() Config {
	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("API_GATEWAY_DATABASE_URL"),
		Redis: redisx.Config{
			Addr:     getEnv("API_GATEWAY_REDIS_ADDR", "localhost:6379"),
			Password: os.Getenv("API_GATEWAY_REDIS_PASSWORD"),
			DB:       getEnvInt("API_GATEWAY_REDIS_DB", 0),
		},
		Wallet: WalletConfig{
			URL:         getEnv("API_GATEWAY_WALLET_SERVICE_URL", "http://localhost:8105"),
			ServiceName: getEnv("API_GATEWAY_WALLET_SERVICE_NAME", "api-gateway"),
			Secret:      os.Getenv("API_GATEWAY_WALLET_SERVICE_SECRET"),
		},
		WebhookSecret:          os.Getenv("WEBHOOK_SIGNING_SECRET"),
		WebhookMaxRetries:      getEnvInt("WEBHOOK_MAX_RETRIES", 5),
		WebhookDeliveryTimeout: time.Duration(getEnvInt("WEBHOOK_DELIVERY_TIMEOUT_SECONDS", 30)) * time.Second,
		PartnerAuth: PartnerAuthConfig{
			JWTSecret:          os.Getenv("PARTNER_JWT_SECRET"),
			JWTTTL:             time.Duration(getEnvInt("PARTNER_JWT_TTL_HOURS", 24)) * time.Hour,
			InternalServiceKey: os.Getenv("PARTNER_INTERNAL_SERVICE_KEY"),
		},
		Pricing: PricingConfig{
			BaseKobo:          getEnvInt64("PRICING_BASE_KOBO", 50000),
			StandardPerKmKobo: getEnvInt64("PRICING_STANDARD_PER_KM_KOBO", 15000),
			ExpressPerKmKobo:  getEnvInt64("PRICING_EXPRESS_PER_KM_KOBO", 25000),
			LargeMultiplier:   getEnvFloat("PRICING_LARGE_MULTIPLIER", 1.5),
		},
		Billing: BillingConfig{
			// 10000 kobo (₦100.00) is a PLACEHOLDER, not a real price. Set
			// BILLING_RATE_PER_DELIVERY_KOBO in .env before this touches real invoices.
			RatePerDeliveryKobo: getEnvInt64("BILLING_RATE_PER_DELIVERY_KOBO", 10000),
		},
	}
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

func getEnvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
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