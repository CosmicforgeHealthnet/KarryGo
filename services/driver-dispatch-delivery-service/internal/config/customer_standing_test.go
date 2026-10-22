package config

import (
	"strings"
	"testing"
)

func setRequiredDispatchEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DISPATCH_RIDER_ACCESS_TOKEN_SECRET", "env-access-secret")
	t.Setenv("DISPATCH_RIDER_REFRESH_TOKEN_SECRET", "env-refresh-secret")
	t.Setenv("DISPATCH_RIDER_OTP_SECRET", "env-otp-secret")
	t.Setenv("WALLET_SERVICE_SECRET", "env-wallet-secret")
	t.Setenv("WALLET_SERVICE_URL", "http://wallet:8105/api/v1/payment-wallet")
	t.Setenv("WALLET_SERVICE_SOURCE", "dispatch-delivery-service")
}

func TestCustomerStandingEnforcementDefaultsOff(t *testing.T) {
	setRequiredDispatchEnv(t)
	t.Setenv("DISPATCH_CUSTOMER_STANDING_ENFORCED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CustomerStandingEnforced {
		t.Fatal("DISPATCH_CUSTOMER_STANDING_ENFORCED must default to false")
	}
}

func TestCustomerStandingEnforcedRequiresCustomerService(t *testing.T) {
	cases := []struct {
		name, url, secret, want string
	}{
		{"missing url", "", "s", "DISPATCH_CUSTOMER_SERVICE_URL"},
		{"missing secret", "http://localhost:8101", "", "DISPATCH_CUSTOMER_SERVICE_SECRET"},
		{"path in url", "http://localhost:8101/api/v1/customer", "s", "bare origin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRequiredDispatchEnv(t)
			t.Setenv("DISPATCH_CUSTOMER_STANDING_ENFORCED", "true")
			t.Setenv("DISPATCH_CUSTOMER_SERVICE_URL", tc.url)
			t.Setenv("DISPATCH_CUSTOMER_SERVICE_SECRET", tc.secret)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load() error = %v, want mention of %s", err, tc.want)
			}
		})
	}
	setRequiredDispatchEnv(t)
	t.Setenv("DISPATCH_CUSTOMER_STANDING_ENFORCED", "true")
	t.Setenv("DISPATCH_CUSTOMER_SERVICE_URL", "http://localhost:8101")
	t.Setenv("DISPATCH_CUSTOMER_SERVICE_SECRET", "development-dispatch-customer-service-secret")
	cfg, err := Load()
	if err != nil || !cfg.CustomerStandingEnforced {
		t.Fatalf("complete standing config refused: %v", err)
	}
}
