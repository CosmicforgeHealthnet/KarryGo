package config

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	AppEnv               string
	HTTPAddr             string
	DatabaseURL          string
	Migration            bool
	PrivateAPIEnabled    bool
	ServiceSecrets       string
	AllowedCallers       string
	VendorCallbackURL    string
	VendorCallbackSecret string
}

func Load() Config {
	return Config{
		AppEnv:               getEnv("APP_ENV", "development"),
		HTTPAddr:             getEnv("HTTP_ADDR", ":8108"),
		DatabaseURL:          getEnv("VERIFICATION_COMPLIANCE_DATABASE_URL", "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5440/verification_compliance_service?sslmode=disable"),
		Migration:            getEnv("VERIFICATION_COMPLIANCE_MIGRATION", "false") == "true",
		PrivateAPIEnabled:    getEnv("VERIFICATION_COMPLIANCE_PRIVATE_API_ENABLED", "false") == "true",
		ServiceSecrets:       os.Getenv("VERIFICATION_COMPLIANCE_SERVICE_SECRETS"),
		AllowedCallers:       getEnv("VERIFICATION_COMPLIANCE_ALLOWED_CALLERS", "fleet-partner-service,vendor-service,admin-backoffice-service"),
		VendorCallbackURL:    strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_URL")),
		VendorCallbackSecret: strings.TrimSpace(os.Getenv("VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_SECRET")),
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
	if (c.VendorCallbackURL == "") != (c.VendorCallbackSecret == "") {
		p = append(p, "VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_URL and VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_SECRET must be configured together")
	}
	if c.VendorCallbackURL != "" {
		parsed, err := url.ParseRequestURI(c.VendorCallbackURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			p = append(p, "VERIFICATION_COMPLIANCE_VENDOR_CALLBACK_URL must be an absolute http(s) URL")
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
