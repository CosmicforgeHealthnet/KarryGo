package adminfeed

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestDistinctTransportKeysRequired(t *testing.T) {
	for _, key := range []string{"NOTIFICATION_ADMIN_EVENTS_SECRET_FILE", "NOTIFICATION_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE", "NOTIFICATION_CAMPAIGN_SECRET_FILE", "NOTIFICATION_ADMIN_EVENTS_ENABLED", "NOTIFICATION_CAMPAIGNS_ENABLED"} {
		t.Setenv(key, "")
	}
	t.Setenv("NOTIFICATION_ADMIN_EVENTS_SNAPSHOT_ENABLED", "true")
	file := func(name string, key byte) string {
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{key}, 32))), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	publisher := file("publisher", 1)
	snapshot := file("snapshot", 2)
	t.Setenv("NOTIFICATION_ADMIN_CONTEXT_PUBLISH_SECRET_FILE", publisher)
	t.Setenv("NOTIFICATION_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE", snapshot)
	if err := checkTransportSecrets(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOTIFICATION_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE", publisher)
	if err := checkTransportSecrets(); err == nil {
		t.Fatal("publisher key reused for snapshot")
	}
	t.Setenv("NOTIFICATION_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE", snapshot)
	t.Setenv("NOTIFICATION_CAMPAIGN_SECRET_FILE", snapshot)
	if err := checkTransportSecrets(); err == nil {
		t.Fatal("sending key reused for snapshot")
	}
}
