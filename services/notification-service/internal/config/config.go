package config

import (
	"karrygo/shared/go/redisx"
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
		HTTPAddr:    getEnv("HTTP_ADDR", ":8106"),
		DatabaseURL: getEnv("NOTIFICATION_DATABASE_URL", "postgres://karrygo:karrygo@localhost:5438/notification_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("NOTIFICATION_REDIS_ADDR", "localhost:6385"),
			Password: os.Getenv("NOTIFICATION_REDIS_PASSWORD"),
			DB:       getEnvInt("NOTIFICATION_REDIS_DB", 0),
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
