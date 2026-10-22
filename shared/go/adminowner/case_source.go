package adminowner

import (
	"bytes"
	"context"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicyenv"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// CaseSourceReader keeps case-evidence grants in a distinct signed namespace.
// Ordinary directory/read allocations cannot silently become evidence grants.
type CaseSourceReader struct {
	Owner  string
	Source Source
}

func (r CaseSourceReader) Binding(ctx context.Context, tx pgx.Tx, kind, ref string) (admincontext.RecordBinding, error) {
	if !strings.HasPrefix(kind, admincase.ContextPrefix) {
		return admincontext.RecordBinding{}, admincontext.ErrUnmapped
	}
	native := strings.TrimPrefix(kind, admincase.ContextPrefix)
	spec, ok := admincase.Find(r.Owner, native)
	if !ok || r.Source == nil {
		return admincontext.RecordBinding{}, admincontext.ErrUnmapped
	}
	binding, err := r.Source.Binding(ctx, tx, native, ref)
	if err != nil {
		return admincontext.RecordBinding{}, err
	}
	item, err := r.Source.Read(ctx, tx, native, ref, false)
	if err != nil || item.Status == nil {
		return admincontext.RecordBinding{}, admincontext.ErrUnmapped
	}
	binding.SourceContract = spec.SourceContract
	binding.Fingerprint = admincontext.Fingerprint(struct {
		Native   admincontext.RecordBinding
		Subjects []Reference
	}{binding, item.References})
	return binding, nil
}

type CaseAuthorityHandler struct {
	Owner, Path string
	DB          *pgxpool.Pool
	Source      Source
	Contexts    *admincontext.Store
	Policy      *adminpolicy.Engine
	Secret      []byte
	Enabled     bool
	Publisher   *Handler
}

func (h *CaseAuthorityHandler) RegisterOn(group *gin.RouterGroup) {
	prefix := strings.TrimPrefix(h.Path, strings.TrimRight(group.BasePath(), "/"))
	group.GET(prefix+"/proofs/:kind/:id/:subject/:reference", h.Serve)
	if h.Publisher != nil {
		group.POST(prefix+"/authorization-contexts", h.Publisher.PublishContext)
		group.GET(prefix+"/authorization-contexts/bindings/:kind/:id", h.Publisher.PrepareContext)
	}
}
func (h *CaseAuthorityHandler) Serve(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if !h.Enabled || h.DB == nil || h.Source == nil || h.Contexts == nil || len(h.Secret) < 32 || h.Policy == nil {
		c.JSON(503, gin.H{"error": "case_authority_disabled"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	r := c.Request
	fail := func(status int, code string) { (&Handler{Owner: h.Owner, DB: h.DB}).fail(c, status, code) }
	if r.URL.RawPath != "" || len(r.URL.RequestURI()) > 8192 || strings.ContainsAny(r.URL.Path, "%\\") || r.ContentLength != 0 || len(r.TransferEncoding) > 0 || r.Body != nil && r.Body != http.NoBody {
		fail(400, "invalid_request")
		return
	}
	if r.Body == nil {
		r.Body = http.NoBody
	}
	for _, header := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderNonce, serviceauth.HeaderTimestamp, serviceauth.HeaderSignature} {
		if len(r.Header.Values(header)) != 1 {
			fail(401, "unauthorized")
			return
		}
	}
	if _, err := serviceauth.NewVerifier(serviceauth.Secrets{admincase.Caller: h.Secret}, 30*time.Second).VerifyRequest(r); err != nil {
		fail(401, "unauthorized")
		return
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !adminread.Opaque(nonce) {
		fail(401, "unauthorized")
		return
	}
	sum := sha256.Sum256([]byte(h.Owner + ":case-proof:" + nonce))
	tag, err := h.DB.Exec(ctx, `INSERT INTO admin_owner_read_nonces(owner,nonce_hash,expires_at)VALUES($1,$2,now()+interval '11 minutes')ON CONFLICT DO NOTHING`, h.Owner, hex.EncodeToString(sum[:]))
	if err != nil {
		fail(503, "replay_protection_unavailable")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(409, "replay_rejected")
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		fail(400, "invalid_query")
		return
	}
	q, err := adminread.Parse(values, time.Now())
	if err != nil || q.Limit != 1 || q.Cursor != "" {
		fail(400, "invalid_query")
		return
	}
	c.Set("admin_owner_verified_query", q)
	spec, ok := admincase.Find(h.Owner, c.Param("kind"))
	if !ok {
		fail(404, "not_found")
		return
	}
	ref, subject, subjectRef := c.Param("id"), c.Param("subject"), c.Param("reference")
	if !adminread.Opaque(ref) || !adminread.Opaque(subjectRef) || len(spec.Subjects[subject]) == 0 {
		fail(400, "invalid_subject")
		return
	}
	if q.PolicyID != h.Policy.ID() || q.PolicyVersion != h.Policy.Version() || q.PolicyChecksum != h.Policy.Checksum() {
		fail(503, "policy_revision_mismatch")
		return
	}
	now := time.Now().UTC()
	scope := adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
	actor := adminpolicy.Actor{Product: "karryam", Staff: q.Staff, Roles: []string{q.Role}, Permissions: []string{spec.Permission}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}}
	if err := h.Policy.AuthorizeDelegation(actor, spec.Permission, q.Purpose, scope, now); err != nil {
		fail(403, "forbidden")
		return
	}
	if err := adminpolicy.PinRevision(ctx, h.DB, "admin_owner_policy_revisions", h.Policy); err != nil {
		fail(503, "blocked_policy")
		return
	}
	tx, err := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		fail(503, "source_unavailable")
		return
	}
	defer tx.Rollback(ctx)
	facts, expiry, err := h.Contexts.ResolveWithExpiry(ctx, tx, admincase.ContextPrefix+spec.Kind, ref, spec.Permission, scope, now)
	if err != nil || facts.SourceContract != spec.SourceContract {
		fail(404, "case_unavailable")
		return
	}
	if err := h.Policy.AuthorizeOwner(actor, spec.Permission, q.Purpose, scope, facts, now); err != nil {
		fail(403, "forbidden")
		return
	}
	item, err := h.Source.Read(ctx, tx, spec.Kind, ref, false)
	if err != nil || item.Status == nil || !admincase.Contains(spec.States, *item.Status) {
		fail(404, "case_unavailable")
		return
	}
	member := false
	for _, role := range spec.Subjects[subject] {
		if role == "$CASE" && subjectRef == ref {
			member = true
		}
		for _, link := range item.References {
			if link.Kind == role && link.Reference == subjectRef {
				member = true
			}
		}
	}
	if !member {
		fail(404, "subject_not_in_case")
		return
	}
	if h.Policy.ExpiresAt().Before(expiry) {
		expiry = h.Policy.ExpiresAt()
	}
	proof := admincase.Proof{ContractVersion: admincase.Contract, Owner: h.Owner, CaseKind: spec.Kind, CaseReference: ref, SubjectType: subject, SubjectReference: subjectRef, Scope: scope, SourceContract: spec.SourceContract, NativeFingerprint: admincontext.Fingerprint(item), ExpiresAt: expiry, PolicyID: q.PolicyID, PolicyVersion: q.PolicyVersion, PolicyChecksum: q.PolicyChecksum}
	_, err = tx.Exec(ctx, `INSERT INTO admin_owner_read_audit(owner,resource_kind,target_ref,staff_ref,purpose_code,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,as_of,outcome)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now(),'CASE_SUBJECT_AUTHORIZED')`, h.Owner, admincase.ContextPrefix+spec.Kind, ref, q.Staff, q.Purpose, scope.Market, scope.Zone, scope.Organization, scope.Tenant, q.PolicyID, q.PolicyVersion, q.PolicyChecksum)
	if err != nil {
		fail(503, "audit_unavailable")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		fail(503, "audit_unavailable")
		return
	}
	c.JSON(200, proof)
}
func caseFromEnvironment(ctx context.Context, db *pgxpool.Pool, source Source, owner, prefix, path string) (*CaseAuthorityHandler, error) {
	read, publish := os.Getenv(prefix+"_ADMIN_CASE_SOURCE_READ_ENABLED") == "true", os.Getenv(prefix+"_ADMIN_CASE_CONTEXT_PUBLISH_ENABLED") == "true"
	if !read && !publish {
		return nil, nil
	}
	definitions := map[string]Definition{}
	for _, spec := range admincase.Specs() {
		if spec.Owner == owner {
			kind := admincase.ContextPrefix + spec.Kind
			definitions[kind] = Definition{Kind: kind, Capability: spec.Permission, SourceContract: spec.SourceContract}
		}
	}
	if db == nil || source == nil || len(definitions) == 0 {
		return nil, ErrConfiguration
	}
	var ready bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('admin_owner_context_heads')IS NOT NULL AND to_regclass('admin_owner_context_history')IS NOT NULL AND to_regclass('admin_owner_read_nonces')IS NOT NULL AND to_regclass('admin_owner_read_audit')IS NOT NULL AND to_regclass('admin_owner_policy_revisions')IS NOT NULL AND to_regclass('admin_owner_context_preparation_audit')IS NOT NULL`).Scan(&ready); err != nil || !ready {
		return nil, ErrConfiguration
	}
	keys, err := ownerKeys(os.Getenv(prefix + "_ADMIN_CONTEXT_KEYS_FILE"))
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, ErrConfiguration
	}
	reader := CaseSourceReader{Owner: owner, Source: source}
	store := &admincontext.Store{DB: db, Reader: reader, Authority: admincontext.Authority{Owner: owner, Environment: adminpolicyenv.Environment, Keys: keys}}
	h := &CaseAuthorityHandler{Owner: owner, Path: path + "/case-authority", DB: db, Source: source, Contexts: store, Enabled: read}
	if read {
		h.Secret, err = secretFile(os.Getenv(prefix + "_ADMIN_CASE_SOURCE_SECRET_FILE"))
		if err != nil {
			return nil, err
		}
		h.Policy, err = adminpolicy.LoadFiles(os.Getenv(prefix+"_ADMIN_ACCESS_POLICY_FILE"), os.Getenv(prefix+"_ADMIN_ACCESS_POLICY_KEYS_FILE"), adminpolicyenv.Environment, time.Now())
		if err != nil && !errors.Is(err, adminpolicy.ErrBlocked) {
			return nil, ErrConfiguration
		}
	}
	h.Publisher = &Handler{Owner: owner, Path: h.Path, DB: db, PreparationReader: reader, Contexts: store, Definitions: definitions, PublishEnabled: publish}
	if publish {
		h.Publisher.PublishSecret, err = secretFile(os.Getenv(prefix + "_ADMIN_CASE_CONTEXT_PUBLISH_SECRET_FILE"))
		if err != nil {
			return nil, err
		}
		if bytes.Equal(h.Publisher.PublishSecret, h.Secret) {
			return nil, ErrConfiguration
		}
	}
	return h, nil
}
