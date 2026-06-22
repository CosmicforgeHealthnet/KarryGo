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
	ServiceSecrets            string

	AccessTokenTTL time.Duration
}

func Load() Config {
	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8107"),
		DatabaseURL: getEnv("SUPPORT_DISPUTE_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5439/support_dispute_service?sslmode=disable"),
		Migration:   getEnvBool("MIGRATION", false),

		CustomerAccessTokenSecret: getEnv("SUPPORT_DISPUTE_CUSTOMER_ACCESS_TOKEN_SECRET", "development-customer-access-token-secret"),
		ServiceSecrets:            getEnv("SUPPORT_DISPUTE_SERVICE_SECRETS", "customer-service=development-support-dispute-service-secret,taxi-service=development-support-dispute-service-secret,dispatch-delivery-service=development-support-dispute-service-secret,hauling-service=development-support-dispute-service-secret,admin-backoffice-service=development-support-dispute-service-secret"),
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
