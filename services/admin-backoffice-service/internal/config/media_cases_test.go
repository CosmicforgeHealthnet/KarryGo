package config

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestMediaCaseConfigurationOffAndAudienceSeparation(t *testing.T) {
	t.Setenv("ADMIN_MEDIA_CASE_READ_ENABLED", "false")
	t.Setenv("ADMIN_MEDIA_CASE_REVOKE_ENABLED", "false")
	t.Setenv("ADMIN_MEDIA_CASE_SECRET_FILE", "must-not-be-read")
	if client, read, revoke, err := (Config{}).MediaCaseClient(); err != nil || client != nil || read || revoke {
		t.Fatal("disabled Media required credentials", err)
	}
	dir := t.TempDir()
	key := bytes.Repeat([]byte("m"), 32)
	other := bytes.Repeat([]byte("o"), 32)
	file := func(name string, key []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(base64.RawURLEncoding.EncodeToString(key)), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := file("case.key", key)
	resource := file("resource.key", other)
	t.Setenv("ADMIN_MEDIA_CASE_SECRET_FILE", path)
	t.Setenv("ADMIN_MEDIA_CASE_ORIGIN", "http://127.0.0.1:12345")
	t.Setenv("ADMIN_MEDIA_CASE_REVOKE_ENABLED", "true")
	cfg := Config{OwnerResources: []OwnerResources{{Owner: "customer-service", SecretFile: resource}}}
	client, read, revoke, err := cfg.MediaCaseClient()
	if err != nil || client == nil || read || !revoke {
		t.Fatal("revocation-only client failed", err)
	}
	client.Close()
	cfg.Trust.ServiceKeys = map[string]string{"hq": base64.RawURLEncoding.EncodeToString(key)}
	if _, _, _, err := cfg.MediaCaseClient(); err == nil {
		t.Fatal("HQ key reused")
	}
	cfg.Trust.ServiceKeys = nil
	cfg.OwnerResources[0].SecretFile = path
	if _, _, _, err := cfg.MediaCaseClient(); err == nil {
		t.Fatal("resource key reused")
	}
	cfg.OwnerResources[0].SecretFile = resource
	t.Setenv("ADMIN_ANALYTICS_SCOPED_READ_ENABLED", "true")
	t.Setenv("ADMIN_ANALYTICS_SCOPED_SECRET_FILE", path)
	cfg.Features.AnalyticsReads = true
	if _, _, _, err := cfg.MediaCaseClient(); err == nil {
		t.Fatal("scoped Analytics key reused")
	}
	cfg.Features.AnalyticsReads = false
	t.Setenv("ADMIN_CUSTOMER_REVIEW_READBACK_ENABLED", "true")
	t.Setenv("ADMIN_CUSTOMER_REVIEW_READBACK_SECRET_FILE", path)
	if _, _, _, err := cfg.MediaCaseClient(); err == nil {
		t.Fatal("Customer readback key reused")
	}
	t.Setenv("ADMIN_CUSTOMER_REVIEW_READBACK_ENABLED", "false")
	t.Setenv("ADMIN_MEDIA_CASE_ORIGIN", "https://test.invalid/arbitrary")
	if _, _, _, err := cfg.MediaCaseClient(); err == nil {
		t.Fatal("arbitrary Media proxy origin accepted")
	}
}
