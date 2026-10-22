package adminprivacy

// Owner coverage manifests: ingestion and verification.
//
// A manifest is the owner's signed statement of the complete set of
// individual facts it published for one fact family, five-dimension scope and
// window (see shared/go/analyticsadmin/coverage.go). Ingestion verifies the
// signature with the owner's pinned authority and the digest over the members,
// then records a monotonic head plus immutable history/members. Ingestion
// never declares coverage: VerifyCoverage re-checks the signature, digest,
// tiling and every retained native fact on every report snapshot.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCoverage = errors.New("owner coverage incomplete or unverifiable")
var errStaleManifest = errors.New("stale or conflicting manifest revision")

// ManifestStore imports owner manifests into the Analytics database.
type ManifestStore struct {
	DB        *pgxpool.Pool
	Authority admincontext.Authority
}

// Import verifies and records one manifest. A lower revision, or the same
// revision with a different checksum, is refused (replay/stale fencing).
func (s *ManifestStore) Import(ctx context.Context, raw []byte, now time.Time) error {
	if s == nil || s.DB == nil || len(s.Authority.Keys) == 0 {
		return ErrUnmapped
	}
	manifest, err := analyticsadmin.VerifyManifest(s.Authority, raw, now)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	a := manifest.Approval
	var current int64
	err = tx.QueryRow(ctx, `INSERT INTO analytics_admin_coverage_heads(owner,context_id,revision,checksum,family,product,market,zone,organization_ref,tenant_ref,window_start,window_end,digest,member_count,state)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
 ON CONFLICT(owner,context_id) DO UPDATE SET revision=excluded.revision,checksum=excluded.checksum,digest=excluded.digest,member_count=excluded.member_count,state=excluded.state,recorded_at=now()
 WHERE analytics_admin_coverage_heads.family=excluded.family AND analytics_admin_coverage_heads.window_start=excluded.window_start AND analytics_admin_coverage_heads.window_end=excluded.window_end
 AND analytics_admin_coverage_heads.product=excluded.product AND analytics_admin_coverage_heads.market=excluded.market AND analytics_admin_coverage_heads.zone=excluded.zone AND analytics_admin_coverage_heads.organization_ref=excluded.organization_ref AND analytics_admin_coverage_heads.tenant_ref=excluded.tenant_ref
 AND (analytics_admin_coverage_heads.revision<excluded.revision OR (analytics_admin_coverage_heads.revision=excluded.revision AND analytics_admin_coverage_heads.checksum=excluded.checksum))
 RETURNING revision`, s.Authority.Owner, a.ID, a.Revision, manifest.Checksum, manifest.Family, a.Scope.Product, a.Scope.Market, a.Scope.Zone, a.Scope.Organization, a.Scope.Tenant, manifest.Start, manifest.End, a.BindingFingerprint, len(manifest.Members), a.State).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return errStaleManifest
	}
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO analytics_admin_coverage_history(owner,context_id,revision,checksum,envelope) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, s.Authority.Owner, a.ID, a.Revision, manifest.Checksum, raw)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		batch := &pgx.Batch{}
		for _, m := range manifest.Members {
			batch.Queue(`INSERT INTO analytics_admin_coverage_members(owner,context_id,revision,event_id,event_name,entity_type,entity_id,source_version,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, s.Authority.Owner, a.ID, a.Revision, m.EventID, m.EventName, m.EntityType, m.EntityID, m.Version, m.OccurredAt)
		}
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CoverageHandler is the fixed per-owner manifest publication route. It reuses
// the owner's existing publisher workload credential and durable nonce store.
type CoverageHandler struct {
	Owner   string
	DB      *pgxpool.Pool
	Secret  []byte
	Store   *ManifestStore
	Enabled bool
}

func (h *CoverageHandler) Publish(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	fail := func(status int, code string) { c.JSON(status, gin.H{"error": gin.H{"code": code}}) }
	if h == nil || !h.Enabled || h.DB == nil || h.Store == nil || len(h.Secret) < 32 {
		fail(503, "coverage_publication_disabled")
		return
	}
	r := c.Request
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" || r.URL.RawPath != "" || r.URL.RawQuery != "" || strings.ContainsAny(r.URL.Path, "%\\") || r.URL.Path != analyticsadmin.CoverageManifestPath+h.Owner {
		fail(400, "invalid_request")
		return
	}
	for _, header := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderNonce, serviceauth.HeaderTimestamp, serviceauth.HeaderSignature} {
		if len(r.Header.Values(header)) != 1 {
			fail(401, "unauthorized")
			return
		}
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, analyticsadmin.MaxManifestBodyBytes+1))
	if err != nil || len(raw) > analyticsadmin.MaxManifestBodyBytes {
		fail(413, "request_too_large")
		return
	}
	r.Body = io.NopCloser(strings.NewReader(string(raw)))
	if _, err := serviceauth.NewVerifier(serviceauth.Secrets{analyticsadmin.ProducerPublisherCaller: h.Secret}, 30*time.Second).VerifyRequest(r); err != nil {
		fail(401, "unauthorized")
		return
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !adminread.Opaque(nonce) {
		fail(401, "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	sum := sha256.Sum256([]byte(h.Owner + ":coverage-manifest:" + nonce))
	tag, err := h.DB.Exec(ctx, `INSERT INTO analytics_admin_coverage_nonces(owner,nonce_hash,expires_at) VALUES($1,$2,now()+interval '11 minutes') ON CONFLICT DO NOTHING`, h.Owner, hex.EncodeToString(sum[:]))
	if err != nil {
		fail(503, "replay_protection_unavailable")
		return
	}
	if tag.RowsAffected() != 1 {
		fail(409, "replay_rejected")
		return
	}
	if err := h.Store.Import(ctx, raw, time.Now()); err != nil {
		switch {
		case errors.Is(err, errStaleManifest):
			fail(409, "stale_manifest_revision")
		case errors.Is(err, analyticsadmin.ErrManifest), errors.Is(err, ErrUnmapped):
			fail(403, "source_owner_manifest_rejected")
		default:
			fail(503, "manifest_store_unavailable")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "RECORDED"})
}

// manifestRef pins exactly which signed manifest revision a release used.
type manifestRef struct {
	ContextID string `json:"context_id"`
	Revision  int64  `json:"revision"`
	Checksum  string `json:"checksum"`
}

// coveredFact is a retained native fact proven present by a verified manifest.
type coveredFact struct {
	EventID    string
	OccurredAt time.Time
}

// coverageProof is the verified, exactly-tiled manifest set for one family.
type coverageProof struct {
	Family    string
	Global    bool
	Manifests []manifestRef
	Facts     []coveredFact
	AsOf      time.Time
}

// VerifyCoverage proves that the owner's verified manifests for one family
// exactly tile [start,end) for scope (or, for global families, the global
// scope) and that every member inside the window is a retained PROCESSED fact
// with the identical owner, name, entity, version and time. Any gap, overlap,
// revocation, expiry, digest mismatch or missing/changed fact is ErrCoverage.
func VerifyCoverage(ctx context.Context, tx pgx.Tx, authority admincontext.Authority, family string, scope adminpolicy.Scope, start, end, now time.Time) (coverageProof, error) {
	proof := coverageProof{Family: family}
	scoped, global := analyticsadmin.ScopedFamily(family)
	if scoped == "" || len(authority.Keys) == 0 || !end.After(start) {
		return proof, ErrCoverage
	}
	proof.Global = global
	if global {
		scope = analyticsadmin.GlobalScope
	}
	rows, err := tx.Query(ctx, `SELECT h.context_id,h.revision,h.checksum,h.window_start,h.window_end,h.digest,h.member_count,h.state,h.recorded_at,v.envelope
 FROM analytics_admin_coverage_heads h JOIN analytics_admin_coverage_history v ON v.owner=h.owner AND v.context_id=h.context_id AND v.revision=h.revision AND v.checksum=h.checksum
 WHERE h.owner=$1 AND h.family=$2 AND h.product=$3 AND h.market=$4 AND h.zone=$5 AND h.organization_ref=$6 AND h.tenant_ref=$7 AND h.window_start<$9 AND h.window_end>$8
 ORDER BY h.window_start LIMIT 1001`, authority.Owner, family, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant, start, end)
	if err != nil {
		return proof, err
	}
	type head struct {
		id, checksum, digest, state string
		revision                    int64
		start, end, recorded        time.Time
		count                       int
		envelope                    []byte
	}
	var heads []head
	for rows.Next() {
		var h head
		if err := rows.Scan(&h.id, &h.revision, &h.checksum, &h.start, &h.end, &h.digest, &h.count, &h.state, &h.recorded, &h.envelope); err != nil {
			rows.Close()
			return proof, err
		}
		heads = append(heads, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return proof, err
	}
	if len(heads) == 0 || len(heads) > 1000 {
		return proof, ErrCoverage
	}
	// Exact tiling: contiguous, no overlap, covering the whole window.
	if heads[0].start.After(start) {
		return proof, ErrCoverage
	}
	for i := 1; i < len(heads); i++ {
		if !heads[i].start.Equal(heads[i-1].end) {
			return proof, ErrCoverage
		}
	}
	if heads[len(heads)-1].end.Before(end) {
		return proof, ErrCoverage
	}
	for _, h := range heads {
		if h.state != "APPROVED" {
			return proof, ErrCoverage
		}
		// Re-verify the signed document on every read: a key rotation,
		// expiry or environment change withdraws the coverage.
		envelopeRaw, err := envelopeOf(h.envelope)
		if err != nil {
			return proof, ErrCoverage
		}
		approval, checksum, err := authority.Verify(envelopeRaw, now)
		if err != nil || checksum != h.checksum || approval.ID != h.id || approval.Revision != h.revision || approval.State != "APPROVED" || approval.BindingFingerprint != h.digest || approval.Scope != scope || approval.ResourceKind != analyticsadmin.CoverageManifestKindPrefix+family || approval.ID != analyticsadmin.ManifestContextID(authority.Owner, family, scope, h.start, h.end) {
			return proof, ErrCoverage
		}
		members, err := loadMembers(ctx, tx, authority.Owner, h.id, h.revision)
		if err != nil {
			return proof, err
		}
		digest, err := analyticsadmin.ManifestDigest(authority.Owner, family, scope, h.start, h.end, members)
		if err != nil || digest != h.digest || len(members) != h.count {
			return proof, ErrCoverage
		}
		proof.Manifests = append(proof.Manifests, manifestRef{ContextID: h.id, Revision: h.revision, Checksum: h.checksum})
		if h.recorded.After(proof.AsOf) {
			proof.AsOf = h.recorded
		}
		inWindow := []analyticsadmin.ManifestMember{}
		for _, m := range members {
			if !m.OccurredAt.Before(start) && m.OccurredAt.Before(end) {
				inWindow = append(inWindow, m)
			}
		}
		if err := verifyRetained(ctx, tx, authority.Owner, inWindow); err != nil {
			return proof, err
		}
		for _, m := range inWindow {
			proof.Facts = append(proof.Facts, coveredFact{EventID: m.EventID, OccurredAt: m.OccurredAt})
		}
	}
	sort.Slice(proof.Facts, func(i, j int) bool { return proof.Facts[i].EventID < proof.Facts[j].EventID })
	return proof, nil
}

func envelopeOf(raw []byte) ([]byte, error) {
	manifest, _, err := analyticsadmin.DecodeManifest(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(manifest.Envelope)
}

func loadMembers(ctx context.Context, tx pgx.Tx, owner, id string, revision int64) ([]analyticsadmin.ManifestMember, error) {
	rows, err := tx.Query(ctx, `SELECT event_id,event_name,entity_type,entity_id,source_version,occurred_at FROM analytics_admin_coverage_members WHERE owner=$1 AND context_id=$2 AND revision=$3 ORDER BY event_id LIMIT $4`, owner, id, revision, analyticsadmin.MaxManifestMembers+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []analyticsadmin.ManifestMember{}
	for rows.Next() {
		var m analyticsadmin.ManifestMember
		if err := rows.Scan(&m.EventID, &m.EventName, &m.EntityType, &m.EntityID, &m.Version, &m.OccurredAt); err != nil {
			return nil, err
		}
		m.OccurredAt = m.OccurredAt.UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// verifyRetained requires every attested member to be retained exactly once
// as a PROCESSED fact with the identical immutable identity. A STALE, IGNORED,
// missing or differently-versioned fact means the owner's attestation and the
// ingested facts disagree, so the report cannot be complete.
func verifyRetained(ctx context.Context, tx pgx.Tx, owner string, members []analyticsadmin.ManifestMember) error {
	if len(members) == 0 {
		return nil
	}
	ids := make([]string, len(members))
	byID := map[string]analyticsadmin.ManifestMember{}
	for i, m := range members {
		ids[i] = m.EventID
		byID[m.EventID] = m
	}
	rows, err := tx.Query(ctx, `SELECT event_id,event_name,source_service,source_entity_type,source_entity_id,source_version,occurred_at,status FROM analytics_network_events WHERE event_id=ANY($1) FOR SHARE`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var id, name, service, entityType, entityID, status string
		var version int64
		var at time.Time
		if err := rows.Scan(&id, &name, &service, &entityType, &entityID, &version, &at, &status); err != nil {
			return err
		}
		m, ok := byID[id]
		canonical, _ := canonicalLineageOwner(service)
		if !ok || status != "PROCESSED" || canonical != owner || name != m.EventName || entityType != m.EntityType || entityID != m.EntityID || version != m.Version || !at.UTC().Truncate(time.Microsecond).Equal(m.OccurredAt) {
			return ErrCoverage
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if seen != len(members) {
		return ErrCoverage
	}
	return nil
}
