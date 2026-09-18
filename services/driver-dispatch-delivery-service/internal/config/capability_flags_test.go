package config

import (
	"os"
	"testing"
)

// Every capability flag must default to off, so deploying this build changes
// no behaviour until someone deliberately turns it on.
func TestCapabilityFlagsDefaultOff(t *testing.T) {
	for _, key := range []string{
		"DISPATCH_CAPABILITY_MATCHING_ENABLED",
		"DISPATCH_CAPABILITY_MATCHING_ENFORCE",
		"DISPATCH_SUPPLY_PREVIEW_ENABLED",
		"DISPATCH_CERTIFICATION_REQUIRED_PRODUCTS",
	} {
		os.Unsetenv(key)
	}
	cfg := mustLoad(t)

	if cfg.CapabilityMatchingEnabled || cfg.CapabilityMatchingEnforce || cfg.SupplyPreviewEnabled {
		t.Fatal("a capability flag defaults on")
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

// The three rollout stages must each load as intended, and the pair must be
// readable as a stage rather than as two unrelated switches.
func TestRolloutStagesLoadAsIntended(t *testing.T) {
	cases := []struct {
		name            string
		enabled         string
		enforce         string
		wantEnabled     bool
		wantEnforce     bool
		describesShadow bool
	}{
		{"legacy", "false", "false", false, false, false},
		{"shadow", "true", "false", true, false, true},
		{"enforced", "true", "true", true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DISPATCH_CAPABILITY_MATCHING_ENABLED", tc.enabled)
			t.Setenv("DISPATCH_CAPABILITY_MATCHING_ENFORCE", tc.enforce)
			cfg := mustLoad(t)

			if cfg.CapabilityMatchingEnabled != tc.wantEnabled ||
				cfg.CapabilityMatchingEnforce != tc.wantEnforce {
				t.Fatalf("%s loaded as enabled=%v enforce=%v",
					tc.name, cfg.CapabilityMatchingEnabled, cfg.CapabilityMatchingEnforce)
			}
			shadow := cfg.CapabilityMatchingEnabled && !cfg.CapabilityMatchingEnforce
			if shadow != tc.describesShadow {
				t.Fatalf("%s: shadow = %v, want %v", tc.name, shadow, tc.describesShadow)
			}
		})
	}
}

// Enforce without matching is a flag that silently does nothing. Dispatch has
// no config Validate to refuse it, so the router logs a notice instead; this
// pins that the combination is at least representable and inert rather than
// quietly enforcing.
func TestEnforceWithoutMatchingLoadsButCannotEnforce(t *testing.T) {
	t.Setenv("DISPATCH_CAPABILITY_MATCHING_ENABLED", "false")
	t.Setenv("DISPATCH_CAPABILITY_MATCHING_ENFORCE", "true")
	cfg := mustLoad(t)

	if cfg.CapabilityMatchingEnabled {
		t.Fatal("matching turned itself on")
	}
	if !cfg.CapabilityMatchingEnforce {
		t.Fatal("the enforce flag was silently dropped rather than left inert")
	}
}

// mustLoad fails the test rather than masking a config error, so a broken
// environment is reported as such instead of surfacing as a wrong flag value.
func mustLoad(t *testing.T) Config {
	t.Helper()
	// These secrets are required by Load and unrelated to the flags under
	// test; setting them here keeps a failure about capability flags from
	// being reported as a missing token.
	for _, key := range []string{
		"DISPATCH_RIDER_ACCESS_TOKEN_SECRET",
		"DISPATCH_RIDER_REFRESH_TOKEN_SECRET",
		"DISPATCH_RIDER_OTP_SECRET",
		"WALLET_SERVICE_SECRET",
	} {
		if os.Getenv(key) == "" {
			t.Setenv(key, "test-"+key)
		}
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}
