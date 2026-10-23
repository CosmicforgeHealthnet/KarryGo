package config

import (
	"strings"
	"testing"
)

func TestVendorCustomerStandingDefaultsAndDedicatedEnrollment(t *testing.T) {
	t.Setenv("VENDOR_CUSTOMER_STANDING_ENFORCED", "")
	cfg := withAuthSecrets(Load())
	if cfg.CustomerStandingEnforced {
		t.Fatal("standing enforcement defaults on")
	}
	cfg.CustomerStandingEnforced = true
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "STANDING_ENFORCED") {
		t.Fatal("missing standing enrollment accepted", err)
	}
	cfg.CustomerStandingURL = "http://127.0.0.1:1234"
	cfg.CustomerStandingSecret = "test-only-dedicated-standing-secret"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.CustomerSecret = cfg.CustomerStandingSecret
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "dedicated") {
		t.Fatal("reused key allowed", err)
	}
}
