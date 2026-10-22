package config

import (
	"bytes"
	"cosmicforge/logistics/shared/go/adminmedia"
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

func (c Config) MediaCaseClient() (*adminmedia.Client, bool, bool, error) {
	reads, revokes := os.Getenv("ADMIN_MEDIA_CASE_READ_ENABLED") == "true", os.Getenv("ADMIN_MEDIA_CASE_REVOKE_ENABLED") == "true"
	if !reads && !revokes {
		return nil, false, false, nil
	}
	readKey := func(path string) ([]byte, error) {
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) > 4096 {
			return nil, errors.New("private Media key unavailable")
		}
		key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil || len(key) < 32 {
			return nil, errors.New("invalid private Media key")
		}
		return key, nil
	}
	key, err := readKey(os.Getenv("ADMIN_MEDIA_CASE_SECRET_FILE"))
	if err != nil {
		return nil, false, false, err
	}
	for _, encoded := range c.Trust.ServiceKeys {
		hq, _ := base64.RawURLEncoding.DecodeString(encoded)
		if bytes.Equal(hq, key) {
			return nil, false, false, errors.New("Media case key reused HQ audience")
		}
	}
	for _, owner := range c.OwnerResources {
		other, err := readKey(owner.SecretFile)
		if err != nil {
			return nil, false, false, err
		}
		if bytes.Equal(other, key) {
			return nil, false, false, errors.New("Media case key reused source resource audience")
		}
	}
	activeKeys := map[string]bool{
		"ADMIN_ANALYTICS_SCOPED_SECRET_FILE":         c.Features.AnalyticsReads && os.Getenv("ADMIN_ANALYTICS_SCOPED_READ_ENABLED") == "true",
		"ADMIN_CUSTOMER_REVIEW_WRITE_SECRET_FILE":    os.Getenv("ADMIN_CUSTOMER_REVIEW_WRITES_ENABLED") == "true" || os.Getenv("ADMIN_CUSTOMER_REVIEW_WORK_ITEM_WRITES_ENABLED") == "true",
		"ADMIN_CUSTOMER_REVIEW_READBACK_SECRET_FILE": os.Getenv("ADMIN_CUSTOMER_REVIEW_READBACK_ENABLED") == "true" || os.Getenv("ADMIN_CUSTOMER_REVIEW_WORK_ITEM_READBACK_ENABLED") == "true",
	}
	if c.TaxiReadEnabled {
		other, err := readKey(c.TaxiSecretFile)
		if err != nil {
			return nil, false, false, err
		}
		if bytes.Equal(other, key) {
			return nil, false, false, errors.New("Media case key reused Taxi audience")
		}
	}
	for name, active := range activeKeys {
		if !active {
			continue
		}
		other, err := readKey(os.Getenv(name))
		if err != nil {
			return nil, false, false, err
		}
		if bytes.Equal(other, key) {
			return nil, false, false, errors.New("Media case key reused outbound audience")
		}
	}
	client, err := adminmedia.NewClient(os.Getenv("ADMIN_MEDIA_CASE_ORIGIN"), key)
	return client, reads, revokes, err
}
