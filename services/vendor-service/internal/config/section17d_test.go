package config

import (
	"strings"
	"testing"
	"time"
)

func section17DBaseConfig() Config {
	cfg := section17BBaseConfig()
	cfg.Flags.VendorOwnDeliveryEnabled = true
	cfg.Flags.CommerceFulfilmentPlanningEnabled = true
	cfg.Flags.CommerceQuoteLockEnabled = true
	cfg.Fulfilment = FulfilmentConfig{
		QuoteTimeout:              8 * time.Second,
		PlanTTL:                   15 * time.Minute,
		ConsolidationRadiusMeters: 150,
		ProximityRadiusMeters:     3000,
		ExpiryInterval:            time.Minute,
	}
	cfg.Quote = QuoteConfig{TTL: 5 * time.Minute}
	return cfg
}

func TestLoad_Section17DDefaultsSafe(t *testing.T) {
	cfg := Load()
	if cfg.Flags.CommerceQuoteLockEnabled {
		t.Fatal("COMMERCE_QUOTE_LOCK_ENABLED must default off")
	}
	if cfg.Quote.TTL != 5*time.Minute {
		t.Fatalf("quote TTL default = %v", cfg.Quote.TTL)
	}
}

func TestValidate_Section17DBaselineIsValid(t *testing.T) {
	if err := section17DBaseConfig().Validate(); err != nil {
		t.Fatalf("coherent Section 17D config must validate: %v", err)
	}
}

func TestValidate_Section17DRequiresCheckoutAndFulfilment(t *testing.T) {
	for name, disable := range map[string]func(*Config){
		"checkout":   func(c *Config) { c.Flags.CommerceCheckoutEnabled = false },
		"fulfilment": func(c *Config) { c.Flags.CommerceFulfilmentPlanningEnabled = false },
	} {
		cfg := section17DBaseConfig()
		disable(&cfg)
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "COMMERCE_") {
			t.Fatalf("%s dependency accepted: %v", name, err)
		}
	}
}

func TestValidate_Section17DRefusesUnsafeTTL(t *testing.T) {
	for name, ttl := range map[string]time.Duration{
		"unset": 0, "short": 59 * time.Second, "long": 25 * time.Hour,
	} {
		cfg := section17DBaseConfig()
		cfg.Quote.TTL = ttl
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "COMMERCE_QUOTE_TTL_SECONDS") {
			t.Fatalf("%s TTL accepted: %v", name, err)
		}
	}
}

func TestValidate_Section17DRequiresVendorServiceMasterSwitch(t *testing.T) {
	cfg := section17DBaseConfig()
	cfg.Flags.VendorServiceEnabled = false
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "VENDOR_SERVICE_ENABLED") {
		t.Fatalf("quote lock accepted while service disabled: %v", err)
	}
}
