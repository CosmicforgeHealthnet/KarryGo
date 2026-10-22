package analyticsadmin

// Owner-side coverage producer and supply roster recorder.
//
// These run INSIDE a source owner (Taxi, Dispatch, Hauling). The owner keeps
// its business data; this package only (1) records an individually verifiable
// supply roster from the owner's own availability records, and (2) attests,
// per window, scope and family, exactly which individual facts the owner
// published to Analytics, signed with the owner's own key. Scope comes only
// from the owner's own signed resource authorization contexts carrying
// CoverageExportCapability; a fact whose anchor has none belongs to no scope.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicyenv"
	"cosmicforge/logistics/shared/go/adminread"
	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed producer_schema.sql
var ProducerSchema string

// ProducerSchemaSHA256 lets each owner migration prove it embeds this schema.
func ProducerSchemaSHA256() string {
	sum := sha256.Sum256([]byte(ProducerSchema))
	return hex.EncodeToString(sum[:])
}

// ProducerFact is one fact the owner delivered (or will deliver) to Analytics,
// with the owner resource whose authorization context assigns its scope. An
// empty AnchorKind marks a fact of a declared unattested population
// (UnattestedPopulations): it is counted, never anchored and never attested.
type ProducerFact struct {
	Member     ManifestMember
	Family     string
	AnchorKind string
	AnchorRef  string
}

// FactSource lists the owner's analytics-addressed facts whose occurred_at is
// in [start,end). It must read the owner's own outbox (the durable record of
// what it publishes), never Analytics.
type FactSource interface {
	Facts(ctx context.Context, start, end time.Time) ([]ProducerFact, error)
}

// ScopeResolver maps an owner resource to the scopes its own signed contexts
// authorize for coverage export.
type ScopeResolver interface {
	AnchorScopes(ctx context.Context, kind, ref string, now time.Time) ([]adminpolicy.Scope, error)
	KnownScopes(ctx context.Context, kinds []string) ([]adminpolicy.Scope, error)
}

type ManifestPublisher interface {
	PublishManifest(ctx context.Context, raw []byte) error
}

// ContextScopeResolver resolves anchors through the owner's admincontext
// store, re-verifying every signed context (signature, expiry, binding).
type ContextScopeResolver struct {
	DB    *pgxpool.Pool
	Store *admincontext.Store
}

func (r *ContextScopeResolver) AnchorScopes(ctx context.Context, kind, ref string, now time.Time) ([]adminpolicy.Scope, error) {
	if r == nil || r.DB == nil || r.Store == nil || len(r.Store.Authority.Keys) == 0 {
		return nil, ErrInvalid
	}
	tx, err := r.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT DISTINCT product,market,zone,organization_ref,tenant_ref FROM admin_owner_context_heads WHERE owner=$1 AND resource_kind=$2 AND resource_ref=$3 AND $4=ANY(capabilities) LIMIT 33`, r.Store.Authority.Owner, kind, ref, CoverageExportCapability)
	if err != nil {
		return nil, err
	}
	var candidates []adminpolicy.Scope
	for rows.Next() {
		var s adminpolicy.Scope
		if err := rows.Scan(&s.Product, &s.Market, &s.Zone, &s.Organization, &s.Tenant); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []adminpolicy.Scope{}
	for _, s := range candidates {
		if _, err := r.Store.Resolve(ctx, tx, kind, ref, CoverageExportCapability, s, now); err == nil {
			out = append(out, s)
		} else if !errors.Is(err, admincontext.ErrUnmapped) {
			return nil, err
		}
	}
	return out, nil
}

func (r *ContextScopeResolver) KnownScopes(ctx context.Context, kinds []string) ([]adminpolicy.Scope, error) {
	if r == nil || r.DB == nil || r.Store == nil {
		return nil, ErrInvalid
	}
	rows, err := r.DB.Query(ctx, `SELECT DISTINCT product,market,zone,organization_ref,tenant_ref FROM admin_owner_context_heads WHERE owner=$1 AND resource_kind=ANY($2) AND $3=ANY(capabilities) ORDER BY 1,2,3,4,5 LIMIT 1001`, r.Store.Authority.Owner, kinds, CoverageExportCapability)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []adminpolicy.Scope{}
	for rows.Next() {
		var s adminpolicy.Scope
		if err := rows.Scan(&s.Product, &s.Market, &s.Zone, &s.Organization, &s.Tenant); err != nil {
			return nil, err
		}
		if s.Product == "karryam" {
			out = append(out, s)
		}
	}
	if len(out) > 1000 {
		return nil, ErrInvalid
	}
	return out, rows.Err()
}

// Producer publishes signed coverage manifests for closed windows.
type Producer struct {
	Owner, Environment         string
	DB                         *pgxpool.Pool
	Source                     FactSource
	Resolver                   ScopeResolver
	AnchorKinds                []string
	Signer                     Signer
	Publisher                  ManifestPublisher
	Period, Lookback, Settle   time.Duration
	TTL                        time.Duration
	ProvenanceRef, ApprovalRef string
	// RosterEnabled adds supply-roster manifests, only for windows whose every
	// roster bucket was actually observed by the owner.
	RosterEnabled bool
	// Global attests the owner's complete publication across all scopes
	// (global families). Scope membership is then proven per fact by the
	// owner's existing signed per-fact allocations, verified by Analytics.
	Global bool
	Now    func() time.Time
}

type ProducerResult struct {
	Published, Unchanged, Unscoped, SkippedRoster int
	// Unattested counts facts of declared unattested populations: delivered to
	// Analytics, but with no resource whose context could place them in a scope.
	Unattested int
}

func (p *Producer) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func (p *Producer) validate() error {
	if p == nil || p.DB == nil || p.Source == nil || (p.Resolver == nil && !p.Global) || p.Publisher == nil || len(p.Signer.Key) != ed25519.PrivateKeySize || !adminread.Opaque(p.ProvenanceRef) || !adminread.Opaque(p.ApprovalRef) || p.Period <= 0 || p.Period > MaxManifestWindow || p.Lookback < p.Period || p.Settle < 0 || p.TTL < p.Lookback || p.Environment == "" {
		return ErrInvalid
	}
	return nil
}

type groupKey struct {
	family string
	scope  adminpolicy.Scope
	start  time.Time
}

// RunOnce attests every closed period in the lookback. Unchanged windows are
// skipped; changed ones get a new monotonic revision. An unknown outcome is
// retried with the identical stored body.
func (p *Producer) RunOnce(ctx context.Context) (ProducerResult, error) {
	var result ProducerResult
	if err := p.validate(); err != nil {
		return result, err
	}
	now := p.now()
	end := now.Add(-p.Settle).Truncate(p.Period)
	start := end.Add(-p.Lookback).Truncate(p.Period)
	if !end.After(start) {
		return result, nil
	}
	facts, err := p.Source.Facts(ctx, start, end)
	if err != nil {
		return result, err
	}
	scopes := []adminpolicy.Scope{GlobalScope}
	if !p.Global {
		if scopes, err = p.Resolver.KnownScopes(ctx, p.AnchorKinds); err != nil {
			return result, err
		}
	}
	groups := map[groupKey][]ManifestMember{}
	cache := map[string][]adminpolicy.Scope{}
	for _, fact := range facts {
		if fact.Family != FamilyNetworkEvents && fact.Family != FamilySupplyRoster {
			continue
		}
		if fact.Family == FamilySupplyRoster && !p.RosterEnabled {
			continue
		}
		if fact.AnchorKind == "" && !p.Global {
			if !UnattestedEntityType(p.Owner, fact.Member.EntityType) {
				// Only a declared population may be left out of every scope.
				return result, ErrInvalid
			}
			result.Unattested++
			continue
		}
		key := fact.AnchorKind + "\x00" + fact.AnchorRef
		anchorScopes, ok := cache[key]
		if p.Global {
			anchorScopes, ok = scopes, true
		}
		if !ok {
			if anchorScopes, err = p.Resolver.AnchorScopes(ctx, fact.AnchorKind, fact.AnchorRef, now); err != nil {
				return result, err
			}
			cache[key] = anchorScopes
		}
		if len(anchorScopes) == 0 {
			result.Unscoped++
			continue
		}
		window := fact.Member.OccurredAt.UTC().Truncate(p.Period)
		for _, s := range anchorScopes {
			k := groupKey{p.family(fact.Family), s, window}
			groups[k] = append(groups[k], fact.Member)
		}
	}
	families := []string{p.family(FamilyNetworkEvents)}
	if p.RosterEnabled {
		families = append(families, p.family(FamilySupplyRoster))
	}
	for window := start; window.Before(end); window = window.Add(p.Period) {
		windowEnd := window.Add(p.Period)
		for _, family := range families {
			if scoped, _ := ScopedFamily(family); scoped == FamilySupplyRoster {
				complete, err := p.rosterObserved(ctx, window, windowEnd)
				if err != nil {
					return result, err
				}
				if !complete {
					result.SkippedRoster++
					continue
				}
			}
			for _, scope := range scopes {
				published, err := p.attest(ctx, family, scope, window, windowEnd, groups[groupKey{family, scope, window}], now)
				if err != nil {
					return result, err
				}
				if published {
					result.Published++
				} else {
					result.Unchanged++
				}
			}
		}
	}
	return result, nil
}

func (p *Producer) family(f string) string {
	if !p.Global {
		return f
	}
	if f == FamilySupplyRoster {
		return FamilySupplyRosterGlobal
	}
	return FamilyNetworkEventsGlobal
}

// rosterObserved is true only when every roster bucket in the window has a
// recorded owner observation run.
func (p *Producer) rosterObserved(ctx context.Context, start, end time.Time) (bool, error) {
	var n int
	if err := p.DB.QueryRow(ctx, `SELECT count(*) FROM analytics_supply_roster_runs WHERE bucket_start>=$1 AND bucket_start<$2`, start, end).Scan(&n); err != nil {
		return false, err
	}
	return n == int(end.Sub(start)/networkanalytics.SupplyBucket), nil
}

func (p *Producer) attest(ctx context.Context, family string, scope adminpolicy.Scope, start, end time.Time, members []ManifestMember, now time.Time) (bool, error) {
	digest, err := ManifestDigest(p.Owner, family, scope, start, end, members)
	if err != nil {
		return false, err
	}
	id := ManifestContextID(p.Owner, family, scope, start, end)
	var lastDigest, state string
	var lastRevision int64
	var body []byte
	err = p.DB.QueryRow(ctx, `SELECT digest,revision,state,body FROM analytics_coverage_publications WHERE context_id=$1`, id).Scan(&lastDigest, &lastRevision, &state, &body)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return false, err
	case lastDigest == digest && state == "PUBLISHED":
		return false, nil
	case lastDigest == digest && state == "PENDING":
		return true, p.send(ctx, id, body)
	}
	revision := now.UnixMilli()
	if revision <= lastRevision {
		revision = lastRevision + 1
	}
	effective := now.Truncate(time.Second).Add(-time.Minute)
	approval, err := ManifestApproval{Owner: p.Owner, Environment: p.Environment, Family: family, Scope: scope, Start: start, End: end, Revision: revision, ProvenanceRef: p.ProvenanceRef, ApprovalRef: p.ApprovalRef, EffectiveAt: effective, ExpiresAt: effective.Add(p.TTL)}.Document(members)
	if err != nil {
		return false, err
	}
	raw, err := p.Signer.SignManifest(approval, members)
	if err != nil {
		return false, err
	}
	// Record intent before sending, so a crash or unknown outcome re-sends the
	// same signed body instead of minting a revision the receiver may not have.
	if _, err := p.DB.Exec(ctx, `INSERT INTO analytics_coverage_publications(context_id,family,window_start,window_end,digest,member_count,revision,body,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'PENDING')
 ON CONFLICT(context_id) DO UPDATE SET digest=excluded.digest,member_count=excluded.member_count,revision=excluded.revision,body=excluded.body,state='PENDING',updated_at=now() WHERE analytics_coverage_publications.revision<excluded.revision`, id, family, start, end, digest, len(members), revision, raw); err != nil {
		return false, err
	}
	return true, p.send(ctx, id, raw)
}

func (p *Producer) send(ctx context.Context, id string, raw []byte) error {
	err := p.Publisher.PublishManifest(ctx, raw)
	if err != nil && !errors.Is(err, ErrStaleManifest) {
		return err
	}
	// Stale means the receiver already holds this (or a newer) revision.
	_, err = p.DB.Exec(ctx, `UPDATE analytics_coverage_publications SET state='PUBLISHED',updated_at=now() WHERE context_id=$1 AND body=$2`, id, raw)
	return err
}

// RosterSource observes the owner's current supply roster from the same
// availability records its matcher dispatches from.
type RosterSource interface {
	Observe(ctx context.Context, bucket time.Time) ([]networkanalytics.SupplyRosterMember, error)
}

// RosterOutbox enqueues one envelope in the owner's analytics outbox inside
// the caller's transaction, so the roster record and its publication commit
// together.
type RosterOutbox interface {
	Enqueue(ctx context.Context, tx pgx.Tx, envelope sharedevents.Envelope) error
}

type RosterRecorder struct {
	SourceService string
	PolicyVersion string
	DB            *pgxpool.Pool
	Source        RosterSource
	Outbox        RosterOutbox
	Now           func() time.Time
}

// RunOnce records the current bucket exactly once across replicas. Members
// are individual providers; nothing is aggregated.
func (r *RosterRecorder) RunOnce(ctx context.Context) (int, error) {
	if r == nil || r.DB == nil || r.Source == nil || r.Outbox == nil || r.SourceService == "" || r.PolicyVersion == "" {
		return 0, ErrInvalid
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	bucket := networkanalytics.BucketStart(now)
	members, err := r.Source.Observe(ctx, bucket)
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	for _, m := range members {
		key := m.ProviderID + "|" + m.ProductCode
		if !m.Valid() || !m.BucketStart.Equal(bucket) || seen[key] {
			return 0, ErrInvalid
		}
		seen[key] = true
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `INSERT INTO analytics_supply_roster_runs(bucket_start,member_count,policy_version) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, bucket, len(members), r.PolicyVersion)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, nil
	}
	for _, m := range members {
		if _, err := tx.Exec(ctx, `INSERT INTO analytics_supply_roster_members(bucket_start,provider_id,product_code,eligible,occupied,event_id) VALUES($1,$2,$3,$4,$5,$6)`, bucket, m.ProviderID, m.ProductCode, m.Eligible, m.Occupied, m.EventID()); err != nil {
			return 0, err
		}
		envelope, err := networkanalytics.RosterEnvelope(r.SourceService, m)
		if err != nil {
			return 0, err
		}
		if err := r.Outbox.Enqueue(ctx, tx, envelope); err != nil {
			return 0, err
		}
	}
	return len(members), tx.Commit(ctx)
}

// ProducerOptions are the owner-fixed parts of the runtime.
type ProducerOptions struct {
	Prefix, Owner string
	DB            *pgxpool.Pool
	Source        FactSource
	Binding       admincontext.BindingReader // the owner's own admin resource source
	AnchorKinds   []string
	Roster        RosterSource
	RosterOutbox  RosterOutbox
	PolicyVersion string
	// Global owners have no own five-scope resource contexts; their scope
	// membership comes from per-fact allocations they already sign.
	Global bool
}

// ProducerRuntime is the default-off composition of producer and recorder.
type ProducerRuntime struct {
	Producer *Producer
	Recorder *RosterRecorder
	Client   *PublisherClient
}

// ProducerFromEnvironment reads <PREFIX>_ANALYTICS_PRODUCER_* settings. With
// <PREFIX>_ANALYTICS_PRODUCER_ENABLED unset it returns (nil, nil) and touches
// no credential. The roster additionally needs <PREFIX>_ANALYTICS_ROSTER_ENABLED.
func ProducerFromEnvironment(ctx context.Context, o ProducerOptions) (*ProducerRuntime, error) {
	if os.Getenv(o.Prefix+"_ANALYTICS_PRODUCER_ENABLED") != "true" {
		return nil, nil
	}
	if o.DB == nil || o.Source == nil || !o.Global && (o.Binding == nil || len(o.AnchorKinds) == 0) {
		return nil, ErrInvalid
	}
	var ready bool
	if err := o.DB.QueryRow(ctx, `SELECT to_regclass('analytics_coverage_publications') IS NOT NULL AND to_regclass('analytics_supply_roster_runs') IS NOT NULL AND ($1 OR to_regclass('admin_owner_context_heads') IS NOT NULL)`, o.Global).Scan(&ready); err != nil || !ready {
		return nil, errors.New("analytics producer schema unavailable; apply normal migrations")
	}
	signer, err := LoadSigner(os.Getenv(o.Prefix + "_ANALYTICS_PRODUCER_SIGNING_KEY_FILE"))
	if err != nil {
		return nil, errors.New("analytics producer signing key unavailable")
	}
	secret, err := readSecretFile(os.Getenv(o.Prefix + "_ANALYTICS_PRODUCER_PUBLISH_SECRET_FILE"))
	if err != nil {
		return nil, errors.New("analytics producer publication credential unavailable")
	}
	var resolver ScopeResolver
	if !o.Global {
		keys, err := readContextKeys(os.Getenv(o.Prefix + "_ADMIN_CONTEXT_KEYS_FILE"))
		if err != nil || len(keys) == 0 {
			return nil, errors.New("owner authorization context keys required for coverage scope")
		}
		resolver = &ContextScopeResolver{DB: o.DB, Store: &admincontext.Store{DB: o.DB, Reader: o.Binding, Authority: admincontext.Authority{Owner: o.Owner, Environment: adminpolicyenv.Environment, Keys: keys}}}
	}
	client, err := NewPublisherClient(os.Getenv(o.Prefix+"_ANALYTICS_PRODUCER_ORIGIN"), o.Owner, secret)
	if err != nil {
		return nil, err
	}
	lookback := 36 * 24 * time.Hour
	if raw := os.Getenv(o.Prefix + "_ANALYTICS_PRODUCER_LOOKBACK_HOURS"); raw != "" {
		hours, err := strconv.Atoi(raw)
		if err != nil || hours < 1 || hours > 24*90 {
			return nil, ErrInvalid
		}
		lookback = time.Duration(hours) * time.Hour
	}
	rt := &ProducerRuntime{Client: client, Producer: &Producer{Owner: o.Owner, Environment: adminpolicyenv.Environment, DB: o.DB, Source: o.Source, Resolver: resolver, Global: o.Global, AnchorKinds: o.AnchorKinds, Signer: signer, Publisher: client,
		Period: time.Hour, Lookback: lookback, Settle: 10 * time.Minute, TTL: lookback + 90*24*time.Hour,
		ProvenanceRef: os.Getenv(o.Prefix + "_ANALYTICS_PRODUCER_PROVENANCE_REF"), ApprovalRef: os.Getenv(o.Prefix + "_ANALYTICS_PRODUCER_APPROVAL_REF")}}
	if err := rt.Producer.validate(); err != nil {
		client.Close()
		return nil, errors.New("analytics producer approval/provenance references required")
	}
	if os.Getenv(o.Prefix+"_ANALYTICS_ROSTER_ENABLED") == "true" {
		if o.Roster == nil || o.RosterOutbox == nil || o.PolicyVersion == "" {
			client.Close()
			return nil, ErrInvalid
		}
		rt.Recorder = &RosterRecorder{SourceService: o.Owner, PolicyVersion: o.PolicyVersion, DB: o.DB, Source: o.Roster, Outbox: o.RosterOutbox}
		rt.Producer.RosterEnabled = true
	}
	return rt, nil
}

// Run drives the recorder each roster bucket and the producer each period.
func (rt *ProducerRuntime) Run(ctx context.Context, report func(string, error)) {
	if rt == nil {
		return
	}
	defer rt.Client.Close()
	roster := time.NewTicker(networkanalytics.SupplyBucket)
	defer roster.Stop()
	produce := time.NewTicker(15 * time.Minute)
	defer produce.Stop()
	step := func(name string, fn func() error) {
		if err := fn(); err != nil && report != nil && ctx.Err() == nil {
			report(name, err)
		}
	}
	runRoster := func() error {
		if rt.Recorder == nil {
			return nil
		}
		_, err := rt.Recorder.RunOnce(ctx)
		return err
	}
	runProducer := func() error { _, err := rt.Producer.RunOnce(ctx); return err }
	step("roster", runRoster)
	step("coverage", runProducer)
	for {
		select {
		case <-ctx.Done():
			return
		case <-roster.C:
			step("roster", runRoster)
		case <-produce.C:
			step("coverage", runProducer)
		}
	}
}

func readSecretFile(path string) ([]byte, error) {
	raw, err := readLimited(path, 4096)
	if err != nil {
		return nil, err
	}
	secret, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(secret) < 32 {
		return nil, ErrInvalid
	}
	return secret, nil
}

// readContextKeys reads the owner's existing context key file format:
// {"key-id": "<base64url Ed25519 public key>"}.
func readContextKeys(path string) (map[string]ed25519.PublicKey, error) {
	raw, err := readLimited(path, 16<<10)
	if err != nil {
		return nil, err
	}
	encoded := map[string]string{}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return nil, ErrInvalid
	}
	for d.More() {
		token, err := d.Token()
		id, ok := token.(string)
		if err != nil || !ok || encoded[id] != "" || len(encoded) >= 32 {
			return nil, ErrInvalid
		}
		var value string
		if d.Decode(&value) != nil || value == "" {
			return nil, ErrInvalid
		}
		encoded[id] = value
	}
	if _, err := d.Token(); err != nil {
		return nil, ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF || len(encoded) == 0 {
		return nil, ErrInvalid
	}
	keys := map[string]ed25519.PublicKey{}
	ids := make([]string, 0, len(encoded))
	for id := range encoded {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		key, err := base64.RawURLEncoding.DecodeString(encoded[id])
		if err != nil || len(key) != ed25519.PublicKeySize || !adminread.Opaque(id) {
			return nil, ErrInvalid
		}
		keys[id] = key
	}
	return keys, nil
}

func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrInvalid
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrInvalid
	}
	return raw, nil
}
