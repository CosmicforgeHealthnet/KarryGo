package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"time"

	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicyenv"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AdminCaseRuntime never manufactures a source adapter or a missing policy.
// The caller must enroll concrete, independently authorizing case adapters.
func AdminCaseRuntime(ctx context.Context, service *usecases.Service, pool *pgxpool.Pool, legacy serviceauth.Secrets, downloadSecret []byte, authority usecases.CaseAuthority) (*AdminCaseHandler, error) {
	reads := os.Getenv("MEDIA_ADMIN_CASE_READ_ENABLED") == "true"
	revokes := os.Getenv("MEDIA_ADMIN_CASE_REVOKE_ENABLED") == "true"
	if !reads && !revokes || service == nil {
		return &AdminCaseHandler{}, nil
	}
	if pool == nil {
		return nil, errors.New("private Media case database required")
	}
	raw, err := os.ReadFile(os.Getenv("MEDIA_ADMIN_CASE_SECRET_FILE"))
	if err != nil || len(raw) > 4096 {
		return nil, errors.New("private Media case credential unavailable")
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) < 32 {
		return nil, errors.New("invalid private Media case credential")
	}
	if bytes.Equal(key, downloadSecret) {
		return nil, errors.New("private Media case credential cannot reuse download signing key")
	}
	for _, old := range legacy {
		if bytes.Equal(key, old) {
			return nil, errors.New("private Media case credential cannot reuse owner credentials")
		}
	}
	var policy *adminpolicy.Engine
	if reads {
		policy, err = adminpolicy.LoadFiles(os.Getenv("MEDIA_ADMIN_CASE_POLICY_FILE"), os.Getenv("MEDIA_ADMIN_CASE_POLICY_KEYS_FILE"), adminpolicyenv.Environment, time.Now())
		if err != nil && !errors.Is(err, adminpolicy.ErrBlocked) {
			return nil, errors.New("invalid private Media case policy")
		}
	}
	ready, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var ok bool
	if err := pool.QueryRow(ready, `SELECT to_regclass('media_service_request_nonces') IS NOT NULL AND to_regclass('media_admin_case_grants') IS NOT NULL AND to_regclass('media_admin_case_revocations') IS NOT NULL AND to_regclass('media_admin_case_policy_heads') IS NOT NULL`).Scan(&ok); err != nil || !ok {
		return nil, errors.New("private Media case schema unavailable; apply normal migrations")
	}
	access := service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: reads, Policy: policy, Authority: authority})
	return NewAdminCaseHandler(access, pool, key, reads, revokes), nil
}
