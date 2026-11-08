package config

import (
	"cosmicforge/logistics/shared/go/redisx"
	"strings"
	"testing"
)

func stagingConfig() Config {
	return Config{AppEnv: "staging", DatabaseURL: "postgres://analytics:" + strings.Repeat("d", 32) + "@analytics-db/analytics?sslmode=disable", Redis: redisx.Config{Addr: "analytics-redis:6379", Password: strings.Repeat("r", 32)}, ServiceSecrets: "producer=" + strings.Repeat("p", 32) + ",admin-backoffice-service=" + strings.Repeat("b", 32), AllowedCallers: "producer", NetworkEventCallers: "producer", NetworkReadCallers: "producer", AdminCaller: "admin-backoffice-service"}
}
func TestStagingRequiresExplicitCredentials(t *testing.T) {
	good := stagingConfig()
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*Config){
		"development service secret": func(c *Config) { c.ServiceSecrets = "producer=development-analytics-service-secret" },
		"missing caller key":         func(c *Config) { c.NetworkEventCallers = "unenrolled-producer" },
		"reused audience key": func(c *Config) {
			c.ServiceSecrets = "producer=" + strings.Repeat("p", 32) + ",admin-backoffice-service=" + strings.Repeat("p", 32)
		},
		"short database credential": func(c *Config) { c.DatabaseURL = "postgres://analytics:default@analytics-db/analytics" },
		"missing redis credential":  func(c *Config) { c.Redis.Password = "" },
		"empty caller":              func(c *Config) { c.AllowedCallers = "producer," },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c := stagingConfig()
			mutate(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("unsafe staging configuration accepted")
			}
		})
	}
}
func TestDevelopmentConfigurationRemainsLocal(t *testing.T) {
	if err := (Config{AppEnv: "development"}).Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestProductionRequiresSameCredentialBoundary(t *testing.T) {
	c := stagingConfig()
	c.AppEnv = "production"
	c.Redis.Password = ""
	if err := c.Validate(); err == nil {
		t.Fatal("production accepted missing credential")
	}
}
