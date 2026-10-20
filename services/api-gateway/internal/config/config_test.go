package config

import "testing"

func TestPartnerOriginIsMandatory(t *testing.T) {
	t.Setenv("GATEWAY_UPSTREAM_PARTNER_WEB_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing partner origin must fail closed")
	}
}
func TestNoPartnerDatabaseRequired(t *testing.T) {
	t.Setenv("GATEWAY_UPSTREAM_PARTNER_WEB_URL", "http://partner-web-service:8080")
	t.Setenv("API_GATEWAY_DATABASE_URL", "")
	t.Setenv("API_GATEWAY_REDIS_ADDR", "")
	if c, err := Load(); err != nil || c.Origins["partner_web"] == "" {
		t.Fatalf("gateway should not own partner state: %#v %v", c, err)
	}
}
