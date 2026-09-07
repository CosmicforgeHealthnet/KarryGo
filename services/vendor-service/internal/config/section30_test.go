package config

import "testing"

func TestSection30SecureHaulingDependencyIsReady(t *testing.T) {
	cfg := withAuthSecrets(Load())
	cfg.Flags.HaulingCommerceIntegrationEnabled = true
	cfg.HaulingURL = "http://driver-hauling-service"
	cfg.HaulingSecret = "test-secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("secure Hauling dependency should be ready: %v", err)
	}
}

func TestSection30HandoverGateDefaultsRequired(t *testing.T) {
	if !Load().Fulfilment.HaulingSecureHandoverRequired {
		t.Fatal("HAULING_SECURE_HANDOVER_REQUIRED must default true")
	}
}
