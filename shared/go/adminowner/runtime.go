package adminowner

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicyenv"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrConfiguration = errors.New("invalid source-owned admin configuration")

func readFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrConfiguration
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrConfiguration
	}
	return raw, nil
}
func secretFile(path string) ([]byte, error) {
	raw, err := readFile(path, 4096)
	if err != nil {
		return nil, err
	}
	var value string
	for _, b := range raw {
		if b != '\n' && b != '\r' {
			value += string(b)
		}
	}
	secret, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(secret) < 32 {
		return nil, ErrConfiguration
	}
	return secret, nil
}
func ownerKeys(path string) (map[string]ed25519.PublicKey, error) {
	keys := map[string]ed25519.PublicKey{}
	if path == "" {
		return keys, nil
	}
	raw, err := readFile(path, 16<<10)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytesReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrConfiguration
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, ErrConfiguration
		}
		id, ok := token.(string)
		if !ok || !adminread.Opaque(id) || keys[id] != nil || len(keys) >= 32 {
			return nil, ErrConfiguration
		}
		var encoded string
		if decoder.Decode(&encoded) != nil {
			return nil, ErrConfiguration
		}
		key, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || len(key) != ed25519.PublicKeySize {
			return nil, ErrConfiguration
		}
		keys[id] = key
	}
	if _, err := decoder.Token(); err != nil {
		return nil, ErrConfiguration
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrConfiguration
	}
	return keys, nil
}

// FromEnvironment is called by the source composition root, with a fixed owner,
// prefix and route dictionary. Disabled defaults do not touch credentials or
// create schema. TEST_ONLY approvals require the explicit test build tag.
func FromEnvironment(ctx context.Context, db *pgxpool.Pool, source Source, definitions map[string]Definition, owner, prefix, path string) (*Handler, error) {
	h := &Handler{Owner: owner, Path: path, DB: db, Source: source, Definitions: definitions, Enabled: os.Getenv(prefix+"_ADMIN_RESOURCE_READ_ENABLED") == "true", PublishEnabled: os.Getenv(prefix+"_ADMIN_CONTEXT_PUBLISH_ENABLED") == "true"}
	h.Contexts = &admincontext.Store{DB: db, Reader: source, Authority: admincontext.Authority{Owner: owner, Environment: adminpolicyenv.Environment, Keys: map[string]ed25519.PublicKey{}}}
	cases, caseErr := caseFromEnvironment(ctx, db, source, owner, prefix, path)
	if caseErr != nil {
		return nil, caseErr
	}
	h.Cases = cases
	if !h.Enabled && !h.PublishEnabled {
		return h, nil
	}
	if db == nil || source == nil {
		return nil, ErrConfiguration
	}
	var ready bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('admin_owner_context_heads') IS NOT NULL AND to_regclass('admin_owner_context_history') IS NOT NULL AND to_regclass('admin_owner_read_nonces') IS NOT NULL AND to_regclass('admin_owner_read_audit') IS NOT NULL AND to_regclass('admin_owner_policy_revisions') IS NOT NULL AND to_regclass('admin_owner_context_preparation_audit') IS NOT NULL`).Scan(&ready); err != nil || !ready {
		return nil, ErrConfiguration
	}
	keys, err := ownerKeys(os.Getenv(prefix + "_ADMIN_CONTEXT_KEYS_FILE"))
	if err != nil {
		return nil, err
	}
	h.Contexts.Authority.Keys = keys
	if h.Enabled {
		h.Secret, err = secretFile(os.Getenv(prefix + "_ADMIN_RESOURCE_SECRET_FILE"))
		if err != nil {
			return nil, err
		}
		h.Policy, err = adminpolicy.LoadFiles(os.Getenv(prefix+"_ADMIN_ACCESS_POLICY_FILE"), os.Getenv(prefix+"_ADMIN_ACCESS_POLICY_KEYS_FILE"), adminpolicyenv.Environment, time.Now())
		if err != nil && !errors.Is(err, adminpolicy.ErrBlocked) {
			return nil, ErrConfiguration
		}
		if h.Policy != nil {
			if err := adminpolicy.PinRevision(ctx, db, "admin_owner_policy_revisions", h.Policy); err != nil {
				return nil, ErrConfiguration
			}
		}
	}
	if h.PublishEnabled {
		h.PublishSecret, err = secretFile(os.Getenv(prefix + "_ADMIN_CONTEXT_PUBLISH_SECRET_FILE"))
		if err != nil {
			return nil, err
		}
	}
	if h.Cases != nil {
		for _, key := range [][]byte{h.Secret, h.PublishSecret} {
			if len(key) > 0 && (bytes.Equal(key, h.Cases.Secret) || bytes.Equal(key, h.Cases.Publisher.PublishSecret)) {
				return nil, ErrConfiguration
			}
		}
	}
	return h, nil
}
