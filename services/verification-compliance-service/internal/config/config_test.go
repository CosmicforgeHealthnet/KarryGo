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
