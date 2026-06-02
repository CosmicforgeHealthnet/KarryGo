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
		HTTPAddr:    getEnv("HTTP_ADDR", ":8104"),
		DatabaseURL: getEnv("HAULING_DATABASE_URL", "postgres://karrygo:karrygo@localhost:5436/hauling_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("HAULING_REDIS_ADDR", "localhost:6383"),
			Password: os.Getenv("HAULING_REDIS_PASSWORD"),
			DB:       getEnvInt("HAULING_REDIS_DB", 0),
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
