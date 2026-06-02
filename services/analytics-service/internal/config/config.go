package config

import "os"

type Config struct {
	AppEnv      string
	HTTPAddr    string
	DatabaseURL string
}

func Load() Config {
	return Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8111"),
		DatabaseURL: getEnv("ANALYTICS_DATABASE_URL", "postgres://karrygo:karrygo@localhost:5443/analytics_service?sslmode=disable"),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
