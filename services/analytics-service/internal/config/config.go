package config

import (
	"cosmicforge/logistics/shared/go/serviceauth"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
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

// Validate prevents a staging/production deployment from inheriting local-only
// credentials. It reports configuration names, never credential values.
func (c Config) Validate() error {
	if c.AppEnv != "staging" && c.AppEnv != "production" {
		return nil
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.User == nil {
		return errors.New("ANALYTICS_DATABASE_URL must identify the private Analytics database")
	}
	password, ok := u.User.Password()
	if !ok || len(password) < 24 || password == "cosmicforge_logistics" {
		return errors.New("ANALYTICS_DATABASE_URL requires an explicit database credential")
	}
	if strings.TrimSpace(c.Redis.Addr) == "" || len(c.Redis.Password) < 24 {
		return errors.New("Analytics requires an explicit private Redis address and credential")
	}
	secrets := serviceauth.ParseSecrets(c.ServiceSecrets)
	if len(secrets) == 0 {
		return errors.New("ANALYTICS_SERVICE_SECRETS is required")
	}
	seen := map[string]bool{}
	for _, secret := range secrets {
		value := string(secret)
		if len(secret) < 32 || strings.Contains(strings.ToLower(value), "development-") || seen[value] {
			return errors.New("ANALYTICS_SERVICE_SECRETS requires distinct non-development credentials of at least 32 bytes")
		}
		seen[value] = true
	}
	for _, list := range []string{c.AllowedCallers, c.NetworkEventCallers, c.NetworkReadCallers, c.AdminCaller} {
		for _, caller := range strings.Split(list, ",") {
			caller = strings.TrimSpace(caller)
			if caller == "" || len(secrets[caller]) < 32 {
				return errors.New("Every configured Analytics caller requires its own service credential")
			}
		}
	}
	return nil
}
