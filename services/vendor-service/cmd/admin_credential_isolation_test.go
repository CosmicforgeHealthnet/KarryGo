package main

import (
	"bytes"
	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	"testing"
)

func TestVendorAdminCredentialsNeverReuseStandingOrApplicationAudiences(t *testing.T) {
	key := bytes.Repeat([]byte("k"), 32)
	if err := vendorAdminCredentialIsolation(config.Config{}, &adminowner.Handler{}, &admincommand.Handler{}); err != nil {
		t.Fatal("disabled channels", err)
	}
	for _, cfg := range []config.Config{{CustomerStandingSecret: string(key)}, {ServiceSecrets: "customer-service=" + string(key)}, {WalletSecret: string(key)}, {Auth: config.AuthConfig{AccessTokenSecret: key}}} {
		if err := vendorAdminCredentialIsolation(cfg, &adminowner.Handler{Secret: key}, nil); err == nil {
			t.Fatal("application audience key accepted")
		}
	}
	if err := vendorAdminCredentialIsolation(config.Config{}, &adminowner.Handler{Secret: key, PublishSecret: key}, nil); err == nil {
		t.Fatal("publication shared read audience")
	}
	if err := vendorAdminCredentialIsolation(config.Config{}, &adminowner.Handler{Secret: key}, &admincommand.Handler{ReadSecret: key}); err == nil {
		t.Fatal("commands shared resource audience")
	}
	if err := vendorAdminCredentialIsolation(config.Config{}, &adminowner.Handler{Secret: key}, &admincommand.Handler{ReadSecret: bytes.Repeat([]byte("r"), 32), WriteSecret: bytes.Repeat([]byte("w"), 32)}); err != nil {
		t.Fatal("separate test keys denied", err)
	}
}
