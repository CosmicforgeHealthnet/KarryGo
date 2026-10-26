package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/repositories"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AdminCaseHandler struct {
	access         *usecases.CaseAccessService
	pool           *pgxpool.Pool
	secret         []byte
	reads, revokes bool
}

func NewAdminCaseHandler(access *usecases.CaseAccessService, pool *pgxpool.Pool, secret []byte, reads, revokes bool) *AdminCaseHandler {
	return &AdminCaseHandler{access: access, pool: pool, secret: append([]byte{}, secret...), reads: reads, revokes: revokes}
}
func (h *AdminCaseHandler) Register(root *gin.Engine) {
	root.POST(adminmedia.GrantPath, h.issue)
	root.POST(adminmedia.GrantPath+"/:id/content", h.read)
	root.POST(adminmedia.GrantPath+"/:id/revocations", h.revoke)
}
func caseFail(c *gin.Context, status int, code string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code}})
}
func (h *AdminCaseHandler) authenticate(c *gin.Context, enabled bool, out any) (context.Context, context.CancelFunc, bool) {
	return authenticateBridge(c, enabled && h.access != nil, h.pool, h.secret, "case-access-v1", "media-admin-case-v1:", "case_access_disabled", out)
}

// authenticateBridge is the shared private Bridge transport check: a single
// JSON POST with no query, signed by the Bridge caller with its dedicated
// key, a durable single-use nonce (per route family) and strict decoding.
func authenticateBridge(c *gin.Context, enabled bool, pool *pgxpool.Pool, secret []byte, family, noncePrefix, disabledCode string, out any) (context.Context, context.CancelFunc, bool) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	fail := func(status int, code string) (context.Context, context.CancelFunc, bool) {
		caseFail(c, status, code)
		cancel()
		return nil, func() {}, false
	}
	if !enabled || pool == nil || len(secret) < 32 {
		return fail(503, disabledCode)
	}
	r := c.Request
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.Method != "POST" || r.URL.RawQuery != "" || r.URL.RawPath != "" || strings.ContainsAny(r.URL.Path, "%\\") || len(r.URL.RequestURI()) > 1024 {
		return fail(400, "invalid_request")
	}
	for _, name := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderTimestamp, serviceauth.HeaderNonce, serviceauth.HeaderSignature} {
		if len(r.Header.Values(name)) != 1 {
			return fail(401, "unauthorized")
		}
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, adminmedia.MaxBody+1))
	if err != nil || len(raw) > adminmedia.MaxBody {
		return fail(413, "request_too_large")
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminmedia.Caller: secret}, 30*time.Second).VerifyRequest(r); err != nil {
		return fail(401, "unauthorized")
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !adminread.Opaque(nonce) || nonce != strings.TrimSpace(nonce) {
		return fail(401, "unauthorized")
	}
	nonceHash := sha256.Sum256([]byte(noncePrefix + nonce))
	fingerprint := sha256.Sum256(append([]byte(r.Method+"\x00"+r.URL.Path+"\x00"), raw...))
	err = repositories.ClaimServiceNonce(ctx, pool, adminmedia.Caller+"/"+family, hex.EncodeToString(nonceHash[:]), hex.EncodeToString(fingerprint[:]), time.Now().UTC().Add(10*time.Minute))
	if errors.Is(err, repositories.ErrConflict) {
		return fail(409, "replay_rejected")
	}
	if err != nil {
		return fail(503, "replay_protection_unavailable")
	}
	if adminmedia.Decode(raw, out) != nil {
		return fail(400, "invalid_request")
	}
	if id := c.Param("id"); id != "" {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return fail(400, "invalid_grant_reference")
		}
	}
	return ctx, cancel, true
}
func caseActor(a adminmedia.Actor) usecases.CaseActor {
	return usecases.CaseActor{Delegation: a.Delegation(), ExpiresAt: a.ExpiresAt}
}
func caseError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, adminpolicy.ErrBlocked):
		caseFail(c, 503, "case_policy_or_source_unavailable")
	case errors.Is(err, adminpolicy.ErrDenied):
		caseFail(c, 403, "forbidden")
	case errors.Is(err, usecases.ErrNotFound):
		caseFail(c, 404, "case_grant_unavailable")
	case errors.Is(err, usecases.ErrNotDownloadable):
		caseFail(c, 409, "media_unavailable")
	default:
		caseFail(c, 503, "case_authority_unavailable")
	}
}
func (h *AdminCaseHandler) issue(c *gin.Context) {
	var payload adminmedia.IssueRequest
	ctx, cancel, ok := h.authenticate(c, h.reads, &payload)
	if !ok {
		return
	}
	defer cancel()
	if !payload.Actor.Valid(time.Now()) || !payload.Request.Valid() {
		caseFail(c, 400, "invalid_request")
		return
	}
	grant, err := h.access.Issue(ctx, caseActor(payload.Actor), payload.Request)
	if err != nil {
		caseError(c, err)
		return
	}
	c.JSON(200, grant)
}
func (h *AdminCaseHandler) read(c *gin.Context) {
	var payload adminmedia.ReadRequest
	ctx, cancel, ok := h.authenticate(c, h.reads, &payload)
	if !ok {
		return
	}
	defer cancel()
	if !payload.Actor.Valid(time.Now()) || !payload.Request.Valid() || len(payload.Token) != 43 {
		caseFail(c, 400, "invalid_request")
		return
	}
	content, err := h.access.Read(ctx, caseActor(payload.Actor), payload.Request, c.Param("id"), payload.Token)
	if err != nil {
		caseError(c, err)
		return
	}
	c.Header("X-Media-Case-Contract", adminmedia.Contract)
	c.Header("X-Media-Object-ID", payload.Request.MediaObjectID)
	c.Header("Content-Disposition", "attachment")
	c.Data(http.StatusOK, "application/octet-stream", content)
}
func (h *AdminCaseHandler) revoke(c *gin.Context) {
	var payload adminmedia.RevokeRequest
	ctx, cancel, ok := h.authenticate(c, h.revokes, &payload)
	if !ok {
		return
	}
	defer cancel()
	if !payload.Actor.Valid(time.Now()) || !adminread.Opaque(payload.ReasonCode) {
		caseFail(c, 400, "invalid_request")
		return
	}
	if err := h.access.Revoke(ctx, caseActor(payload.Actor), c.Param("id"), payload.ReasonCode); err != nil {
		caseError(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "REVOKED"})
}
