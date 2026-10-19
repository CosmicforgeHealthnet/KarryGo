package adminowner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

const ContextPublisher = "owner-authorization-context-publisher"

func bytesReader(raw []byte) *bytes.Reader { return bytes.NewReader(raw) }

// PublishContext can carry only an independently source-owner-signed approval.
// Its workload secret is separate from the bridge read secret; HQ staff cannot
// approve a missing context by changing tenant, product or provenance input.
func (h *Handler) PublishContext(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if !h.PublishEnabled || h.DB == nil || h.Contexts == nil || len(h.Contexts.Authority.Keys) == 0 || len(h.PublishSecret) < 32 {
		h.fail(c, 503, "owner_context_publication_disabled")
		return
	}
	r := c.Request
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || r.URL.RawPath != "" || r.URL.RawQuery != "" || strings.ContainsAny(r.URL.Path, "%\\") {
		h.fail(c, 400, "invalid_request")
		return
	}
	for _, header := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderNonce, serviceauth.HeaderTimestamp, serviceauth.HeaderSignature} {
		if len(r.Header.Values(header)) != 1 {
			h.fail(c, 401, "unauthorized")
			return
		}
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (32<<10)+1))
	if err != nil || len(raw) > 32<<10 {
		h.fail(c, 413, "request_too_large")
		return
	}
	r.Body = io.NopCloser(bytesReader(raw))
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{ContextPublisher: h.PublishSecret}, 30*time.Second)
	if _, err := verifier.VerifyRequest(r); err != nil {
		h.fail(c, 401, "unauthorized")
		return
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !adminread.Opaque(nonce) {
		h.fail(c, 401, "unauthorized")
		return
	}
	hash := sha256.Sum256([]byte(h.Owner + ":" + ContextPublisher + ":" + nonce))
	tag, err := h.DB.Exec(ctx, `INSERT INTO admin_owner_read_nonces(owner,nonce_hash,expires_at) VALUES($1,$2,now()+interval '11 minutes') ON CONFLICT DO NOTHING`, h.Owner, hex.EncodeToString(hash[:]))
	if err != nil {
		h.fail(c, 503, "replay_protection_unavailable")
		return
	}
	if tag.RowsAffected() != 1 {
		h.fail(c, 409, "replay_rejected")
		return
	}
	if err := h.Contexts.Import(ctx, raw, time.Now()); err != nil {
		h.fail(c, 403, "source_owner_approval_required")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "RECORDED"})
}
