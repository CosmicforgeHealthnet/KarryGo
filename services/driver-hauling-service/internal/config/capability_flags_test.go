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

// The gap-closure flags default off too, so deploying this build changes
// nothing until someone deliberately turns it on.
func TestGapClosureFlagsDefaultOff(t *testing.T) {
	for _, key := range []string{
		"HAULING_RECOVERY_ENABLED", "HAULING_RECOVERY_EVENTS_ENABLED",
		"HAULING_POLICY_INGEST_ENABLED", "HAULING_ADMIN_ALLOWED_HOSTS",
	} {
		os.Unsetenv(key)
	}
	cfg := Load()
	if cfg.RecoveryEnabled || cfg.RecoveryEventsEnabled || cfg.PolicyIngestEnabled {
		t.Fatalf("a gap-closure flag defaults on: %+v", cfg)
	}
	// An empty host allowlist is the safe default: it disables read-back
	// entirely rather than allowing any host a publication happens to name.
	if cfg.AdminAllowedHosts != "" {
		t.Fatalf("admin hosts default to %q", cfg.AdminAllowedHosts)
	}
	if cfg.PolicyReconcileInterval <= 0 {
		t.Fatalf("reconcile interval is not usable: %v", cfg.PolicyReconcileInterval)
	}
}

// Recovery selects a replacement through the capability store. Without
// capability matching it would fall back to proximity alone -- the exact
// fail-open Section 9 removed from the original matcher.
func TestRecoveryWithoutCapabilityMatchingIsRefused(t *testing.T) {
	withEnv(t, map[string]string{
		"HAULING_CAPABILITY_MATCHING_ENABLED": "false",
		"HAULING_RECOVERY_ENABLED":            "true",
	})
	err := Load().Validate()
	if err == nil {
		t.Fatal("recovery without capability matching was accepted")
	}
	if !strings.Contains(err.Error(), "HAULING_RECOVERY_ENABLED") {
		t.Fatalf("error does not name the offending flag: %v", err)
	}
}

// An events flag with no recovery behind it silently does nothing.
func TestRecoveryEventsWithoutRecoveryIsRefused(t *testing.T) {
	withEnv(t, map[string]string{
		"HAULING_CAPABILITY_MATCHING_ENABLED": "true",
		"HAULING_RECOVERY_ENABLED":            "false",
		"HAULING_RECOVERY_EVENTS_ENABLED":     "true",
	})
	err := Load().Validate()
	if err == nil {
		t.Fatal("recovery events without recovery were accepted")
	}
	if !strings.Contains(err.Error(), "HAULING_RECOVERY_EVENTS_ENABLED") {
		t.Fatalf("error does not name the offending flag: %v", err)
	}
}

// The full end state -- matching enforced, recovery on, events on, ingestion
// on -- must load and validate.
func TestFullyEnabledDeploymentIsValid(t *testing.T) {
	withEnv(t, map[string]string{
		"HAULING_CAPABILITY_MATCHING_ENABLED": "true",
		"HAULING_FAIL_CLOSED_TRUCK_LOOKUP":    "true",
		"HAULING_SUPPLY_PREVIEW_ENABLED":      "true",
		"HAULING_RECOVERY_ENABLED":            "true",
		"HAULING_RECOVERY_EVENTS_ENABLED":     "true",
		"HAULING_POLICY_INGEST_ENABLED":       "true",
	})
	if err := Load().Validate(); err != nil {
		t.Fatalf("the fully enabled deployment was rejected: %v", err)
	}
}
