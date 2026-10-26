package config

import "testing"

func validConfig() Config {
	return Config{DatabaseURL: "postgres://test", PrivateAPIEnabled: true, ServiceSecrets: "vendor-service=secret", AllowedCallers: "vendor-service"}
}

func TestVendorCallbackConfigurationIsPairedAndAbsolute(t *testing.T) {
	cfg := validConfig()
	cfg.VendorCallbackURL = "http://vendor-service:8112"
	if err := cfg.Validate(); err == nil {
		t.Fatal("callback URL without secret accepted")
	}
	cfg.VendorCallbackSecret = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid callback config rejected: %v", err)
	}
	cfg.VendorCallbackURL = "/relative"
	if err := cfg.Validate(); err == nil {
		t.Fatal("relative callback URL accepted")
	}
}

func TestSection18FlagsDefaultOffAndExternalAuthorityRefused(t *testing.T) {
	for _, key := range []string{"VERIFICATION_COMPLIANCE_AUTOMATION_ENABLED", "VERIFICATION_COMPLIANCE_CASE_EXPIRY_ENABLED", "VERIFICATION_COMPLIANCE_EXTERNAL_AUTHORITY"} {
		t.Setenv(key, "")
	}
	cfg := Load()
	if cfg.AutomationEnabled || cfg.CaseExpiryEnabled || cfg.ExternalAuthority != "" {
		t.Fatalf("section 18 flags must default off: %+v", cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.ExternalAuthority = "disabled"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled external authority refused: %v", err)
	}
	for _, provider := range []string{"smile-identity", "nimc", "true"} {
		cfg.ExternalAuthority = provider
		if err := cfg.Validate(); err == nil {
			t.Fatalf("uncontracted external authority %q accepted", provider)
		}
	}
}
