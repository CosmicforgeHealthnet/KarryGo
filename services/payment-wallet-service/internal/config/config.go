package config

import (
	"cosmicforge/logistics/shared/go/redisx"
	"os"
	"strconv"
)

type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string
	Redis       redisx.Config
}

func Load() Config {
	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8105"),
		DatabaseURL: getEnv("PAYMENT_WALLET_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5437/payment_wallet_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("PAYMENT_WALLET_REDIS_ADDR", "localhost:6384"),
			Password: os.Getenv("PAYMENT_WALLET_REDIS_PASSWORD"),
			DB:       getEnvInt("PAYMENT_WALLET_REDIS_DB", 0),
		},
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
