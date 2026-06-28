package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string
	Migration   bool

	CustomerAccessTokenSecret string
	// Provider bearer secrets verify provider tokens issued by each driver
	// service (role+service claim must match). Must equal each driver service's
	// PROVIDER_TOKEN_SECRET.
	HaulingProviderTokenSecret  string
	TaxiProviderTokenSecret     string
	DispatchProviderTokenSecret string
	ServiceSecrets              string

	// Outbound clients (bare origins). Empty disables the feature (local dev).
	NotificationURL    string
	NotificationSecret string
	PaymentURL         string
	PaymentSecret      string

	// Owning-service identity lookup endpoints (bare origins) + shared HMAC
	// secrets. Empty disables enrichment for that service.
	CustomerServiceURL    string
	CustomerServiceSecret string
	HaulingServiceURL     string
	HaulingServiceSecret  string
	TaxiServiceURL        string
	TaxiServiceSecret     string
	DispatchServiceURL    string
	DispatchServiceSecret string

	AccessTokenTTL time.Duration
}

func Load() Config {
	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8107"),
		DatabaseURL: getEnv("SUPPORT_DISPUTE_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5439/support_dispute_service?sslmode=disable"),
		Migration:   getEnvBool("MIGRATION", false),

		CustomerAccessTokenSecret:   getEnv("SUPPORT_DISPUTE_CUSTOMER_ACCESS_TOKEN_SECRET", "development-customer-access-token-secret"),
		HaulingProviderTokenSecret:  getEnv("SUPPORT_DISPUTE_HAULING_PROVIDER_TOKEN_SECRET", "development-hauling-provider-token-secret"),
		TaxiProviderTokenSecret:     getEnv("SUPPORT_DISPUTE_TAXI_PROVIDER_TOKEN_SECRET", "development-taxi-provider-token-secret"),
		DispatchProviderTokenSecret: getEnv("SUPPORT_DISPUTE_DISPATCH_PROVIDER_TOKEN_SECRET", "development-dispatch-provider-token-secret"),
		ServiceSecrets:              getEnv("SUPPORT_DISPUTE_SERVICE_SECRETS", "customer-service=development-support-dispute-service-secret,taxi-service=development-support-dispute-service-secret,dispatch-delivery-service=development-support-dispute-service-secret,hauling-service=development-support-dispute-service-secret,admin-backoffice-service=development-support-dispute-service-secret"),

		// Bare origins only. Empty = feature disabled.
		NotificationURL:    getEnv("SUPPORT_DISPUTE_NOTIFICATION_URL", ""),
		NotificationSecret: getEnv("SUPPORT_DISPUTE_NOTIFICATION_SECRET", "development-support-dispute-notification-secret"),
		PaymentURL:         getEnv("SUPPORT_DISPUTE_PAYMENT_URL", ""),
		PaymentSecret:      getEnv("SUPPORT_DISPUTE_PAYMENT_SECRET", "development-support-dispute-payment-secret"),

		CustomerServiceURL:    getEnv("SUPPORT_DISPUTE_CUSTOMER_SERVICE_URL", ""),
		CustomerServiceSecret: getEnv("SUPPORT_DISPUTE_CUSTOMER_SERVICE_SECRET", "development-support-dispute-service-secret"),
		HaulingServiceURL:     getEnv("SUPPORT_DISPUTE_HAULING_SERVICE_URL", ""),
		HaulingServiceSecret:  getEnv("SUPPORT_DISPUTE_HAULING_SERVICE_SECRET", "development-support-dispute-service-secret"),
		TaxiServiceURL:        getEnv("SUPPORT_DISPUTE_TAXI_SERVICE_URL", ""),
		TaxiServiceSecret:     getEnv("SUPPORT_DISPUTE_TAXI_SERVICE_SECRET", "development-support-dispute-service-secret"),
		DispatchServiceURL:    getEnv("SUPPORT_DISPUTE_DISPATCH_SERVICE_URL", ""),
		DispatchServiceSecret: getEnv("SUPPORT_DISPUTE_DISPATCH_SERVICE_SECRET", "development-support-dispute-service-secret"),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
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
