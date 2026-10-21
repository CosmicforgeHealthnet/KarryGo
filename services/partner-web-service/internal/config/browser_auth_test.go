package config

import "testing"

func TestPartnerProductionBrowserPolicy(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("API_GATEWAY_DATABASE_URL", "configured-test-only-not-connected")
	t.Setenv("PARTNER_JWT_SECRET", "test-only-secret-at-least-thirty-two-bytes")
	t.Setenv("PARTNER_AUTH_WEB_ORIGINS", "https://partner.example.test")
	cfg := Load()
	if !cfg.PartnerAuth.BrowserPolicy.Secure || cfg.PartnerAuth.BrowserPolicy.Path != "/api/v1/partner/v1/auth" {
		t.Fatal("unsafe production policy")
	}
	t.Setenv("PARTNER_AUTH_WEB_ORIGINS", "")
	defer func() {
		if recover() == nil {
			t.Fatal("missing production origins must fail startup")
		}
	}()
	_ = Load()
}

func TestDisabledPartnerDoesNotRequireBrowserOrigin(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("API_GATEWAY_DATABASE_URL", "")
	t.Setenv("PARTNER_AUTH_WEB_ORIGINS", "")
	_ = Load()
}
