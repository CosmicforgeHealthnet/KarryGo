package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"os"
	"strings"
	"time"

	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/repositories"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LegacyAdoptionHandler is the private, owner-attested legacy adoption route.
// Each enrolled source owner signs as itself with a dedicated key; the
// authenticated caller is the only owner whose legacy assets it may adopt.
type LegacyAdoptionHandler struct {
	adoption *usecases.LegacyAdoptionService
	pool     *pgxpool.Pool
	secrets  serviceauth.Secrets
	enabled  bool
}

func NewLegacyAdoptionHandler(adoption *usecases.LegacyAdoptionService, pool *pgxpool.Pool, secrets serviceauth.Secrets, enabled bool) *LegacyAdoptionHandler {
	copied := serviceauth.Secrets{}
	for k, v := range secrets {
		copied[k] = append([]byte{}, v...)
	}
	return &LegacyAdoptionHandler{adoption: adoption, pool: pool, secrets: copied, enabled: enabled}
}
func (h *LegacyAdoptionHandler) Register(root *gin.Engine) {
	root.POST(adminmedia.LegacyAdoptionPath, h.adopt)
}
func (h *LegacyAdoptionHandler) adopt(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if !h.enabled || h.adoption == nil || h.pool == nil || len(h.secrets) == 0 {
		caseFail(c, 503, "legacy_adoption_disabled")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	r := c.Request
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.URL.RawQuery != "" || r.URL.RawPath != "" || strings.ContainsAny(r.URL.Path, "%\\") {
		caseFail(c, 400, "invalid_request")
		return
	}
	for _, name := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderTimestamp, serviceauth.HeaderNonce, serviceauth.HeaderSignature} {
		if len(r.Header.Values(name)) != 1 {
			caseFail(c, 401, "unauthorized")
			return
		}
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, adminmedia.MaxBody+1))
	if err != nil || len(raw) > adminmedia.MaxBody {
		caseFail(c, 413, "request_too_large")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	caller, err := serviceauth.NewVerifier(h.secrets, 30*time.Second).VerifyRequest(r)
	if err != nil {
		caseFail(c, 401, "unauthorized")
		return
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !adminread.Opaque(nonce) {
		caseFail(c, 401, "unauthorized")
		return
	}
	nonceHash := sha256.Sum256([]byte("media-legacy-adoption-v1:" + caller + ":" + nonce))
	fingerprint := sha256.Sum256(append([]byte(r.Method+"\x00"+r.URL.Path+"\x00"), raw...))
	err = repositories.ClaimServiceNonce(ctx, h.pool, caller+"/legacy-adoption-v1", hex.EncodeToString(nonceHash[:]), hex.EncodeToString(fingerprint[:]), time.Now().UTC().Add(10*time.Minute))
	if errors.Is(err, repositories.ErrConflict) {
		caseFail(c, 409, "replay_rejected")
		return
	}
	if err != nil {
		caseFail(c, 503, "replay_protection_unavailable")
		return
	}
	var payload adminmedia.LegacyAdoption
	if adminmedia.Decode(raw, &payload) != nil || !payload.Valid(caller) {
		caseFail(c, 400, "invalid_request")
		return
	}
	receipt, err := h.adoption.Adopt(ctx, caller, payload)
	switch {
	case err == nil:
		c.JSON(200, receipt)
	case errors.Is(err, usecases.ErrLegacyDenied):
		caseFail(c, 403, "forbidden")
	case errors.Is(err, usecases.ErrNotFound):
		caseFail(c, 404, "legacy_asset_unavailable")
	case errors.Is(err, usecases.ErrConflict):
		caseFail(c, 409, "adoption_conflict")
	default:
		caseFail(c, 503, "legacy_adoption_unavailable")
	}
}

// LegacyAdoptionRuntime is off unless MEDIA_LEGACY_ADOPTION_ENABLED=true. It
// then requires the private Media service, the legacy storage client, its
// bucket and MEDIA_LEGACY_ADOPTION_SOURCES_FILE ({"sources":[{"owner",
// "secret_file"}]}). Only owners with a fixed legacy identity are accepted;
// every key must differ from each other and from every existing credential.
func LegacyAdoptionRuntime(service *usecases.Service, pool *pgxpool.Pool, content usecases.LegacyContent, bucket string, maxBytes int64, others [][]byte) (*LegacyAdoptionHandler, error) {
	if os.Getenv("MEDIA_LEGACY_ADOPTION_ENABLED") != "true" {
		return &LegacyAdoptionHandler{}, nil
	}
	if service == nil || pool == nil || content == nil || bucket == "" {
		return nil, errors.New("legacy adoption requires the private Media service and legacy storage")
	}
	raw, err := os.ReadFile(os.Getenv("MEDIA_LEGACY_ADOPTION_SOURCES_FILE"))
	if err != nil || len(raw) > 16384 {
		return nil, errors.New("legacy adoption enrollment unavailable")
	}
	var config struct {
		Sources []struct {
			Owner      string `json:"owner"`
			SecretFile string `json:"secret_file"`
		} `json:"sources"`
	}
	if admincase.Strict(raw, &config) != nil || len(config.Sources) == 0 {
		return nil, errors.New("invalid legacy adoption enrollment")
	}
	if path := os.Getenv("MEDIA_ADMIN_CASE_SECRET_FILE"); path != "" {
		if encoded, e := os.ReadFile(path); e == nil {
			if key, e := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded))); e == nil {
				others = append(others, key)
			}
		}
	}
	secrets := serviceauth.Secrets{}
	for _, entry := range config.Sources {
		if _, ok := usecases.LegacyIdentity(entry.Owner); !ok || secrets[entry.Owner] != nil {
			return nil, errors.New("unrecognized or duplicate legacy adoption owner")
		}
		encoded, err := os.ReadFile(entry.SecretFile)
		if err != nil || len(encoded) > 4096 {
			return nil, errors.New("legacy adoption key unavailable")
		}
		key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded)))
		if err != nil || len(key) < 32 {
			return nil, errors.New("invalid legacy adoption key")
		}
		for _, other := range others {
			if bytes.Equal(key, other) {
				return nil, errors.New("legacy adoption key reuses another credential")
			}
		}
		others = append(others, key)
		secrets[entry.Owner] = key
	}
	ready, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var ok bool
	if err := pool.QueryRow(ready, `SELECT to_regclass('media_legacy_adoptions') IS NOT NULL AND to_regclass('media_service_request_nonces') IS NOT NULL`).Scan(&ok); err != nil || !ok {
		return nil, errors.New("legacy adoption schema unavailable; apply normal migrations")
	}
	adoption := service.NewLegacyAdoption(usecases.LegacyAdoptionConfig{Content: content, Bucket: bucket, MaxBytes: maxBytes})
	return NewLegacyAdoptionHandler(adoption, pool, secrets, true), nil
}
