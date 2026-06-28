package config

import (
	"cosmicforge/logistics/shared/go/redisx"
	"cosmicforge/logistics/shared/go/serviceauth"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv   string
	HTTPAddr string

	DatabaseURL string
	Redis       redisx.Config

	CustomerAccessTokenSecret  []byte
	ProviderAccessTokenSecrets map[string][]byte
	ServiceSecrets             serviceauth.Secrets

	// Notification-service client (this service is a sender). When URL/secret are
	// empty the wallet notifier is a no-op.
	NotificationURL    string
	NotificationSecret []byte

	Paystack PaystackConfig

	PlatformFeeBPS        int64
	HTTPRequestTimeout    time.Duration
	WebhookMaxSkew        time.Duration
	WithdrawalMinKobo     int64
	WithdrawalMaxKobo     int64
	RequireManualPayouts  bool
	DefaultCurrency       string
	PublicCallbackBaseURL string
}

type PaystackConfig struct {
	BaseURL   string
	PublicKey string
	SecretKey string
}

func Load() Config {
	return Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPAddr: getEnv("HTTP_ADDR", ":8105"),

		DatabaseURL: getEnv("PAYMENT_WALLET_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5437/payment_wallet_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("PAYMENT_WALLET_REDIS_ADDR", "localhost:6384"),
			Password: os.Getenv("PAYMENT_WALLET_REDIS_PASSWORD"),
			DB:       getEnvInt("PAYMENT_WALLET_REDIS_DB", 0),
		},

		CustomerAccessTokenSecret: []byte(getEnv("PAYMENT_WALLET_CUSTOMER_ACCESS_TOKEN_SECRET", "development-customer-access-token-secret")),
		// Provider access-token secrets must match the secret each provider service
		// signs access tokens with. The hauling default mirrors
		// HAULING_PROVIDER_TOKEN_SECRET in driver-hauling-service. Production must set
		// matching values on both services explicitly.
		ProviderAccessTokenSecrets: parseSecretMap(getEnv("PAYMENT_WALLET_PROVIDER_ACCESS_TOKEN_SECRETS", "taxi=development-taxi-access-token-secret,dispatch=development-dispatch-access-token-secret,hauling=development-hauling-provider-token-secret")),
		// driver-hauling-service signs with HAULING_PAYMENT_SECRET; the key must
		// match that service's HMAC identity (driver-hauling-service) and secret.
		ServiceSecrets:             serviceauth.ParseSecrets(getEnv("PAYMENT_WALLET_SERVICE_SECRETS", "taxi-service=development-payment-wallet-service-secret,dispatch-delivery-service=development-payment-wallet-service-secret,driver-hauling-service=development-hauling-payment-secret,admin-backoffice-service=development-payment-wallet-service-secret")),

		NotificationURL:    os.Getenv("PAYMENT_WALLET_NOTIFICATION_URL"),
		NotificationSecret: []byte(os.Getenv("PAYMENT_WALLET_NOTIFICATION_SECRET")),

		Paystack: PaystackConfig{
			BaseURL:   getEnv("PAYMENT_WALLET_PAYSTACK_BASE_URL", "https://api.paystack.co"),
			PublicKey: os.Getenv("PAYMENT_WALLET_PAYSTACK_PUBLIC_KEY"),
			SecretKey: os.Getenv("PAYMENT_WALLET_PAYSTACK_SECRET_KEY"),
		},

		PlatformFeeBPS:        int64(getEnvInt("PAYMENT_WALLET_PLATFORM_FEE_BPS", 1500)),
		HTTPRequestTimeout:    time.Duration(getEnvInt("PAYMENT_WALLET_HTTP_TIMEOUT_SECONDS", 15)) * time.Second,
		WebhookMaxSkew:        time.Duration(getEnvInt("PAYMENT_WALLET_WEBHOOK_MAX_SKEW_SECONDS", 300)) * time.Second,
		WithdrawalMinKobo:     int64(getEnvInt("PAYMENT_WALLET_WITHDRAWAL_MIN_KOBO", 10000)),
		WithdrawalMaxKobo:     int64(getEnvInt("PAYMENT_WALLET_WITHDRAWAL_MAX_KOBO", 500000000)),
		RequireManualPayouts:  getEnvBool("PAYMENT_WALLET_REQUIRE_MANUAL_PAYOUTS", true),
		DefaultCurrency:       getEnv("PAYMENT_WALLET_DEFAULT_CURRENCY", "NGN"),
		PublicCallbackBaseURL: os.Getenv("PAYMENT_WALLET_PUBLIC_CALLBACK_BASE_URL"),
	}
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

func parseSecretMap(value string) map[string][]byte {
	secrets := map[string][]byte{}
	for service, secret := range serviceauth.ParseSecrets(value) {
		secrets[service] = secret
	}
	return secrets
}
