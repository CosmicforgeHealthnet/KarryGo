package config

import (
	"strings"
	"testing"
)

func baseline() Config {
	return Config{ProviderTokenSecret: []byte("a"), ProviderRefreshSecret: []byte("b"), ProviderOTPSecret: []byte("c"), CustomerTokenSecret: []byte("d")}
}

func TestNoSecretFallbacks(t *testing.T) {
	t.Setenv("HAULING_PROVIDER_TOKEN_SECRET", "")
	t.Setenv("HAULING_PROVIDER_REFRESH_SECRET", "")
	t.Setenv("HAULING_PROVIDER_OTP_SECRET", "")
	t.Setenv("HAULING_CUSTOMER_TOKEN_SECRET", "")
	cfg := Load()
	if err := cfg.Validate(); err == nil {
		t.Fatal("empty secrets accepted")
	}
}
func TestFleetEnablementRequiresCompleteProductionConfig(t *testing.T) {
	cfg := baseline()
	cfg.FleetIntegrationEnabled = true
	err := cfg.Validate()
	if err == nil {
		t.Fatal("partial fleet config accepted")
	}
	for _, want := range []string{"HAULING_SERVICE_SECRETS", "HAULING_OUTBOX_WORKER_ENABLED", "HAULING_FLEET_EVENT_URL", "HAULING_FLEET_EVENT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %s", err, want)
		}
	}
}
func TestFleetKillSwitchOverridesIntegrationRequirements(t *testing.T) {
	cfg := baseline()
	cfg.FleetIntegrationEnabled = true
	cfg.FleetIntegrationKillSwitch = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("kill switch did not override integration: %v", err)
	}
}

func TestCommerceIntegrationDefaultsDisabled(t *testing.T) {
	t.Setenv("HAULING_COMMERCE_INTEGRATION_ENABLED", "")
	if Load().CommerceIntegrationEnabled {
		t.Fatal("Hauling Commerce must be explicitly enabled")
	}
}
