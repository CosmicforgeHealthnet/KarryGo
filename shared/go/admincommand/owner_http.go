package admincommand

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

var ErrConfiguration = errors.New("invalid administrative command configuration")

// Handler is the owner's private, HMAC v2 signed command boundary. It is never
// registered on a public route group and carries its own write and readback
// keys, separate from read, case and context publication credentials.
type Handler struct {
	Service                 *Service
	WriteSecret, ReadSecret []byte
}

func (h *Handler) routes(r gin.IRoutes, prefix string) {
	r.POST(prefix, h.execute)
	r.GET(prefix+"/:kind/:id", h.history)
	r.GET(prefix+"/:kind/:id/:key", h.readback)
}
func (h *Handler) Register(root *gin.Engine) { h.routes(root, h.Service.Spec.Path()) }
func (h *Handler) RegisterOn(group *gin.RouterGroup) {
	h.routes(group, strings.TrimPrefix(h.Service.Spec.Path(), strings.TrimRight(group.BasePath(), "/")))
}

func (h *Handler) fail(c *gin.Context, status int, code string, q *adminread.Query, target string) {
	if q != nil && h.Service != nil && h.Service.DB != nil {
		if !adminread.Opaque(target) {
			target = ""
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
		defer cancel()
		if _, err := h.Service.DB.Exec(ctx, `INSERT INTO admin_owner_read_audit(owner,resource_kind,target_ref,staff_ref,purpose_code,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,as_of,outcome) VALUES($1,'admin-commands',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now(),$12)`, h.Service.Spec.Owner, target, q.Staff, q.Purpose, q.Scope.Market, q.Scope.Zone, q.Scope.Organization, q.Scope.Tenant, q.PolicyID, q.PolicyVersion, q.PolicyChecksum, code); err != nil {
			status, code = 503, "audit_unavailable"
		}
	}
	c.JSON(status, gin.H{"error": code})
}

func (h *Handler) authenticate(c *gin.Context, write bool) (*adminread.Query, []byte, bool) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	secret, enabled := h.ReadSecret, h.Service != nil && h.Service.ReadEnabled
	if write {
		secret, enabled = h.WriteSecret, h.Service != nil && h.Service.WriteEnabled
	}
	if !enabled || len(secret) < 32 || h.Service.DB == nil {
		h.fail(c, 503, "capability_blocked", nil, "")
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
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			h.fail(c, 415, "json_required", nil, "")
			return nil, nil, false
		}
		raw, err = io.ReadAll(io.LimitReader(r.Body, maxBody+1))
		if err != nil || len(raw) > maxBody {
			h.fail(c, 413, "invalid_body", nil, "")
			return nil, nil, false
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
	} else if r.ContentLength != 0 || len(r.TransferEncoding) > 0 || r.Body != http.NoBody {
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
	sum := sha256.Sum256([]byte(h.Service.Spec.Owner + ":admin-command:" + r.Method + ":" + nonce))
	tag, err := h.Service.DB.Exec(r.Context(), `INSERT INTO admin_owner_read_nonces(owner,nonce_hash,expires_at) VALUES($1,$2,now()+interval '11 minutes') ON CONFLICT DO NOTHING`, h.Service.Spec.Owner, hex.EncodeToString(sum[:]))
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

// Actor reconstructs the delegated staff identity carried by the signed
// query. The owner independently re-authorizes it against its own policy.
func Actor(q *adminread.Query, capability string) (adminpolicy.Actor, adminpolicy.Scope) {
	scope := adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
	return adminpolicy.Actor{Product: "karryam", Staff: q.Staff, Roles: []string{q.Role}, Permissions: []string{capability}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}}, scope
}

func (h *Handler) problem(c *gin.Context, err error, q *adminread.Query, target string) {
	switch {
	case errors.Is(err, ErrDisabled), errors.Is(err, adminpolicy.ErrBlocked):
		h.fail(c, 503, "capability_blocked", q, target)
	case errors.Is(err, ErrInvalid):
		h.fail(c, 400, "invalid_command", q, target)
	case errors.Is(err, ErrConflict):
		h.fail(c, 409, "identity_or_version_conflict", q, target)
	case errors.Is(err, ErrRejected):
		h.fail(c, 422, "transition_rejected", q, target)
	case errors.Is(err, ErrNotFound), errors.Is(err, admincontext.ErrUnmapped):
		h.fail(c, 404, "not_found", q, target)
	case errors.Is(err, adminpolicy.ErrDenied):
		h.fail(c, 403, "forbidden", q, target)
	default:
		h.fail(c, 503, "owner_outcome_unavailable", q, target)
	}
}
func (h *Handler) execute(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	q, raw, ok := h.authenticate(c, true)
	if !ok {
		return
	}
	cmd, err := DecodeCommand(raw)
	if err != nil {
		h.problem(c, err, q, "")
		return
	}
	actor, scope := Actor(q, h.Service.Spec.Capability)
	receipt, err := h.Service.Execute(ctx, actor, q.Purpose, scope, cmd)
	if err != nil {
		h.problem(c, err, q, cmd.SubjectID)
		return
	}
	c.JSON(200, receipt)
}
func (h *Handler) readback(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	q, _, ok := h.authenticate(c, false)
	if !ok {
		return
	}
	actor, scope := Actor(q, h.Service.Spec.ReadCapability)
	receipt, err := h.Service.Readback(ctx, actor, q.Purpose, scope, c.Param("kind"), c.Param("id"), c.Param("key"))
	if err != nil {
		h.problem(c, err, q, c.Param("id"))
		return
	}
	c.JSON(200, receipt)
}
func (h *Handler) history(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	q, _, ok := h.authenticate(c, false)
	if !ok {
		return
	}
	actor, scope := Actor(q, h.Service.Spec.ReadCapability)
	out, err := h.Service.History(ctx, actor, q.Purpose, scope, c.Param("kind"), c.Param("id"))
	if err != nil {
		h.problem(c, err, q, c.Param("id"))
		return
	}
	c.JSON(200, out)
}

func secretFile(path string) ([]byte, error) {
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

// Runtime composes the owner's command boundary from the already initialized
// source-owner context/policy handler. Both switches default off; with both
// off no key, policy or schema is touched. Keys must differ from every other
// Admin credential the owner holds.
func Runtime(owner *adminowner.Handler, spec Spec, executor Executor, prefix string) (*Handler, error) {
	write := os.Getenv(prefix+"_ADMIN_COMMAND_WRITES_ENABLED") == "true"
	read := os.Getenv(prefix+"_ADMIN_COMMAND_READBACK_ENABLED") == "true"
	h := &Handler{Service: &Service{Spec: spec, Executor: executor, WriteEnabled: write, ReadEnabled: read}}
	if !write && !read {
		return h, nil
	}
	if write && !read {
		// An unreadable write could never be reconciled after an ambiguous outcome.
		return nil, ErrConfiguration
	}
	if owner == nil || !owner.Enabled || owner.DB == nil || owner.Contexts == nil || len(owner.Contexts.Authority.Keys) == 0 || owner.Owner != spec.Owner || executor == nil {
		return nil, ErrConfiguration
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var ready bool
	if err := owner.DB.QueryRow(ctx, `SELECT to_regclass('admin_owner_command_ledger') IS NOT NULL`).Scan(&ready); err != nil || !ready {
		return nil, ErrConfiguration
	}
	h.Service.DB, h.Service.Contexts, h.Service.Policy = owner.DB, owner.Contexts, owner.Policy
	others := [][]byte{owner.Secret, owner.PublishSecret}
	if owner.Cases != nil {
		others = append(others, owner.Cases.Secret)
	}
	var err error
	if write {
		if h.WriteSecret, err = secretFile(os.Getenv(prefix + "_ADMIN_COMMAND_WRITE_SECRET_FILE")); err != nil {
			return nil, err
		}
		others = append(others, h.WriteSecret)
	}
	if h.ReadSecret, err = secretFile(os.Getenv(prefix + "_ADMIN_COMMAND_READBACK_SECRET_FILE")); err != nil {
		return nil, err
	}
	for _, other := range others {
		if len(other) > 0 && bytes.Equal(other, h.ReadSecret) {
			return nil, ErrConfiguration
		}
	}
	for _, other := range others[:len(others)-1] {
		if write && len(other) > 0 && bytes.Equal(other, h.WriteSecret) {
			return nil, ErrConfiguration
		}
	}
	return h, nil
}
