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
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LegacyMappingHandler is the private Bridge lookup from an adopted legacy
// asset to its modern object (Section 19). Only admin-backoffice-service may
// call it, signing with its dedicated private Media case key; every request
// consumes a durable nonce. The answer is opaque IDs plus the attestation
// digest; it never carries bytes, URLs or case facts.
type LegacyMappingHandler struct {
	adoption *usecases.LegacyAdoptionService
	pool     *pgxpool.Pool
	secret   []byte
	enabled  bool
}

func NewLegacyMappingHandler(adoption *usecases.LegacyAdoptionService, pool *pgxpool.Pool, secret []byte, enabled bool) *LegacyMappingHandler {
	return &LegacyMappingHandler{adoption: adoption, pool: pool, secret: append([]byte{}, secret...), enabled: enabled}
}

func (h *LegacyMappingHandler) Register(root *gin.Engine) {
	root.POST(adminmedia.LegacyMappingPath, h.lookup)
}

func (h *LegacyMappingHandler) lookup(c *gin.Context) {
	var payload adminmedia.LegacyMappingRequest
	ctx, cancel, ok := authenticateBridge(c, h.enabled && h.adoption != nil, h.pool, h.secret, "legacy-mapping-v1", "media-legacy-mapping-v1:", "legacy_mapping_disabled", &payload)
	if !ok {
		return
	}
	defer cancel()
	if !payload.Valid() {
		caseFail(c, 400, "invalid_request")
		return
	}
	mapping, err := h.adoption.LookupMapping(ctx, adminmedia.Caller, payload.LegacyAssetID)
	switch {
	case err == nil:
		c.JSON(200, mapping)
	case errors.Is(err, usecases.ErrNotFound):
		caseFail(c, 404, "legacy_mapping_unavailable")
	case errors.Is(err, usecases.ErrLegacyMappingInconsistent):
		caseFail(c, 409, "legacy_mapping_inconsistent")
	default:
		caseFail(c, 503, "legacy_mapping_unavailable")
	}
}

// LegacyMappingRuntime is off unless MEDIA_LEGACY_MAPPING_LOOKUP_ENABLED=true.
// It then requires the private Media service and the Bridge's private case
// key (MEDIA_ADMIN_CASE_SECRET_FILE), which must differ from the download
// signing key and every owner credential.
func LegacyMappingRuntime(ctx context.Context, service *usecases.Service, pool *pgxpool.Pool, downloadSecret []byte, others serviceauth.Secrets) (*LegacyMappingHandler, error) {
	if os.Getenv("MEDIA_LEGACY_MAPPING_LOOKUP_ENABLED") != "true" {
		return &LegacyMappingHandler{}, nil
	}
	if service == nil || pool == nil {
		return nil, errors.New("legacy mapping lookup requires the private Media service")
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
	for _, other := range others {
		if bytes.Equal(key, other) {
			return nil, errors.New("private Media case credential cannot reuse owner credentials")
		}
	}
	ready, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var ok bool
	if err := pool.QueryRow(ready, `SELECT to_regclass('media_legacy_adoptions') IS NOT NULL AND to_regclass('media_service_request_nonces') IS NOT NULL`).Scan(&ok); err != nil || !ok {
		return nil, errors.New("legacy mapping schema unavailable; apply normal migrations")
	}
	return NewLegacyMappingHandler(service.NewLegacyAdoption(usecases.LegacyAdoptionConfig{}), pool, key, true), nil
}
