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
		HTTPAddr:    getEnv("HTTP_ADDR", ":8110"),
		DatabaseURL: getEnv("ADMIN_BACKOFFICE_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5442/admin_backoffice_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("ADMIN_BACKOFFICE_REDIS_ADDR", "localhost:6386"),
			Password: os.Getenv("ADMIN_BACKOFFICE_REDIS_PASSWORD"),
			DB:       getEnvInt("ADMIN_BACKOFFICE_REDIS_DB", 0),
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
