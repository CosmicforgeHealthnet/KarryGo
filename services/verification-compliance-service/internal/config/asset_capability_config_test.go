package config

import (
	"strings"
	"testing"
)

// baseValid is a configuration that passes everything unrelated to Section 12,
// so each test below fails for exactly the reason it is about.
func baseValid() Config {
	return Config{
		DatabaseURL: "postgres://localhost/verification",
	}
}

// TestCertificationRequiresSomewhereToDeliverTo is the impossible-combination
// check that matters most.
//
// A certification feature with no reachable authority is not a degraded
// feature, it is a silent one: decisions are recorded here and the services
// that actually dispatch work never hear them. The asset ends up suspended in
// one place and eligible in the other, and no read on either side would ever
// disagree loudly enough to notice.
func TestCertificationRequiresSomewhereToDeliverTo(t *testing.T) {
	cfg := baseValid()
	cfg.AssetCapabilityCertificationEnabled = true
	cfg.MediaBaseURL = "https://media.internal"
	cfg.MediaToken = "media-token"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("a certification feature with no authority callback was accepted")
	}
	if !strings.Contains(err.Error(), "at least one authority callback") {
		t.Fatalf("error = %v, want it to name the missing callback", err)
	}

	cfg.TaxiCallbackURL = "https://taxi.internal"
	cfg.TaxiCallbackSecret = "taxi-secret"
	if err = cfg.Validate(); err != nil {
		t.Fatalf("one configured authority should be enough: %v", err)
	}
}

// TestCertificationRequiresMedia guards the evidence path.
//
// Without Media no document can be registered at all, because registering one
// binds its media object. The feature would look enabled while its entire
// evidence half was dead.
func TestCertificationRequiresMedia(t *testing.T) {
	cfg := baseValid()
	cfg.AssetCapabilityCertificationEnabled = true
	cfg.TaxiCallbackURL = "https://taxi.internal"
	cfg.TaxiCallbackSecret = "taxi-secret"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "MEDIA_BASE_URL") {
		t.Fatalf("error = %v, want a refusal naming the missing Media configuration", err)
	}
}

// TestCallbackURLAndSecretAreRequiredTogether refuses a half-configured
// authority, which would otherwise fail only at the first delivery.
func TestCallbackURLAndSecretAreRequiredTogether(t *testing.T) {
	cfg := baseValid()
	cfg.HaulingCallbackURL = "https://hauling.internal"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "HAULING_CALLBACK_SECRET") {
		t.Fatalf("error = %v, want a refusal naming the missing secret", err)
	}

	cfg.HaulingCallbackSecret = "hauling-secret"
	if err = cfg.Validate(); err != nil {
		t.Fatalf("a complete pair should validate: %v", err)
	}
}

// TestCallbackURLMustBeAnOrigin catches a path pasted into a base URL, which
// would produce a doubled path at delivery time and a 404 nobody reads.
func TestCallbackURLMustBeAnOrigin(t *testing.T) {
	cfg := baseValid()
	cfg.DispatchCallbackURL = "not-a-url"
	cfg.DispatchCallbackSecret = "dispatch-secret"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DISPATCH_CALLBACK_URL") {
		t.Fatalf("error = %v, want a refusal naming the malformed URL", err)
	}
}

// TestDisabledCertificationNeedsNoIntegration keeps the default deployment
// runnable: the flag is off by default and must not demand credentials for a
// feature nobody has turned on.
func TestDisabledCertificationNeedsNoIntegration(t *testing.T) {
	cfg := baseValid()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the default configuration must validate: %v", err)
	}
	if cfg.AssetCapabilityCertificationEnabled {
		t.Fatal("asset capability certification must default to off")
	}
}
