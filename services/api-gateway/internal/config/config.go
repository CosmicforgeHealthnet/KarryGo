package config

import (
	"cosmicforge/logistics/shared/go/redisx"
	"os"
	"strconv"
)

type Config struct {
	AppEnv   string
	HTTPAddr string
	Redis    redisx.Config
}

func Load() Config {
	return Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPAddr: getEnv("HTTP_ADDR", ":8080"),
		Redis: redisx.Config{
			Addr:     getEnv("API_GATEWAY_REDIS_ADDR", "localhost:6379"),
			Password: os.Getenv("API_GATEWAY_REDIS_PASSWORD"),
			DB:       getEnvInt("API_GATEWAY_REDIS_DB", 0),
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
