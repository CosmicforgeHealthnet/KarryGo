package usecases

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"

	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	"cosmicforge/logistics/services/media-file-service/internal/platform/audit"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const CaseContentPermission = adminmedia.Permission
const CaseContentContract = adminmedia.Contract

// CaseActor must come from the existing authenticated staff boundary, never
// from an end-user JSON body. The private transport accepts metadata only
// from its separately credentialed, trusted Bridge workload.
type CaseActor struct {
	Delegation adminpolicy.Actor
	ExpiresAt  time.Time
}
type CaseRequest = adminmedia.Request

// CaseProof comes from a live authoritative case adapter. Caller-supplied
// object IDs, tenant labels and legacy media_assets references are not proof.
type CaseProof struct {
	Facts                                                                    adminpolicy.Facts
	CaseKind, SubjectType, SubjectReference, MediaObjectID                   string
	ObjectOwnerService, ObjectOwnerType, ObjectOwnerReference, ObjectPurpose string
	BindingType, BindingID, NativeFingerprint                                string
	ExpiresAt                                                                time.Time
}
type CaseAuthority interface {
	AuthorizeCase(context.Context, CaseActor, CaseRequest) (CaseProof, error)
}
type CaseAccessConfig struct {
	Enabled   bool
	Policy    *adminpolicy.Engine
	Authority CaseAuthority
}
type CaseAccessService struct {
	media  *Service
	config CaseAccessConfig
}
type CaseGrant = adminmedia.Grant

// NewCaseAccess uses a separate private transport and credential audience.
// Concrete production source adapters and approved policy remain pending.
// Default configuration denies before reading the ledger or any object bytes.
func (s *Service) NewCaseAccess(config CaseAccessConfig) *CaseAccessService {
	return &CaseAccessService{media: s, config: config}
}
func (s *CaseAccessService) available() bool {
	return s != nil && s.media != nil && s.config.Enabled && s.config.Policy != nil && s.config.Authority != nil && s.media.opts.Pool != nil
}
func validCaseHash(v string) bool { b, e := hex.DecodeString(v); return e == nil && len(b) == 32 }
func caseTokenHash(id, token string) string {
	sum := sha256.Sum256([]byte("media-admin-case-v1\x00" + id + "\x00" + token))
	return hex.EncodeToString(sum[:])
}
func (s *CaseAccessService) assess(ctx context.Context, actor CaseActor, q CaseRequest) (CaseProof, time.Time, error) {
	var empty CaseProof
	if !s.available() {
		return empty, time.Time{}, adminpolicy.ErrBlocked
	}
	now := s.media.opts.Now().UTC()
	if !actor.ExpiresAt.After(now) || !adminread.Opaque(actor.Delegation.Staff) {
		return empty, time.Time{}, adminpolicy.ErrDenied
	}
	for _, v := range []string{q.Purpose, q.SourceCapability, q.CaseOwner, q.CaseKind, q.CaseReference, q.SubjectType, q.SubjectReference, q.MediaObjectID} {
		if !adminread.Opaque(v) {
			return empty, time.Time{}, adminpolicy.ErrDenied
		}
	}
	policy, err := s.config.Policy.MediaDisclosure(now)
	if err != nil {
		return empty, time.Time{}, err
	}
	// Persist a monotonic accepted policy revision even when this request is
	// denied later. A restart/rollback cannot revive grants under older rules.
	var accepted int64
	err = s.media.opts.Pool.QueryRow(ctx, `INSERT INTO media_admin_case_policy_heads(policy_id,version,checksum) VALUES($1,$2,$3) ON CONFLICT(policy_id) DO UPDATE SET version=excluded.version,checksum=excluded.checksum WHERE media_admin_case_policy_heads.version<excluded.version OR (media_admin_case_policy_heads.version=excluded.version AND media_admin_case_policy_heads.checksum=excluded.checksum) RETURNING version`, s.config.Policy.ID(), s.config.Policy.Version(), s.config.Policy.Checksum()).Scan(&accepted)
	if err != nil {
		return empty, time.Time{}, adminpolicy.ErrBlocked
	}

	allowed := false
	for _, capability := range policy.SourceCapabilities {
		allowed = allowed || q.SourceCapability == capability
	}
	if !allowed {
		return empty, time.Time{}, adminpolicy.ErrDenied
	}
	rule, err := s.config.Policy.Rule(CaseContentPermission, now)
	if err != nil || rule.Owner != "media-file-service" || rule.SourceContract != CaseContentContract {
		return empty, time.Time{}, adminpolicy.ErrBlocked
	}
	if err := s.config.Policy.AuthorizeDelegation(actor.Delegation, CaseContentPermission, q.Purpose, q.Scope, now); err != nil {
		return empty, time.Time{}, err
	}
	if err := s.config.Policy.AuthorizeDelegation(actor.Delegation, q.SourceCapability, q.Purpose, q.Scope, now); err != nil {
		return empty, time.Time{}, err
	}
	proof, err := s.config.Authority.AuthorizeCase(ctx, actor, q)
	if err != nil {
		return empty, time.Time{}, err
	}
	if proof.Facts.Owner != q.CaseOwner || proof.Facts.Reference != q.CaseReference || proof.CaseKind != q.CaseKind || proof.SubjectType != q.SubjectType || proof.SubjectReference != q.SubjectReference || proof.MediaObjectID != q.MediaObjectID || !validCaseHash(proof.NativeFingerprint) || !proof.ExpiresAt.After(now) {
		return empty, time.Time{}, adminpolicy.ErrDenied
	}
	for _, v := range []string{proof.ObjectOwnerService, proof.ObjectOwnerType, proof.ObjectOwnerReference, proof.ObjectPurpose, proof.BindingType, proof.BindingID} {
		if !adminread.Opaque(v) {
			return empty, time.Time{}, adminpolicy.ErrDenied
		}
	}
	if err := s.config.Policy.AuthorizeOwner(actor.Delegation, q.SourceCapability, q.Purpose, q.Scope, proof.Facts, now); err != nil {
		return empty, time.Time{}, err
	}
	expires := now.Add(time.Duration(policy.MaxGrantSeconds) * time.Second)
	if actor.ExpiresAt.Before(expires) {
		expires = actor.ExpiresAt
	}
	if proof.ExpiresAt.Before(expires) {
		expires = proof.ExpiresAt
	}
	return proof, expires, nil
}
func (s *CaseAccessService) object(ctx context.Context, tx pgx.Tx, q CaseRequest, p CaseProof) (*models.Object, error) {
	object, err := s.media.opts.Objects.LockObject(ctx, tx, q.MediaObjectID)
	if err != nil {
		return nil, ErrNotFound
	}
	if object.OrganizationID != q.Scope.Organization || object.OwnerService != p.ObjectOwnerService || object.OwnerType != p.ObjectOwnerType || object.OwnerReference != p.ObjectOwnerReference || object.UploadPurpose != p.ObjectPurpose {
		return nil, ErrNotFound
	}
	object.SecondaryBindings, err = s.media.opts.Objects.SecondaryBindings(ctx, tx, object.ID)
	if err != nil {
		return nil, err
	}
	if !object.BoundTo(p.BindingType, p.BindingID) {
		return nil, ErrNotFound
	}
	// Administrative case access always requires positively SAFE modern objects.
	// It cannot lower native safety, deletion, revocation or retention boundaries.
	if object.Visibility != models.VisibilityPrivate || object.EffectiveStatus() != models.StatusActive || !object.Downloadable() || !object.SafetyConfirmed() || object.AccessExpired(s.media.opts.Now().UTC()) {
		return nil, ErrNotDownloadable
	}
	return object, nil
}
func (s *CaseAccessService) record(ctx context.Context, tx pgx.Tx, actor CaseActor, q CaseRequest, id, action string) error {
	return s.media.opts.Audit.Record(ctx, tx, audit.Entry{OrganizationID: q.Scope.Organization, ActorType: "STAFF", ActorID: actor.Delegation.Staff, Action: action, ResourceType: "media_admin_case_grant", ResourceID: id, NewValues: map[string]any{"mediaObjectId": q.MediaObjectID, "caseOwner": q.CaseOwner, "caseKind": q.CaseKind, "caseReference": q.CaseReference, "purpose": q.Purpose}})
}
func (s *CaseAccessService) Issue(ctx context.Context, actor CaseActor, q CaseRequest) (*CaseGrant, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	proof, expires, err := s.assess(ctx, actor, q)
	if err != nil {
		return nil, err
	}
	tx, err := s.media.opts.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	object, err := s.object(ctx, tx, q, proof)
	if err != nil {
		return nil, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	id, token := uuid.NewString(), base64.RawURLEncoding.EncodeToString(raw)
	request, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO media_admin_case_grants(id,token_hash,media_object_id,staff_ref,request,binding_hash,policy_id,policy_version,policy_checksum,object_version,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, caseTokenHash(id, token), object.ID, actor.Delegation.Staff, request, admincontext.Fingerprint(proof), s.config.Policy.ID(), s.config.Policy.Version(), s.config.Policy.Checksum(), object.Version, s.media.opts.Now().UTC(), expires); err != nil {
		return nil, err
	}
	if err := s.record(ctx, tx, actor, q, id, "media.admin_case.authorized"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &CaseGrant{ContractVersion: CaseContentContract, ID: id, Token: token, MediaObjectID: object.ID, ExpiresAt: expires}, nil
}
func (s *CaseAccessService) Read(ctx context.Context, actor CaseActor, q CaseRequest, id, token string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	proof, _, err := s.assess(ctx, actor, q)
	if err != nil {
		return nil, err
	}
	if parsed, err := uuid.Parse(id); err != nil || parsed.String() != id || len(token) != 43 {
		return nil, ErrNotFound
	}
	tx, err := s.media.opts.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var hash, staff, binding, policyID, checksum string
	var request []byte
	var version, objectVersion int64
	var expires time.Time
	var revoked bool
	err = tx.QueryRow(ctx, `SELECT g.token_hash,g.staff_ref,g.request,g.binding_hash,g.policy_id,g.policy_version,g.policy_checksum,g.object_version,g.expires_at,EXISTS(SELECT 1 FROM media_admin_case_revocations r WHERE r.grant_id=g.id) FROM media_admin_case_grants g WHERE g.id=$1 FOR SHARE OF g`, id).Scan(&hash, &staff, &request, &binding, &policyID, &version, &checksum, &objectVersion, &expires, &revoked)
	if err != nil {
		return nil, ErrNotFound
	}
	var original CaseRequest
	if json.Unmarshal(request, &original) != nil || original != q || staff != actor.Delegation.Staff || !hmac.Equal([]byte(hash), []byte(caseTokenHash(id, token))) || revoked || !expires.After(s.media.opts.Now().UTC()) || binding != admincontext.Fingerprint(proof) || policyID != s.config.Policy.ID() || version != s.config.Policy.Version() || checksum != s.config.Policy.Checksum() {
		return nil, ErrNotFound
	}
	object, err := s.object(ctx, tx, q, proof)
	if err != nil {
		return nil, err
	}
	if object.Version != objectVersion {
		return nil, ErrNotFound
	}
	content, err := s.media.opts.Storage.Get(ctx, object.StorageObjectKey)
	if err != nil {
		return nil, ErrUnavailable
	}
	if !expires.After(s.media.opts.Now().UTC()) || !actor.ExpiresAt.After(s.media.opts.Now().UTC()) || !proof.ExpiresAt.After(s.media.opts.Now().UTC()) {
		return nil, ErrNotFound
	}
	if err := s.record(ctx, tx, actor, q, id, "media.admin_case.redeemed"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return content, nil
}

// Revocation can only reduce an authenticated staff member's own grant. It
// stays possible after source-case closure or a policy/read kill switch.
func (s *CaseAccessService) Revoke(ctx context.Context, actor CaseActor, id, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if s == nil || s.media == nil || s.media.opts.Pool == nil {
		return adminpolicy.ErrBlocked
	}
	if actor.Delegation.Product != "karryam" || !adminread.Opaque(actor.Delegation.Staff) || !actor.ExpiresAt.After(s.media.opts.Now().UTC()) || !adminread.Opaque(reason) {
		return adminpolicy.ErrDenied
	}
	tx, err := s.media.opts.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var staff string
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT staff_ref,request FROM media_admin_case_grants WHERE id=$1 FOR UPDATE`, id).Scan(&staff, &raw); err != nil || staff != actor.Delegation.Staff {
		return ErrNotFound
	}
	var q CaseRequest
	if json.Unmarshal(raw, &q) != nil {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `INSERT INTO media_admin_case_revocations(grant_id,staff_ref,reason_code) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, staff, reason); err != nil {
		return err
	}
	if err := s.record(ctx, tx, actor, q, id, "media.admin_case.revoked"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
