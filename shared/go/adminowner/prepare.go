package adminowner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// PrepareContext returns only current source facts and a binding fingerprint to
// the independent owner publication workload. It accepts no proposed mapping
// and grants no access. The owner can review this material before signing an
// approval; the HQ read caller cannot prepare or publish a context.
func (h *Handler) PrepareContext(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if !h.PublishEnabled || h.DB == nil || h.Source == nil || len(h.PublishSecret) < 32 {
		h.fail(c, 503, "owner_context_publication_disabled")
		return
	}
	r := c.Request
	definition, ok := h.Definitions[c.Param("kind")]
	ref := c.Param("id")
	if !ok || !adminread.Opaque(ref) {
		h.fail(c, 404, "not_found")
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(values) != 1 || len(values["purpose"]) != 1 || !adminread.Opaque(values.Get("purpose")) || r.Method != "GET" || r.URL.RawPath != "" || strings.ContainsAny(r.URL.Path, "%\\") || r.ContentLength != 0 || len(r.TransferEncoding) > 0 || r.Body != nil && r.Body != http.NoBody {
		h.fail(c, 400, "invalid_request")
		return
	}
	for _, name := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderNonce, serviceauth.HeaderTimestamp, serviceauth.HeaderSignature} {
		if len(r.Header.Values(name)) != 1 {
			h.fail(c, 401, "unauthorized")
			return
		}
	}
	if _, err := serviceauth.NewVerifier(serviceauth.Secrets{ContextPublisher: h.PublishSecret}, 30*time.Second).VerifyRequest(r); err != nil {
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
	tx, err := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		h.fail(c, 503, "owner_unavailable")
		return
	}
	defer tx.Rollback(ctx)
	binding, err := h.Source.Binding(ctx, tx, definition.Kind, ref)
	if err != nil || binding.SourceContract != definition.SourceContract {
		h.fail(c, 404, "not_found")
		return
	}
	known, err := json.Marshal(binding.Canonical)
	if err != nil {
		h.fail(c, 503, "owner_unavailable")
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO admin_owner_context_preparation_audit(owner,resource_kind,target_ref,workload_principal,purpose_code,binding_fingerprint,source_contract,known_source_dimensions) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, h.Owner, definition.Kind, ref, ContextPublisher, values.Get("purpose"), binding.Fingerprint, binding.SourceContract, known); err != nil {
		h.fail(c, 503, "audit_unavailable")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.fail(c, 503, "audit_unavailable")
		return
	}
	c.JSON(http.StatusOK, gin.H{"state": "DRAFT_PENDING_OWNER_APPROVAL", "binding": binding})
}
