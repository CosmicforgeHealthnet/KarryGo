package config

import (
	"os"
	"strconv"
	"time"

	"cosmicforge/logistics/shared/go/redisx"
)

type Config struct {
	AppEnv         string
	HTTPAddr       string
	DatabaseURL    string
	Migration      bool
	ServiceSecrets string
	AllowedCallers string
	// Section 13 network analytics.
	NetworkEventCallers string
	NetworkReadCallers  string
	AdminCaller         string
	Redis               redisx.Config
	ReadRatePrefix      string
	ReadRateMax         int
	ReadRateWindow      time.Duration
}

func Load() Config {
	return Config{
		AppEnv:         getEnv("APP_ENV", "development"),
		HTTPAddr:       getEnv("HTTP_ADDR", ":8111"),
		DatabaseURL:    getEnv("ANALYTICS_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5443/analytics_service?sslmode=disable"),
		Migration:      getEnvBool("MIGRATION", false),
		ServiceSecrets: getEnv("ANALYTICS_SERVICE_SECRETS", "vendor-service=development-analytics-service-secret,support-dispute-service=development-analytics-service-secret,driver-dispatch-delivery-service=development-analytics-service-secret,driver-hauling-service=development-analytics-service-secret,driver-taxi-service=development-analytics-service-secret,fleet-partner-service=development-analytics-service-secret,admin-backoffice-service=development-analytics-service-secret"),
		AllowedCallers: getEnv("ANALYTICS_COMMERCE_EVENT_CALLERS", "vendor-service,support-dispute-service,driver-dispatch-delivery-service,driver-hauling-service"),
		// Producers of network events, and the services that consult the
		// advisory reads. Each is additionally bound to its own facts: an
		// event's source must be its signer, and an execution service reads
		// provider quality for its own providers only.
		NetworkEventCallers: getEnv("ANALYTICS_NETWORK_EVENT_CALLERS", "driver-taxi-service,driver-dispatch-delivery-service,driver-hauling-service,vendor-service"),
		NetworkReadCallers:  getEnv("ANALYTICS_NETWORK_READ_CALLERS", "vendor-service,driver-taxi-service,driver-dispatch-delivery-service,driver-hauling-service,fleet-partner-service"),
		AdminCaller:         getEnv("ANALYTICS_ADMIN_CALLER", "admin-backoffice-service"),
		Redis: redisx.Config{
			Addr:     getEnv("ANALYTICS_REDIS_ADDR", "localhost:6379"),
			Password: getEnv("ANALYTICS_REDIS_PASSWORD", ""),
			DB:       getEnvInt("ANALYTICS_REDIS_DB", 0),
		},
		ReadRatePrefix: getEnv("ANALYTICS_READ_RATE_LIMIT_REDIS_PREFIX", "analytics:abuse:ratelimit"),
		ReadRateMax:    getEnvInt("ANALYTICS_READ_RATE_MAX", 60),
		ReadRateWindow: time.Duration(getEnvInt("ANALYTICS_READ_RATE_WINDOW_SECONDS", 60)) * time.Second,
	}
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
	return value == "true" || value == "1" || value == "TRUE"
}
