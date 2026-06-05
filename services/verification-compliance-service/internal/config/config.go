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
		HTTPAddr:    getEnv("HTTP_ADDR", ":8108"),
		DatabaseURL: getEnv("VERIFICATION_COMPLIANCE_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5440/verification_compliance_service?sslmode=disable"),
	}
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
