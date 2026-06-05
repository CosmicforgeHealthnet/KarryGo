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
		HTTPAddr:    getEnv("HTTP_ADDR", ":8107"),
		DatabaseURL: getEnv("SUPPORT_DISPUTE_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5439/support_dispute_service?sslmode=disable"),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
