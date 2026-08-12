package app

import (
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

func TestBuildComponentsWiresRequiredProductionContracts(t *testing.T) {
	cfg := config.Config{
		HTTPClientTimeout:         time.Second,
		SessionSecret:             []byte("unit-session"),
		OTPSecret:                 []byte("unit-otp"),
		ContactSecret:             []byte("unit-contact"),
		AccessTokenSecret:         []byte("unit-access"),
		HaulingIntegrationEnabled: true,
		Clients: map[string]config.ClientConfig{
			"notification-service":            {BaseURL: "http://notification.invalid", Secret: []byte("notification")},
			"payment-wallet-service":          {BaseURL: "http://wallet.invalid", Secret: []byte("wallet")},
			"media-file-service":              {BaseURL: "http://media.invalid", Secret: []byte("media")},
			"verification-compliance-service": {BaseURL: "http://verification.invalid", Secret: []byte("verification")},
			"driver-hauling-service":          {BaseURL: "http://hauling.invalid", Secret: []byte("hauling")},
		},
	}
	components := BuildComponents(cfg, nil, nil, observability.NewMetrics())
	if !components.Contracts.Wallet.Configured() || !components.Contracts.Media.Configured() || !components.Contracts.Verification.Configured() {
		t.Fatal("Wallet, Media and Verification must be real configured clients")
	}
	if !components.Contracts.Providers[platformclients.CategoryHauling].Configured() || !components.Contracts.HaulingLinks.Configured() {
		t.Fatal("Hauling provider and link authorities must be real configured clients")
	}
	if components.Contracts.Providers[platformclients.CategoryTaxi].Configured() {
		t.Fatal("Taxi must remain fail closed")
	}
}

func TestGlobalKillSwitchLeavesMutationContractsUnconfigured(t *testing.T) {
	cfg := config.Config{IntegrationsKillSwitch: true, Clients: map[string]config.ClientConfig{
		"payment-wallet-service":          {BaseURL: "http://wallet.invalid", Secret: []byte("wallet")},
		"media-file-service":              {BaseURL: "http://media.invalid", Secret: []byte("media")},
		"verification-compliance-service": {BaseURL: "http://verification.invalid", Secret: []byte("verification")},
		"driver-hauling-service":          {BaseURL: "http://hauling.invalid", Secret: []byte("hauling")},
	}}
	components := BuildComponents(cfg, nil, nil, observability.NewMetrics())
	if components.Contracts.Wallet.Configured() || components.Contracts.Media.Configured() || components.Contracts.Verification.Configured() || components.Contracts.HaulingLinks.Configured() {
		t.Fatal("global integration kill switch must prevent mutation client wiring")
	}
}
