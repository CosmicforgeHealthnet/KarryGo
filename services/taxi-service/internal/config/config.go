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
		HTTPAddr:    getEnv("HTTP_ADDR", ":8102"),
		DatabaseURL: getEnv("TAXI_DATABASE_URL", "postgres://karrygo:karrygo@localhost:5434/taxi_service?sslmode=disable"),
		Redis: redisx.Config{
			Addr:     getEnv("TAXI_REDIS_ADDR", "localhost:6381"),
			Password: os.Getenv("TAXI_REDIS_PASSWORD"),
			DB:       getEnvInt("TAXI_REDIS_DB", 0),
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
