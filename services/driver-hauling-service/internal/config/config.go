package config

import (
	"cosmicforge/logistics/shared/go/redisx"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv   string
	HTTPAddr string

	Migration bool

	DatabaseURL string
	Redis       redisx.Config

	// Provider token signing
	ProviderTokenSecret   []byte
	ProviderRefreshSecret []byte
	ProviderOTPSecret     []byte

	// Customer token verification (mirrors customer-service secret)
	CustomerTokenSecret []byte

	// Token TTLs
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	OTPTTL          time.Duration
	OTPRateWindow   time.Duration
	OTPMaxRequests  int
	OTPMaxAttempts  int
	OTPDebug        bool

	// Internal service clients
	NotificationURL    string
	NotificationSecret []byte
	PaymentURL         string
	PaymentSecret      []byte

	// ServiceSecrets are HMAC secrets for inbound internal callers (e.g.
	// support-dispute-service resolving a provider identity).
	ServiceSecrets string

	// Matching config
	BookingMatchTimeout int     // seconds a matched provider has to accept
	BookingSearchWindow int     // seconds to keep searching (rescanning) before unmatched
	ProviderOnlineTTL   int     // seconds provider stays online without heartbeat
	MatchMaxRadiusKm    float64 // providers farther than this from pickup are not dispatched
}

func Load() Config {
	return Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPAddr: getEnv("HTTP_ADDR", ":8104"),

		Migration: getEnvBool("HAULING_MIGRATION", false),

		DatabaseURL: getEnv("HAULING_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5436/hauling_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("HAULING_REDIS_ADDR", "localhost:6383"),
			Password: os.Getenv("HAULING_REDIS_PASSWORD"),
			DB:       getEnvInt("HAULING_REDIS_DB", 0),
		},

		ProviderTokenSecret:   []byte(getEnv("HAULING_PROVIDER_TOKEN_SECRET", "development-hauling-provider-token-secret")),
		ProviderRefreshSecret: []byte(getEnv("HAULING_PROVIDER_REFRESH_SECRET", "development-hauling-provider-refresh-secret")),
		ProviderOTPSecret:     []byte(getEnv("HAULING_PROVIDER_OTP_SECRET", "development-hauling-provider-otp-secret")),
		CustomerTokenSecret:   []byte(getEnv("HAULING_CUSTOMER_TOKEN_SECRET", "development-customer-token-secret")),

		AccessTokenTTL:  time.Duration(getEnvInt("HAULING_ACCESS_TOKEN_TTL_SECONDS", 3600)) * time.Second,
		RefreshTokenTTL: time.Duration(getEnvInt("HAULING_REFRESH_TOKEN_TTL_DAYS", 30)) * 24 * time.Hour,
		OTPTTL:          time.Duration(getEnvInt("HAULING_OTP_TTL_SECONDS", 600)) * time.Second,
		OTPRateWindow:   time.Duration(getEnvInt("HAULING_OTP_RATE_WINDOW_SECONDS", 60)) * time.Second,
		OTPMaxRequests:  getEnvInt("HAULING_OTP_MAX_REQUESTS", 5),
		OTPMaxAttempts:  getEnvInt("HAULING_OTP_MAX_ATTEMPTS", 5),
		OTPDebug:        getEnvBool("HAULING_OTP_DEBUG", false),

		NotificationURL:    os.Getenv("HAULING_NOTIFICATION_URL"),
		NotificationSecret: []byte(os.Getenv("HAULING_NOTIFICATION_SECRET")),
		PaymentURL:         os.Getenv("HAULING_PAYMENT_URL"),
		PaymentSecret:      []byte(os.Getenv("HAULING_PAYMENT_SECRET")),

		ServiceSecrets: getEnv("HAULING_SERVICE_SECRETS", "support-dispute-service=development-support-dispute-service-secret"),

		BookingMatchTimeout: getEnvInt("HAULING_BOOKING_MATCH_TIMEOUT", 30),
		BookingSearchWindow: getEnvInt("HAULING_BOOKING_SEARCH_WINDOW", 60),
		ProviderOnlineTTL:   getEnvInt("HAULING_PROVIDER_ONLINE_TTL", 7200),
		MatchMaxRadiusKm:    getEnvFloat("HAULING_MATCH_MAX_RADIUS_KM", 25),
	}
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func getEnv(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
