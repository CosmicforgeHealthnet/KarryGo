package adminfeed

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"strings"
)

// Feed credentials are distinct from source publication and sending keys.
// Only enabled/configured transport files are read; no secret is logged.
func checkTransportSecrets() error {
	var secrets [][]byte
	for _, item := range []struct {
		path     string
		required bool
	}{
		{os.Getenv("NOTIFICATION_ADMIN_CONTEXT_PUBLISH_SECRET_FILE"), true},
		{os.Getenv("NOTIFICATION_ADMIN_EVENTS_SECRET_FILE"), os.Getenv("NOTIFICATION_ADMIN_EVENTS_ENABLED") == "true"},
		{os.Getenv("NOTIFICATION_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE"), os.Getenv("NOTIFICATION_ADMIN_EVENTS_SNAPSHOT_ENABLED") == "true"},
		{os.Getenv("NOTIFICATION_CAMPAIGN_SECRET_FILE"), os.Getenv("NOTIFICATION_CAMPAIGNS_ENABLED") == "true"},
	} {
		if item.path == "" {
			if item.required {
				return ErrConfiguration
			}
			continue
		}
		file, err := os.Open(item.path)
		if err != nil {
			return ErrConfiguration
		}
		raw, err := io.ReadAll(io.LimitReader(file, 4097))
		file.Close()
		if err != nil || len(raw) > 4096 {
			return ErrConfiguration
		}
		key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil || len(key) < 32 {
			return ErrConfiguration
		}
		for _, other := range secrets {
			if bytes.Equal(key, other) {
				return ErrConfiguration
			}
		}
		secrets = append(secrets, key)
	}
	return nil
}
