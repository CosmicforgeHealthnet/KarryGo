// Package admincontext records source-owner authorization contexts without
// rewriting business ownership. Governance policy and staff delegation remain
// separate requirements. A query or a policy document cannot create a context.
package admincontext

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SigningDomain = "karryam-owner-authorization-context-v1\n"

var ErrUnmapped = errors.New("owner authorization context unavailable")
var ErrApproval = errors.New("invalid source-owner context approval")

// Approval is a source-owned authorization relationship, not a replacement for
// the business record's organization/settlement/customer ownership. Provenance
// identifies the owner's reviewed remediation/authorization case. No production
// scope, key or approval is supplied by this package.
type Approval struct {
	SchemaVersion      int               `json:"schema_version"`
	ID                 string            `json:"id"`
	Revision           int64             `json:"revision"`
	Owner              string            `json:"owner"`
	Environment        string            `json:"environment"`
	State              string            `json:"state"`
	ResourceKind       string            `json:"resource_kind"`
	ResourceRef        string            `json:"resource_ref"`
	SourceContract     string            `json:"source_contract"`
	BindingFingerprint string            `json:"binding_fingerprint"`
	Scope              adminpolicy.Scope `json:"scope"`
	Capabilities       []string          `json:"capabilities"`
	ProvenanceRef      string            `json:"provenance_ref"`
	ApprovalRef        string            `json:"approval_ref"`
	EffectiveAt        time.Time         `json:"effective_at"`
	ExpiresAt          time.Time         `json:"expires_at"`
}
type Envelope struct {
	Document  json.RawMessage `json:"document"`
	KeyID     string          `json:"key_id"`
	Signature string          `json:"signature"`
}

// RecordBinding comes from actual owner SQL, never from an incoming approval or
// request. Nonempty canonical dimensions must agree with the attestation. The
// fingerprint covers identity and authorization-relevant source links; ordinary
// status/telemetry updates do not require manually remapping an unchanged scope.
type RecordBinding struct {
	Fingerprint    string            `json:"fingerprint"`
	SourceContract string            `json:"source_contract"`
	Canonical      adminpolicy.Scope `json:"known_source_dimensions"`
}
type BindingReader interface {
	Binding(context.Context, pgx.Tx, string, string) (RecordBinding, error)
}
type Authority struct {
	Owner       string
	Environment string
	Keys        map[string]ed25519.PublicKey
}

func Fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func decode(raw []byte, out any) error {
	if len(raw) == 0 || len(raw) > 32<<10 {
		return ErrApproval
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := unique(d); err != nil {
		return ErrApproval
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrApproval
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrApproval
	}
	return nil
}
func unique(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || name != strings.ToLower(name) || seen[name] {
				return ErrApproval
			}
			seen[name] = true
			if err = unique(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := unique(d); err != nil {
				return err
			}
		}
	default:
		return ErrApproval
	}
	_, err = d.Token()
	return err
}
func (a Authority) Verify(raw []byte, now time.Time) (Approval, string, error) {
	var envelope Envelope
	var approval Approval
	if decode(raw, &envelope) != nil || decode(envelope.Document, &approval) != nil {
		return approval, "", ErrApproval
	}
	key := a.Keys[envelope.KeyID]
	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, append([]byte(SigningDomain), envelope.Document...), signature) {
		return approval, "", ErrApproval
	}
	scope := approval.Scope
	if approval.SchemaVersion != 1 || approval.Owner != a.Owner || approval.Environment != a.Environment || (a.Environment != adminpolicy.Production && a.Environment != adminpolicy.TestOnly) || approval.Revision < 1 || (approval.State != "APPROVED" && approval.State != "REVOKED") || !adminread.Opaque(approval.ID) || !adminread.Opaque(approval.ResourceKind) || !adminread.Opaque(approval.ResourceRef) || !adminread.Opaque(approval.SourceContract) || !adminread.Opaque(approval.ApprovalRef) || !adminread.Opaque(approval.ProvenanceRef) || len(approval.BindingFingerprint) != 64 || len(approval.Capabilities) == 0 || len(approval.Capabilities) > 32 || approval.EffectiveAt.IsZero() || approval.ExpiresAt.IsZero() || !approval.ExpiresAt.After(approval.EffectiveAt) || now.Before(approval.EffectiveAt) || !now.Before(approval.ExpiresAt) {
		return approval, "", ErrApproval
	}
	if _, err := hex.DecodeString(approval.BindingFingerprint); err != nil {
		return approval, "", ErrApproval
	}
	for _, dimension := range []string{scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant} {
		if !adminread.Opaque(dimension) {
			return approval, "", ErrApproval
		}
	}
	seen := map[string]bool{}
	for _, capability := range approval.Capabilities {
		if !adminread.Opaque(capability) || seen[capability] {
			return approval, "", ErrApproval
		}
		seen[capability] = true
	}
	sum := sha256.Sum256(envelope.Document)
	return approval, hex.EncodeToString(sum[:]), nil
}

type Store struct {
	DB        *pgxpool.Pool
	Authority Authority
	Reader    BindingReader
}

// Import verifies a separately pinned source-owner signature, live owner record
// and canonical links before atomically recording a monotonic context revision.
// It is an owner-internal ingestion port, never an HQ approval endpoint.
func (s *Store) Import(ctx context.Context, raw []byte, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	approval, checksum, err := s.Authority.Verify(raw, now)
	if err != nil {
		return err
	}
	if s.DB == nil || s.Reader == nil {
		return ErrUnmapped
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if approval.State == "APPROVED" {
		binding, err := s.Reader.Binding(ctx, tx, approval.ResourceKind, approval.ResourceRef)
		if err != nil || !matches(approval, binding) {
			return ErrUnmapped
		}
	}
	var current int64
	err = tx.QueryRow(ctx, `INSERT INTO admin_owner_context_heads(owner,context_id,revision,checksum,resource_kind,resource_ref,product,market,zone,organization_ref,tenant_ref,capabilities) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	 ON CONFLICT(owner,context_id) DO UPDATE SET revision=excluded.revision,checksum=excluded.checksum,product=excluded.product,market=excluded.market,zone=excluded.zone,organization_ref=excluded.organization_ref,tenant_ref=excluded.tenant_ref,capabilities=excluded.capabilities
	 WHERE admin_owner_context_heads.resource_kind=excluded.resource_kind AND admin_owner_context_heads.resource_ref=excluded.resource_ref
	 AND (admin_owner_context_heads.revision<excluded.revision OR (admin_owner_context_heads.revision=excluded.revision AND admin_owner_context_heads.checksum=excluded.checksum)) RETURNING revision`, s.Authority.Owner, approval.ID, approval.Revision, checksum, approval.ResourceKind, approval.ResourceRef, approval.Scope.Product, approval.Scope.Market, approval.Scope.Zone, approval.Scope.Organization, approval.Scope.Tenant, approval.Capabilities).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrApproval
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO admin_owner_context_history(owner,context_id,revision,checksum,envelope) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner,context_id,revision) DO NOTHING`, s.Authority.Owner, approval.ID, approval.Revision, checksum, raw)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func matches(a Approval, b RecordBinding) bool {
	if b.Fingerprint == "" || b.Fingerprint != a.BindingFingerprint || b.SourceContract == "" || b.SourceContract != a.SourceContract {
		return false
	}
	actual := []string{b.Canonical.Product, b.Canonical.Market, b.Canonical.Zone, b.Canonical.Organization, b.Canonical.Tenant}
	requested := []string{a.Scope.Product, a.Scope.Market, a.Scope.Zone, a.Scope.Organization, a.Scope.Tenant}
	for i, value := range actual {
		if value != "" && value != requested[i] {
			return false
		}
	}
	return true
}

// Resolve rechecks the current source links and source-owner signature on every
// disclosure. A revoked head, stale binding or missing record is never authority.
func (s *Store) Resolve(ctx context.Context, tx pgx.Tx, kind, ref, capability string, scope adminpolicy.Scope, now time.Time) (adminpolicy.Facts, error) {
	facts, _, err := s.ResolveWithExpiry(ctx, tx, kind, ref, capability, scope, now)
	return facts, err
}
func (s *Store) ResolveWithExpiry(ctx context.Context, tx pgx.Tx, kind, ref, capability string, scope adminpolicy.Scope, now time.Time) (adminpolicy.Facts, time.Time, error) {
	if s.Reader == nil || tx == nil {
		return adminpolicy.Facts{}, time.Time{}, ErrUnmapped
	}
	rows, err := tx.Query(ctx, `SELECT h.context_id,h.revision,h.checksum,v.envelope FROM admin_owner_context_heads h JOIN admin_owner_context_history v
	 ON v.owner=h.owner AND v.context_id=h.context_id AND v.revision=h.revision AND v.checksum=h.checksum
	 WHERE h.owner=$1 AND h.resource_kind=$2 AND h.resource_ref=$3
	 AND h.revision=(SELECT max(x.revision) FROM admin_owner_context_history x WHERE x.owner=h.owner AND x.context_id=h.context_id)
	 ORDER BY h.context_id LIMIT 33`, s.Authority.Owner, kind, ref)
	if err != nil {
		return adminpolicy.Facts{}, time.Time{}, err
	}
	type candidate struct {
		id, checksum string
		revision     int64
		raw          []byte
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.revision, &item.checksum, &item.raw); err != nil {
			rows.Close()
			return adminpolicy.Facts{}, time.Time{}, err
		}
		candidates = append(candidates, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return adminpolicy.Facts{}, time.Time{}, err
	}
	if len(candidates) > 32 {
		return adminpolicy.Facts{}, time.Time{}, ErrUnmapped
	}
	binding, err := s.Reader.Binding(ctx, tx, kind, ref)
	if err != nil {
		return adminpolicy.Facts{}, time.Time{}, ErrUnmapped
	}
	for _, item := range candidates {
		approval, checksum, err := s.Authority.Verify(item.raw, now)
		if err != nil || approval.ID != item.id || approval.Revision != item.revision || checksum != item.checksum || approval.State != "APPROVED" || approval.ResourceKind != kind || approval.ResourceRef != ref || approval.Scope != scope || !matches(approval, binding) {
			continue
		}
		allowed := false
		for _, value := range approval.Capabilities {
			allowed = allowed || value == capability
		}
		if allowed {
			return adminpolicy.Facts{Owner: s.Authority.Owner, Reference: ref, SourceContract: approval.SourceContract, Scope: approval.Scope}, approval.ExpiresAt, nil
		}
	}
	return adminpolicy.Facts{}, time.Time{}, ErrUnmapped
}
