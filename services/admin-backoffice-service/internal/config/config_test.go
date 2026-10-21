package config

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigurationFailClosed(t *testing.T) {
	for _, name := range []string{"ADMIN_BACKOFFICE_DATABASE_URL", "ADMIN_TLS_CERT_FILE", "ADMIN_TLS_KEY_FILE", "ADMIN_TLS_CLIENT_CA_FILE", "ADMIN_TRUST_FILE", "ADMIN_REMOTE_ENABLED", "ADMIN_TLS_ADDR"} {
		t.Setenv(name, "")
	}
	if _, err := Load(); err == nil {
		t.Fatal("implicit development credentials accepted")
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	expiry := time.Now().Add(time.Hour).Unix()
	trust := Trust{Issuer: "hq-test", Audience: "karryam-test", WorkloadURI: "spiffe://test/karryam/hq", PolicyVersion: "test-v1", AssertionLifetimeSeconds: 60, SessionLifetimeSeconds: 300, AllowedACR: []string{"mfa"}, RequestsPerMinute: 20, StaffKeys: map[string]string{"one": base64.RawURLEncoding.EncodeToString(pub)}, ServiceKeys: map[string]string{"one": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{45}, 32))}, StaffKeyExpires: map[string]int64{"one": expiry}, ServiceKeyExpires: map[string]int64{"one": expiry}}
	data, _ := json.Marshal(trust)
	path := filepath.Join(t.TempDir(), "trust.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"ADMIN_BACKOFFICE_DATABASE_URL": "postgres://fixture.invalid/admin_test", "ADMIN_TLS_CERT_FILE": "fixture.crt", "ADMIN_TLS_KEY_FILE": "fixture.key", "ADMIN_TLS_CLIENT_CA_FILE": "fixture-ca.pem", "ADMIN_TRUST_FILE": path} {
		t.Setenv(name, value)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RemoteEnabled {
		t.Fatal("remote enabled by default")
	}
	if _, err = cfg.TLS(); err == nil {
		t.Fatal("invalid TLS files accepted")
	}
	trust.StaffKeyExpires["one"] = time.Now().Add(-time.Second).Unix()
	if trust.Validate() == nil {
		t.Fatal("expired signing key accepted")
	}
}
