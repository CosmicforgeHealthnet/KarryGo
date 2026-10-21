package adminreview

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminowner"
)

var ErrConfiguration = errors.New("invalid Customer review capability configuration")

func reviewSecret(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrConfiguration
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(raw) > 4096 {
		return nil, ErrConfiguration
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(string(raw), "\r\n"))
	if err != nil || len(key) < 32 {
		return nil, ErrConfiguration
	}
	return key, nil
}

// Runtime does not load keys, policy or schema while both switches are off.
// It uses the already initialized source owner's context/policy boundary and
// separately mounted readback/write keys; ordinary resource credentials cannot
// acquire command authority. A write kill switch leaves readbacks available.
func Runtime(owner *adminowner.Handler) (*Handler, error) {
	write, read := os.Getenv("CUSTOMER_ADMIN_REVIEW_WRITES_ENABLED") == "true", os.Getenv("CUSTOMER_ADMIN_REVIEW_READBACK_ENABLED") == "true"
	h := &Handler{Service: &Service{WriteEnabled: write, ReadEnabled: read}}
	if !write && !read {
		return h, nil
	}
	if owner == nil || !owner.Enabled || owner.DB == nil || owner.Contexts == nil || len(owner.Contexts.Authority.Keys) == 0 {
		return nil, ErrConfiguration
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var schemaReady bool
	if err := owner.DB.QueryRow(ctx, `SELECT to_regclass('customer_admin_review_state') IS NOT NULL AND to_regclass('customer_admin_review_requests') IS NOT NULL AND to_regclass('customer_admin_review_work_items') IS NOT NULL AND to_regclass('customer_admin_review_audit') IS NOT NULL`).Scan(&schemaReady); err != nil || !schemaReady {
		return nil, ErrConfiguration
	}
	h.Service.DB = owner.DB
	h.Service.Contexts = owner.Contexts
	h.Service.Policy = owner.Policy
	var err error
	if write {
		h.WriteSecret, err = reviewSecret(os.Getenv("CUSTOMER_ADMIN_REVIEW_WRITE_SECRET_FILE"))
		if err != nil {
			return nil, err
		}
		if bytes.Equal(h.WriteSecret, owner.Secret) {
			return nil, ErrConfiguration
		}
	}
	if read {
		h.ReadSecret, err = reviewSecret(os.Getenv("CUSTOMER_ADMIN_REVIEW_READBACK_SECRET_FILE"))
		if err != nil {
			return nil, err
		}
		if bytes.Equal(h.ReadSecret, owner.Secret) || write && bytes.Equal(h.ReadSecret, h.WriteSecret) {
			return nil, ErrConfiguration
		}
	}
	return h, nil
}
