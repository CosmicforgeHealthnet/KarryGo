package adminreview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/customerreview"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	Service                 *Service
	WriteSecret, ReadSecret []byte
}

func (h *Handler) Register(root *gin.Engine) {
	root.POST(customerreview.Path, h.signedRequest)
	root.GET(customerreview.Path+"/:customer/:key", h.signedReceipt)
}
func (h *Handler) fail(c *gin.Context, status int, code string, q *adminread.Query, target string) {
	if q != nil && h.Service != nil && h.Service.DB != nil {
		if !adminread.Opaque(target) {
			target = ""
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
		defer cancel()
		_, err := h.Service.DB.Exec(ctx, `INSERT INTO admin_owner_read_audit(owner,resource_kind,target_ref,staff_ref,purpose_code,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,as_of,outcome) VALUES('customer-service','review-requests',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now(),$11)`, target, q.Staff, q.Purpose, q.Scope.Market, q.Scope.Zone, q.Scope.Organization, q.Scope.Tenant, q.PolicyID, q.PolicyVersion, q.PolicyChecksum, code)
		if err != nil {
			status = 503
			code = "audit_unavailable"
		}
	}
	c.JSON(status, gin.H{"error": code})
}
func (h *Handler) authenticate(c *gin.Context, write bool) (*adminread.Query, []byte, bool) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	secret := h.ReadSecret
	enabled := h.Service != nil && h.Service.ReadEnabled
	if write {
		secret = h.WriteSecret
		enabled = h.Service != nil && h.Service.WriteEnabled
	}
	if !enabled || len(secret) < 32 || h.Service.DB == nil {
		h.fail(c, 503, "review_capability_disabled", nil, "")
		return nil, nil, false
	}
	r := c.Request
	if r.Body == nil {
		r.Body = http.NoBody
	}
	if r.URL.RawPath != "" || len(r.URL.RequestURI()) > 8192 || strings.ContainsAny(r.URL.Path, "%\\") {
		h.fail(c, 400, "invalid_request", nil, "")
		return nil, nil, false
	}
	var raw []byte
	if write {
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "application/json" {
			h.fail(c, 415, "json_required", nil, "")
			return nil, nil, false
		}
		raw, err = io.ReadAll(io.LimitReader(r.Body, 16385))
		if err != nil || len(raw) > 16384 {
			h.fail(c, 413, "invalid_body", nil, "")
			return nil, nil, false
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
	} else if r.ContentLength != 0 || len(r.TransferEncoding) > 0 || r.Body != nil && r.Body != http.NoBody {
		h.fail(c, 400, "body_forbidden", nil, "")
		return nil, nil, false
	}
	for _, key := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderNonce, serviceauth.HeaderTimestamp, serviceauth.HeaderSignature} {
		if len(r.Header.Values(key)) != 1 {
			h.fail(c, 401, "unauthorized", nil, "")
			return nil, nil, false
		}
	}
	if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminread.Caller: secret}, 30*time.Second).VerifyRequest(r); err != nil {
		h.fail(c, 401, "unauthorized", nil, "")
		return nil, nil, false
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !adminread.Opaque(nonce) {
		h.fail(c, 401, "unauthorized", nil, "")
		return nil, nil, false
	}
	sum := sha256.Sum256([]byte("customer-service:review:" + r.Method + ":" + nonce))
	tag, err := h.Service.DB.Exec(r.Context(), `INSERT INTO admin_owner_read_nonces(owner,nonce_hash,expires_at) VALUES('customer-service',$1,now()+interval '11 minutes') ON CONFLICT DO NOTHING`, hex.EncodeToString(sum[:]))
	if err != nil {
		h.fail(c, 503, "replay_protection_unavailable", nil, "")
		return nil, nil, false
	}
	if tag.RowsAffected() != 1 {
		h.fail(c, 409, "replay_rejected", nil, "")
		return nil, nil, false
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(c, 400, "invalid_query", nil, "")
		return nil, nil, false
	}
	q, err := adminread.Parse(values, time.Now())
	if err != nil || q.Cursor != "" {
		h.fail(c, 400, "invalid_query", nil, "")
		return nil, nil, false
	}
	if h.Service.Policy == nil || q.PolicyID != h.Service.Policy.ID() || q.PolicyVersion != h.Service.Policy.Version() || q.PolicyChecksum != h.Service.Policy.Checksum() {
		h.fail(c, 503, "policy_revision_mismatch", &q, "")
		return nil, nil, false
	}
	return &q, raw, true
}
func reviewActor(q *adminread.Query, capability string) (adminpolicy.Actor, adminpolicy.Scope) {
	scope := adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
	actor := adminpolicy.Actor{Product: "karryam", Staff: q.Staff, Roles: []string{q.Role}, Permissions: []string{capability}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}}
	return actor, scope
}
func (h *Handler) problem(c *gin.Context, err error, q *adminread.Query, target string) {
	switch {
	case errors.Is(err, ErrDisabled), errors.Is(err, adminpolicy.ErrBlocked):
		h.fail(c, 503, "capability_blocked", q, target)
	case errors.Is(err, customerreview.ErrInvalid):
		h.fail(c, 400, "invalid_request", q, target)
	case errors.Is(err, ErrConflict):
		h.fail(c, 409, "identity_or_revision_conflict", q, target)
	case errors.Is(err, ErrNotFound), errors.Is(err, admincontext.ErrUnmapped):
		h.fail(c, 404, "not_found", q, target)
	case errors.Is(err, adminpolicy.ErrDenied):
		h.fail(c, 403, "forbidden", q, target)
	default:
		h.fail(c, 503, "owner_outcome_unavailable", q, target)
	}
}
func (h *Handler) signedRequest(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	q, raw, ok := h.authenticate(c, true)
	if !ok {
		return
	}
	input, err := customerreview.DecodeRequest(raw)
	if err != nil {
		h.problem(c, err, q, "")
		return
	}
	actor, scope := reviewActor(q, customerreview.RequestCapability)
	receipt, err := h.Service.Request(ctx, actor, q.Purpose, scope, input)
	if err != nil {
		h.problem(c, err, q, input.CustomerID)
		return
	}
	c.JSON(200, receipt)
}
func (h *Handler) signedReceipt(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	q, _, ok := h.authenticate(c, false)
	if !ok {
		return
	}
	actor, scope := reviewActor(q, customerreview.ReadCapability)
	receipt, err := h.Service.Receipt(ctx, actor, q.Purpose, c.Param("customer"), c.Param("key"), scope)
	if err != nil {
		h.problem(c, err, q, c.Param("customer"))
		return
	}
	c.JSON(200, receipt)
}
