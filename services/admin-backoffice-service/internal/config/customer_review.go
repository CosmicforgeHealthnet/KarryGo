package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"strings"

	"cosmicforge/logistics/shared/go/customerreview"
)

// CustomerReviewClient loads no files unless explicitly enabled. Enabling
// writes requires the independent original-identity readback channel as well.
func (c Config) CustomerReviewClient() (*customerreview.Client, bool, bool, error) {
	write := os.Getenv("ADMIN_CUSTOMER_REVIEW_WRITES_ENABLED") == "true"
	read := os.Getenv("ADMIN_CUSTOMER_REVIEW_READBACK_ENABLED") == "true"
	if !write && !read {
		return nil, false, false, nil
	}
	if write && !read {
		return nil, false, false, errors.New("Customer review writes require independent readback")
	}
	var customer *OwnerResources
	for i := range c.OwnerResources {
		if c.OwnerResources[i].Owner == customerreview.Owner {
			customer = &c.OwnerResources[i]
		}
	}
	if customer == nil {
		return nil, false, false, errors.New("Customer review requires configured canonical Customer origin")
	}
	key := func(path string) ([]byte, error) {
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) > 4096 {
			return nil, errors.New("Customer review key file unavailable")
		}
		decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil || len(decoded) < 32 {
			return nil, errors.New("invalid Customer review key")
		}
		return decoded, nil
	}
	resourceKey, err := key(customer.SecretFile)
	if err != nil {
		return nil, false, false, err
	}
	readKey, err := key(os.Getenv("ADMIN_CUSTOMER_REVIEW_READBACK_SECRET_FILE"))
	if err != nil {
		return nil, false, false, err
	}
	var writeKey []byte
	if write {
		writeKey, err = key(os.Getenv("ADMIN_CUSTOMER_REVIEW_WRITE_SECRET_FILE"))
		if err != nil {
			return nil, false, false, err
		}
	}
	for _, secret := range [][]byte{writeKey, readKey} {
		if len(secret) == 0 {
			continue
		}
		if bytes.Equal(secret, resourceKey) {
			return nil, false, false, errors.New("Customer review credential reused resource audience")
		}
		for _, encoded := range c.Trust.ServiceKeys {
			hqKey, _ := base64.RawURLEncoding.DecodeString(encoded)
			if bytes.Equal(secret, hqKey) {
				return nil, false, false, errors.New("Customer review credential reused HQ audience")
			}
		}
	}
	client, err := customerreview.NewClient(customer.Origin, writeKey, readKey)
	return client, write, read, err
}
