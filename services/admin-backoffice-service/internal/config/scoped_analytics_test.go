package config

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestScopedAnalyticsConfigurationNeedsBothFlagsAndDistinctCredential(t *testing.T) {
	t.Setenv("ADMIN_ANALYTICS_SCOPED_READ_ENABLED", "false")
	t.Setenv("ADMIN_ANALYTICS_SCOPED_SECRET_FILE", "missing")
	c := Config{Features: DomainFlags{AnalyticsReads: true}}
	if client, err := c.ScopedAnalyticsClient(); client != nil || err != nil {
		t.Fatal("off loaded key", err)
	}
	t.Setenv("ADMIN_ANALYTICS_SCOPED_READ_ENABLED", "true")
	c.Features.AnalyticsReads = false
	if client, err := c.ScopedAnalyticsClient(); client != nil || err != nil {
		t.Fatal("global read kill loaded key", err)
	}
	c.Features.AnalyticsReads = true
	key := bytes.Repeat([]byte{91}, 32)
	path := filepath.Join(t.TempDir(), "source.key")
	if err := os.WriteFile(path, []byte(base64.RawURLEncoding.EncodeToString(key)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADMIN_ANALYTICS_SCOPED_SECRET_FILE", path)
	t.Setenv("ADMIN_ANALYTICS_SCOPED_ORIGIN", "http://127.0.0.1:12345")
	client, err := c.ScopedAnalyticsClient()
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	c.Trust.ServiceKeys = map[string]string{"hq": base64.RawURLEncoding.EncodeToString(key)}
	if _, err := c.ScopedAnalyticsClient(); err == nil {
		t.Fatal("HQ credential reused for Analytics")
	}
}
