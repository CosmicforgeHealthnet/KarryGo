package adminowner

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Owner, Path string
	Cases       *CaseAuthorityHandler
	DB          *pgxpool.Pool
	// PreparationReader supports binding-only producers without exposing resource reads.
	PreparationReader admincontext.BindingReader
	Source            Source
	Contexts          *admincontext.Store
	Policy            *adminpolicy.Engine
	Secret            []byte
	Enabled           bool
	PublishEnabled    bool
	PublishSecret     []byte
	Definitions       map[string]Definition
}

func (h *Handler) Register(root *gin.Engine) {
	h.RegisterOn(root.Group(""))
}

// RegisterOn preserves additional native owner middleware, including Fleet's
// enrolled service-credential boundary, before this adapter's own checks.
func (h *Handler) RegisterOn(group *gin.RouterGroup) {
	base := strings.TrimRight(group.BasePath(), "/")
	if !strings.HasPrefix(h.Path, base+"/") {
		panic("owner admin route outside native group")
	}
	if h.Cases != nil {
		h.Cases.RegisterOn(group)
	}
	prefix := strings.TrimPrefix(h.Path, base)
	group.GET(prefix+"/resources/:kind", h.Serve)
	group.GET(prefix+"/resources/:kind/:id", h.Serve)
	group.POST(prefix+"/authorization-contexts", h.PublishContext)
	group.GET(prefix+"/authorization-contexts/bindings/:kind/:id", h.PrepareContext)
}
func (h *Handler) fail(c *gin.Context, status int, code string) {
	// Only a verified workload's fully parsed query can enter durable owner
	// audit fields. No raw headers, URI, credentials or rejection text is stored.
	if value, ok := c.Get("admin_owner_verified_query"); ok {
		query := value.(adminread.Query)
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
		defer cancel()
		ref := c.Param("id")
		if !adminread.Opaque(ref) {
			ref = ""
		}
		_, err := h.DB.Exec(ctx, `INSERT INTO admin_owner_read_audit(owner,resource_kind,target_ref,staff_ref,purpose_code,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,as_of,outcome) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now(),$13)`, h.Owner, c.Param("kind"), ref, query.Staff, query.Purpose, query.Scope.Market, query.Scope.Zone, query.Scope.Organization, query.Scope.Tenant, query.PolicyID, query.PolicyVersion, query.PolicyChecksum, code)
		if err != nil {
			slog.Error("owner_admin_denial_audit_unavailable", "owner", h.Owner, "outcome", code)
			status = 503
			code = "audit_unavailable"
		}
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code}})
}

func (h *Handler) Serve(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	definition, ok := h.Definitions[c.Param("kind")]
	if !ok {
		h.fail(c, 404, "not_found")
		return
	}
	if !h.Enabled || h.DB == nil || h.Source == nil || h.Contexts == nil || len(h.Secret) < 32 {
		h.fail(c, 503, "owner_read_disabled")
		return
	}
	if len(h.Contexts.Authority.Keys) == 0 {
		h.fail(c, 503, "owner_context_unavailable")
		return
	}
	if _, err := h.Policy.Rule(definition.Capability, time.Now()); err != nil {
		h.fail(c, 503, "blocked_policy")
		return
	}
	r := c.Request
	if r.Method != http.MethodGet || r.URL.RawPath != "" || len(r.URL.RequestURI()) > 8192 || strings.ContainsAny(r.URL.Path, "%\\") || r.ContentLength != 0 || len(r.TransferEncoding) > 0 || r.Body != nil && r.Body != http.NoBody {
		h.fail(c, 400, "invalid_request")
		return
	}
	for _, header := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderNonce, serviceauth.HeaderTimestamp, serviceauth.HeaderSignature} {
		if len(r.Header.Values(header)) != 1 {
			h.fail(c, 401, "unauthorized")
			return
		}
	}
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{adminread.Caller: h.Secret}, 30*time.Second)
	if _, err := verifier.VerifyRequest(r); err != nil {
		h.fail(c, 401, "unauthorized")
		return
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !adminread.Opaque(nonce) {
		h.fail(c, 401, "unauthorized")
		return
	}
	nonceHash := sha256.Sum256([]byte(h.Owner + ":" + nonce))
	tag, err := h.DB.Exec(ctx, `INSERT INTO admin_owner_read_nonces(owner,nonce_hash,expires_at) VALUES($1,$2,now()+interval '11 minutes') ON CONFLICT DO NOTHING`, h.Owner, hex.EncodeToString(nonceHash[:]))
	if err != nil {
		h.fail(c, 503, "replay_protection_unavailable")
		return
	}
	if tag.RowsAffected() != 1 {
		h.fail(c, 409, "replay_rejected")
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(c, 400, "invalid_query")
		return
	}
	query, err := adminread.Parse(values, time.Now())
	if err != nil {
		h.fail(c, 400, "invalid_query")
		return
	}
	c.Set("admin_owner_verified_query", query)
	if query.PolicyID != h.Policy.ID() || query.PolicyVersion != h.Policy.Version() || query.PolicyChecksum != h.Policy.Checksum() {
		h.fail(c, 503, "policy_revision_mismatch")
		return
	}
	ref := c.Param("id")
	if ref != "" && !adminread.Opaque(ref) {
		h.fail(c, 400, "invalid_reference")
		return
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: query.Scope.Market, Zone: query.Scope.Zone, Organization: query.Scope.Organization, Tenant: query.Scope.Tenant}
	actor := adminpolicy.Actor{Product: "karryam", Staff: query.Staff, Roles: []string{query.Role}, Permissions: []string{definition.Capability}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}}
	if err := h.Policy.AuthorizeDelegation(actor, definition.Capability, query.Purpose, scope, time.Now()); err != nil {
		h.fail(c, 403, "forbidden")
		return
	}
	after, err := h.decodeCursor(query, definition)
	if err != nil || ref != "" && query.Cursor != "" {
		h.fail(c, 400, "invalid_cursor")
		return
	}
	tx, err := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		h.fail(c, 503, "owner_unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var asOf time.Time
	if err := tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&asOf); err != nil {
		h.fail(c, 503, "owner_unavailable")
		return
	}
	refs := []string{ref}
	if ref == "" {
		refs, err = h.Contexts.CandidateRefs(ctx, tx, definition.Kind, definition.Capability, scope, after, query.Limit+1)
		if err != nil {
			h.fail(c, 503, "owner_unavailable")
			return
		}
	}
	more := len(refs) > query.Limit
	if more {
		refs = refs[:query.Limit]
	}
	result := Snapshot{ContractVersion: ContractVersion, Owner: h.Owner, Kind: definition.Kind, Scope: scope, PolicyID: h.Policy.ID(), PolicyVersion: h.Policy.Version(), PolicyChecksum: h.Policy.Checksum(), AsOf: asOf, Coverage: "CONTEXT_SCOPED_PARTIAL", Items: []Resource{}}
	authorizedCandidates := 0
	for _, candidate := range refs {
		facts, err := h.Contexts.Resolve(ctx, tx, definition.Kind, candidate, definition.Capability, scope, time.Now())
		if errors.Is(err, admincontext.ErrUnmapped) {
			continue
		}
		if err != nil {
			h.fail(c, 503, "owner_unavailable")
			return
		}
		if facts.SourceContract != definition.SourceContract || h.Policy.AuthorizeOwner(actor, definition.Capability, query.Purpose, scope, facts, time.Now()) != nil {
			h.fail(c, 403, "forbidden")
			return
		}
		if native, ok := h.Source.(StaffAuthorizer); ok {
			if err := native.AuthorizeStaff(ctx, tx, query.Staff, definition.Kind, candidate); errors.Is(err, pgx.ErrNoRows) {
				continue
			} else if err != nil {
				h.fail(c, 503, "owner_authorization_unavailable")
				return
			}
		}
		authorizedCandidates++
		item, err := h.Source.Read(ctx, tx, definition.Kind, candidate, ref != "")
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			h.fail(c, 503, "owner_unavailable")
			return
		}
		if ref == "" && (item.CreatedAt.Before(query.Start) || !item.CreatedAt.Before(query.End)) {
			continue
		}
		if err := ValidateResource(item, definition, ref != ""); err != nil {
			h.fail(c, 503, "owner_contract_invalid")
			return
		}
		result.Items = append(result.Items, item)
	}
	if authorizedCandidates == 0 && ref == "" {
		h.fail(c, 403, "owner_scope_unmapped")
		return
	}
	if ref != "" && len(result.Items) == 0 {
		h.fail(c, 404, "not_found")
		return
	}
	if more && len(refs) > 0 {
		cursor, err := h.encodeCursor(query, definition, refs[len(refs)-1])
		if err != nil {
			h.fail(c, 503, "owner_unavailable")
			return
		}
		result.NextCursor = &cursor
	}
	if _, err := tx.Exec(ctx, `INSERT INTO admin_owner_read_audit(owner,resource_kind,target_ref,staff_ref,purpose_code,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,as_of) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, h.Owner, definition.Kind, ref, query.Staff, query.Purpose, scope.Market, scope.Zone, scope.Organization, scope.Tenant, h.Policy.ID(), h.Policy.Version(), h.Policy.Checksum(), asOf); err != nil {
		h.fail(c, 503, "audit_unavailable")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.fail(c, 503, "audit_unavailable")
		return
	}
	c.JSON(http.StatusOK, result)
}

type cursorPayload struct {
	After   string `json:"after"`
	Expires int64  `json:"expires"`
}

func (h *Handler) cursorCipher(definition Definition) (cipher.AEAD, error) {
	key := sha256.Sum256(append([]byte("karryam-owner-cursor-v1\x00"+h.Owner+"\x00"+definition.Kind+"\x00"), h.Secret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func cursorAAD(q adminread.Query, definition Definition) []byte {
	raw, _ := json.Marshal(struct {
		Query      adminread.Query
		Definition Definition
	}{q, definition})
	return raw
}
func (h *Handler) encodeCursor(q adminread.Query, definition Definition, after string) (string, error) {
	aead, err := h.cursorCipher(definition)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	raw, _ := json.Marshal(cursorPayload{After: after, Expires: time.Now().Add(10 * time.Minute).Unix()})
	sealed := aead.Seal(nonce, nonce, raw, cursorAAD(q, definition))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}
func (h *Handler) decodeCursor(q adminread.Query, definition Definition) (string, error) {
	if q.Cursor == "" {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(q.Cursor)
	if err != nil {
		return "", err
	}
	aead, err := h.cursorCipher(definition)
	if err != nil || len(raw) < aead.NonceSize()+aead.Overhead() {
		return "", adminread.ErrQuery
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], cursorAAD(q, definition))
	if err != nil {
		return "", err
	}
	var payload cursorPayload
	if json.Unmarshal(plain, &payload) != nil || !adminread.Opaque(payload.After) || payload.Expires <= time.Now().Unix() {
		return "", adminread.ErrQuery
	}
	return payload.After, nil
}
