package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	AppEnv            string
	HTTPAddr          string
	DatabaseURL       string
	Migration         bool
	PrivateAPIEnabled bool
	ServiceSecrets    string
	AllowedCallers    string
}

func Load() Config {
	return Config{
		AppEnv:            getEnv("APP_ENV", "development"),
		HTTPAddr:          getEnv("HTTP_ADDR", ":8108"),
		DatabaseURL:       getEnv("VERIFICATION_COMPLIANCE_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5440/verification_compliance_service?sslmode=disable"),
		Migration:         getEnv("VERIFICATION_COMPLIANCE_MIGRATION", "false") == "true",
		PrivateAPIEnabled: getEnv("VERIFICATION_COMPLIANCE_PRIVATE_API_ENABLED", "false") == "true",
		ServiceSecrets:    os.Getenv("VERIFICATION_COMPLIANCE_SERVICE_SECRETS"),
		AllowedCallers:    getEnv("VERIFICATION_COMPLIANCE_ALLOWED_CALLERS", "fleet-partner-service"),
	}
}

func (c Config) Validate() error {
	var p []string
	if c.DatabaseURL == "" {
		p = append(p, "VERIFICATION_COMPLIANCE_DATABASE_URL is required")
	}
	if c.PrivateAPIEnabled {
		if strings.TrimSpace(c.ServiceSecrets) == "" {
			p = append(p, "VERIFICATION_COMPLIANCE_SERVICE_SECRETS is required")
		}
		if strings.TrimSpace(c.AllowedCallers) == "" {
			p = append(p, "VERIFICATION_COMPLIANCE_ALLOWED_CALLERS is required")
		}
	}
	if len(p) > 0 {
		return errors.New("configuration invalid: " + strings.Join(p, "; "))
	}
	return nil
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
