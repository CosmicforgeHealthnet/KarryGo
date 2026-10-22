package config

import (
	"strings"
	"testing"
)

func TestCustomerStandingEnforcedRequiresCustomerService(t *testing.T) {
	cfg := baseline()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	cfg.CustomerStandingEnforced = true
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "HAULING_CUSTOMER_STANDING_ENFORCED") {
		t.Fatalf("enforced without URL/secret accepted: %v", err)
	}
	cfg.CustomerServiceURL = "http://localhost:8101/api/v1/customer"
	cfg.CustomerServiceSecret = []byte("development-hauling-customer-service-secret")
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "bare origin") {
		t.Fatalf("non-bare origin accepted: %v", err)
	}
	cfg.CustomerServiceURL = "http://localhost:8101"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("complete standing config refused: %v", err)
	}
}

func TestCustomerStandingEnforcementDefaultsOff(t *testing.T) {
	t.Setenv("HAULING_CUSTOMER_STANDING_ENFORCED", "")
	if Load().CustomerStandingEnforced {
		t.Fatal("HAULING_CUSTOMER_STANDING_ENFORCED must default to false")
	}
	t.Setenv("HAULING_CUSTOMER_STANDING_ENFORCED", "true")
	if !Load().CustomerStandingEnforced {
		t.Fatal("HAULING_CUSTOMER_STANDING_ENFORCED=true not read")
	}
}
