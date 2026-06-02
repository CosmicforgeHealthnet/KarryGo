package config

import (
	"os"
	"strconv"
	"time"

	"karrygo/shared/go/redisx"
)

type Config struct {
	AppEnv   string
	HTTPAddr string

	DatabaseURL string
	Redis       redisx.Config

	AccessTokenSecret  []byte
	RefreshTokenSecret []byte
	OTPSecret          []byte

	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	OTPTTL          time.Duration
	OTPRateWindow   time.Duration

	OTPMaxRequests int
	OTPMaxAttempts int
	OTPDebug       bool
}

func Load() Config {
	return Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPAddr: getEnv("HTTP_ADDR", ":8101"),

		DatabaseURL: getEnv("CUSTOMER_DATABASE_URL", "postgres://karrygo:karrygo@localhost:5433/customer_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("CUSTOMER_REDIS_ADDR", "localhost:6380"),
			Password: os.Getenv("CUSTOMER_REDIS_PASSWORD"),
			DB:       getEnvInt("CUSTOMER_REDIS_DB", 0),
		},

		AccessTokenSecret:  []byte(getEnv("CUSTOMER_ACCESS_TOKEN_SECRET", "development-customer-access-token-secret")),
		RefreshTokenSecret: []byte(getEnv("CUSTOMER_REFRESH_TOKEN_SECRET", "development-customer-refresh-token-secret")),
		OTPSecret:          []byte(getEnv("CUSTOMER_OTP_SECRET", "development-customer-otp-secret")),

		AccessTokenTTL:  time.Duration(getEnvInt("CUSTOMER_ACCESS_TOKEN_TTL_SECONDS", 900)) * time.Second,
		RefreshTokenTTL: time.Duration(getEnvInt("CUSTOMER_REFRESH_TOKEN_TTL_HOURS", 720)) * time.Hour,
		OTPTTL:          time.Duration(getEnvInt("CUSTOMER_OTP_TTL_SECONDS", 300)) * time.Second,
		OTPRateWindow:   time.Duration(getEnvInt("CUSTOMER_OTP_RATE_LIMIT_WINDOW_SECONDS", 600)) * time.Second,

		OTPMaxRequests: getEnvInt("CUSTOMER_OTP_MAX_REQUESTS", 5),
		OTPMaxAttempts: getEnvInt("CUSTOMER_OTP_MAX_ATTEMPTS", 5),
		OTPDebug:       getEnvBool("CUSTOMER_DEBUG_OTP", false),
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
