package config

import (
	"os"
	"strings"
	"testing"
)

// baseSecrets are the unrelated secrets Validate already demands. They are set
// so a rollout-stage test fails on the capability flags it is actually about,
// rather than on a token secret it never touched.
var baseSecrets = map[string]string{
	"HAULING_PROVIDER_TOKEN_SECRET":   "test-provider-token-secret",
	"HAULING_PROVIDER_REFRESH_SECRET": "test-provider-refresh-secret",
	"HAULING_PROVIDER_OTP_SECRET":     "test-provider-otp-secret",
	"HAULING_CUSTOMER_TOKEN_SECRET":   "test-customer-token-secret",
}

func withEnv(t *testing.T, values map[string]string) {
	t.Helper()
	for key, value := range baseSecrets {
		t.Setenv(key, value)
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}

// Every capability flag must default to off, so deploying this build changes
// no behaviour until someone deliberately turns it on.
func TestCapabilityFlagsDefaultOff(t *testing.T) {
	for _, key := range []string{
		"HAULING_CAPABILITY_MATCHING_ENABLED",
		"HAULING_FAIL_CLOSED_TRUCK_LOOKUP",
		"HAULING_SUPPLY_PREVIEW_ENABLED",
		"HAULING_CERTIFICATION_REQUIRED_PRODUCTS",
	} {
		os.Unsetenv(key)
	}
	cfg := Load()

	if cfg.CapabilityMatchingEnabled || cfg.FailClosedTruckLookup || cfg.SupplyPreviewEnabled {
		t.Fatalf("a capability flag defaults on: %+v", cfg)
	}
	if cfg.CertificationRequiredProducts != "" {
		t.Fatalf("certification is required by default for %q", cfg.CertificationRequiredProducts)
	}
	// The sweeper's defaults must be real durations, or a deployment that
	// never sets them would purge evidence immediately or never at all.
	if cfg.CapabilityMaintenanceInterval <= 0 || cfg.MatchDecisionRetention <= 0 {
		t.Fatalf("maintenance defaults are not usable: %v / %v",
			cfg.CapabilityMaintenanceInterval, cfg.MatchDecisionRetention)
	}
}

// Fail-closed without capability matching would be a flag that silently does
// nothing: an operator would believe unresolved trucks were being rejected
// while they were still matching. That is worse than a refusal at startup.
func TestFailClosedWithoutMatchingIsRefused(t *testing.T) {
	withEnv(t, map[string]string{
		"HAULING_CAPABILITY_MATCHING_ENABLED": "false",
		"HAULING_FAIL_CLOSED_TRUCK_LOOKUP":    "true",
	})
	err := Load().Validate()
	if err == nil {
		t.Fatal("a fail-closed flag with matching off was accepted")
	}
	if !strings.Contains(err.Error(), "HAULING_FAIL_CLOSED_TRUCK_LOOKUP") {
		t.Fatalf("error does not name the offending flag: %v", err)
	}
}

// A preview answers from the capability store. Without it the endpoint could
// only ever return UNKNOWN, which would teach Vendor's planner to stop asking.
func TestSupplyPreviewWithoutMatchingIsRefused(t *testing.T) {
	withEnv(t, map[string]string{
		"HAULING_CAPABILITY_MATCHING_ENABLED": "false",
		"HAULING_SUPPLY_PREVIEW_ENABLED":      "true",
	})
	err := Load().Validate()
	if err == nil {
		t.Fatal("a supply preview with matching off was accepted")
	}
	if !strings.Contains(err.Error(), "HAULING_SUPPLY_PREVIEW_ENABLED") {
		t.Fatalf("error does not name the offending flag: %v", err)
	}
}

// The full rollout sequence must be accepted at each stage.
func TestRolloutStagesAreAllValid(t *testing.T) {
	stages := []struct {
		name string
		env  map[string]string
	}{
		{"legacy", map[string]string{
			"HAULING_CAPABILITY_MATCHING_ENABLED": "false",
			"HAULING_FAIL_CLOSED_TRUCK_LOOKUP":    "false",
		}},
		{"shadow", map[string]string{
			"HAULING_CAPABILITY_MATCHING_ENABLED": "true",
			"HAULING_FAIL_CLOSED_TRUCK_LOOKUP":    "false",
		}},
		{"enforced", map[string]string{
			"HAULING_CAPABILITY_MATCHING_ENABLED": "true",
			"HAULING_FAIL_CLOSED_TRUCK_LOOKUP":    "true",
			"HAULING_SUPPLY_PREVIEW_ENABLED":      "true",
		}},
	}
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			withEnv(t, stage.env)
			if err := Load().Validate(); err != nil {
				t.Fatalf("%s stage rejected: %v", stage.name, err)
			}
		})
	}
}
