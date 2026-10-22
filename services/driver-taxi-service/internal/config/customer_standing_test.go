package config

import (
	"strings"
	"testing"
)

func TestValidateCustomerStandingEnforcedRequiresCustomerService(t *testing.T) {
	cfg := validCfg()
	if cfg.CustomerStandingEnforced {
		t.Fatal("enforcement must default off")
	}
	cfg.CustomerStandingEnforced = true
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "TAXI_CUSTOMER_STANDING_ENFORCED") {
		t.Fatalf("enforced without URL/secret accepted: %v", err)
	}
	cfg.CustomerServiceURL = "http://localhost:8101"
	if err := cfg.Validate(); err == nil {
		t.Fatal("enforced without secret accepted")
	}
	cfg.CustomerServiceSecret = []byte("development-taxi-customer-service-secret")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("complete standing config refused: %v", err)
	}
	cfg.CustomerServiceURL = "http://localhost:8101/api/v1/customer"
	if err := cfg.Validate(); err == nil {
		t.Fatal("non-bare customer-service origin accepted")
	}
}

func TestLoadCustomerStandingDefaultsOff(t *testing.T) {
	t.Setenv("TAXI_CUSTOMER_STANDING_ENFORCED", "")
	if Load().CustomerStandingEnforced {
		t.Fatal("TAXI_CUSTOMER_STANDING_ENFORCED must default to false")
	}
	t.Setenv("TAXI_CUSTOMER_STANDING_ENFORCED", "true")
	if !Load().CustomerStandingEnforced {
		t.Fatal("TAXI_CUSTOMER_STANDING_ENFORCED=true not read")
	}
}
