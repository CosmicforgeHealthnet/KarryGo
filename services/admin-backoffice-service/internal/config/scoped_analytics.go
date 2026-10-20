package config

import (
	"bytes"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

func (c Config) ScopedAnalyticsClient() (*analyticsadmin.Client, error) {
	if !c.Features.AnalyticsReads || os.Getenv("ADMIN_ANALYTICS_SCOPED_READ_ENABLED") != "true" {
		return nil, nil
	}
	raw, err := os.ReadFile(os.Getenv("ADMIN_ANALYTICS_SCOPED_SECRET_FILE"))
	if err != nil || len(raw) > 4096 {
		return nil, errors.New("scoped Analytics credential unavailable")
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) < 32 {
		return nil, errors.New("invalid scoped Analytics key")
	}
	for _, encoded := range c.Trust.ServiceKeys {
		hq, _ := base64.RawURLEncoding.DecodeString(encoded)
		if bytes.Equal(hq, key) {
			return nil, errors.New("scoped Analytics cannot reuse HQ credentials")
		}
	}
	return analyticsadmin.NewClient(os.Getenv("ADMIN_ANALYTICS_SCOPED_ORIGIN"), key)
}
