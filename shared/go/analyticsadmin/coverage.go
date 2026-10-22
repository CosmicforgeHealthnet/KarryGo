package analyticsadmin

// Owner-signed coverage manifests.
//
// A scoped Analytics report may be released as COMPLETE only when the owner of
// its input facts has attested, for the exact report window and five-dimension
// scope, the complete set of individual facts it published. The attestation is
// an ordinary source-owner context approval (admincontext.Approval), signed by
// the owner's pinned Ed25519 key under the existing admincontext.SigningDomain:
//
//   resource_kind       = "analytics.coverage-manifest.v1:<family>"
//   resource_ref        = "w<start-unix>-<end-unix>" (the attested window)
//   binding_fingerprint = ManifestDigest over owner, family, scope, window and
//                         the canonical sorted member list
//   capabilities        = ["analytics.coverage.attest"]
//
// The member list travels beside the signed document. Because the signature
// covers the digest, a receiver recomputes the digest from the members it was
// given and rejects any list that differs by one member, one version or one
// timestamp. Monotonic revisions, revocation and expiry reuse the existing
// owner-context rules. A manifest grants no HQ access, approves no policy and
// never carries an aggregate count in place of members.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	CoverageManifestContract   = "analytics.coverage-manifest.v1"
	CoverageManifestKindPrefix = CoverageManifestContract + ":"
	// CoverageAttestCapability is the only capability a manifest may carry.
	CoverageAttestCapability = "analytics.coverage.attest"
	// CoverageExportCapability must be present on an owner's OWN resource
	// authorization context before the owner exports any fact anchored on that
	// resource into a scope's manifest. Absent it, the fact belongs to no scope.
	CoverageExportCapability = "analytics.coverage.export"
	CoverageManifestPath     = "/internal/analytics/v1/admin/coverage-manifests/"

	// FamilyNetworkEvents covers every individual network fact (lifecycle,
	// recovery, mismatch, decline, arrival, readiness) an owner published.
	FamilyNetworkEvents = "network-events"
	// FamilySupplyRoster covers individual provider roster members.
	FamilySupplyRoster = "supply-roster"
	// Global families attest the owner's complete publication across all
	// scopes. Scope membership must then come from the existing per-fact
	// signed allocations, and every member must have one.
	FamilyNetworkEventsGlobal = "network-events-global"
	FamilySupplyRosterGlobal  = "supply-roster-global"

	MaxManifestMembers   = 5000
	MaxManifestBodyBytes = 2 << 20
	MaxManifestWindow    = 31 * 24 * time.Hour
)

// GlobalScope is the fixed scope label of a global manifest. It is never a
// valid request scope for a scoped family and is never compared with one.
var GlobalScope = adminpolicy.Scope{Product: "karryam", Market: "global-coverage", Zone: "global-coverage", Organization: "global-coverage", Tenant: "global-coverage"}

var ErrManifest = errors.New("invalid coverage manifest")

// ErrStaleManifest: the receiver already holds this or a newer revision (or the
// request nonce was replayed). It is never a reason to mint a lower revision.
var ErrStaleManifest = fmt.Errorf("%w: stale or replayed manifest revision", ErrDenied)

// ManifestFamilies lists every family a receiver understands.
func ManifestFamilies() []string {
	return []string{FamilyNetworkEvents, FamilySupplyRoster, FamilyNetworkEventsGlobal, FamilySupplyRosterGlobal}
}

// ScopedFamily strips the global suffix: the fact family a manifest covers.
func ScopedFamily(family string) (string, bool) {
	switch family {
	case FamilyNetworkEvents, FamilySupplyRoster:
		return family, false
	case FamilyNetworkEventsGlobal:
		return FamilyNetworkEvents, true
	case FamilySupplyRosterGlobal:
		return FamilySupplyRoster, true
	}
	return "", false
}

// FactFamily classifies one published event into its coverage family. Supply
// snapshots and demand buckets are aggregates: they have no individual
// membership and belong to no family.
func FactFamily(eventName, payloadAuthority string) (string, bool) {
	_, kind, ok := networkanalytics.Classify(eventName, payloadAuthority)
	if !ok {
		return "", false
	}
	switch kind {
	case networkanalytics.KindSupplySnapshot, networkanalytics.KindDemandBucket:
		return "", false
	case networkanalytics.KindSupplyRoster:
		return FamilySupplyRoster, true
	}
	return FamilyNetworkEvents, true
}

// ManifestMember is one published fact's immutable identity, exactly as the
// owner delivered it and as Analytics retains it.
type ManifestMember struct {
	EventID    string    `json:"event_id"`
	EventName  string    `json:"event_name"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	Version    int64     `json:"version"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Manifest is the publication wire shape.
type Manifest struct {
	Envelope admincontext.Envelope `json:"envelope"`
	Members  []ManifestMember      `json:"members"`
}

func ManifestRef(start, end time.Time) string {
	return "w" + strconv.FormatInt(start.UTC().Unix(), 10) + "-" + strconv.FormatInt(end.UTC().Unix(), 10)
}

func ParseManifestRef(ref string) (time.Time, time.Time, error) {
	raw, ok := strings.CutPrefix(ref, "w")
	if !ok {
		return time.Time{}, time.Time{}, ErrManifest
	}
	a, b, ok := strings.Cut(raw, "-")
	if !ok {
		return time.Time{}, time.Time{}, ErrManifest
	}
	start, err1 := strconv.ParseInt(a, 10, 64)
	end, err2 := strconv.ParseInt(b, 10, 64)
	if err1 != nil || err2 != nil || start <= 0 || end <= start || strconv.FormatInt(start, 10) != a || strconv.FormatInt(end, 10) != b || time.Duration(end-start)*time.Second > MaxManifestWindow {
		return time.Time{}, time.Time{}, ErrManifest
	}
	return time.Unix(start, 0).UTC(), time.Unix(end, 0).UTC(), nil
}

// ManifestContextID binds the context identity to owner, family, scope and
// window, so one window's signed revisions can never replace another's.
func ManifestContextID(owner, family string, scope adminpolicy.Scope, start, end time.Time) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{CoverageManifestContract, owner, family, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant, ManifestRef(start, end)}, "\x00")))
	return "coverage-" + hex.EncodeToString(sum[:24])
}

func validMember(m ManifestMember, start, end time.Time) bool {
	return adminread.Opaque(m.EventID) && adminread.Opaque(m.EventName) && adminread.Opaque(m.EntityType) && adminread.Opaque(m.EntityID) && m.Version >= 1 && !m.OccurredAt.IsZero() && !m.OccurredAt.Before(start) && m.OccurredAt.Before(end)
}

// CanonicalMembers validates and sorts members by event identifier. Duplicate
// identifiers, members outside the window or of another family are refused.
func CanonicalMembers(family string, members []ManifestMember, start, end time.Time) ([]ManifestMember, error) {
	scoped, _ := ScopedFamily(family)
	if scoped == "" || len(members) > MaxManifestMembers {
		return nil, ErrManifest
	}
	out := make([]ManifestMember, len(members))
	seen := map[string]bool{}
	for i, m := range members {
		m.OccurredAt = m.OccurredAt.UTC().Truncate(time.Microsecond)
		if !validMember(m, start, end) || seen[m.EventID] {
			return nil, ErrManifest
		}
		// Roster facts carry their authority in the payload; the event name is
		// enough to tell the two families apart.
		memberFamily := FamilyNetworkEvents
		if m.EventName == networkanalytics.EventSupplyRoster {
			memberFamily = FamilySupplyRoster
		} else if m.EventName == networkanalytics.EventSupplySnapshot || m.EventName == networkanalytics.EventDemandBucket {
			return nil, ErrManifest
		}
		if memberFamily != scoped {
			return nil, ErrManifest
		}
		seen[m.EventID] = true
		out[i] = m
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EventID < out[j].EventID })
	return out, nil
}

// ManifestDigest is the binding fingerprint the owner signs.
func ManifestDigest(owner, family string, scope adminpolicy.Scope, start, end time.Time, members []ManifestMember) (string, error) {
	canonical, err := CanonicalMembers(family, members, start, end)
	if err != nil {
		return "", err
	}
	type line struct {
		ID, Name, Type, Entity string
		Version, OccurredMicro int64
	}
	lines := make([]line, len(canonical))
	for i, m := range canonical {
		lines[i] = line{m.EventID, m.EventName, m.EntityType, m.EntityID, m.Version, m.OccurredAt.UnixMicro()}
	}
	raw, err := json.Marshal(struct {
		Contract, Owner, Family string
		Scope                   adminpolicy.Scope
		Start, End              int64
		Count                   int
		Members                 []line
	}{CoverageManifestContract, owner, family, scope, start.UTC().Unix(), end.UTC().Unix(), len(lines), lines})
	if err != nil {
		return "", ErrManifest
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ManifestApproval builds the owner document. Provenance and approval refs are
// the owner's own export decision references; nothing here invents them.
type ManifestApproval struct {
	Owner, Environment, Family string
	Scope                      adminpolicy.Scope
	Start, End                 time.Time
	Revision                   int64
	ProvenanceRef, ApprovalRef string
	EffectiveAt, ExpiresAt     time.Time
	Revoked                    bool
}

func (m ManifestApproval) Document(members []ManifestMember) (admincontext.Approval, error) {
	digest, err := ManifestDigest(m.Owner, m.Family, m.Scope, m.Start, m.End, members)
	if err != nil {
		return admincontext.Approval{}, err
	}
	if _, global := ScopedFamily(m.Family); global != (m.Scope == GlobalScope) {
		return admincontext.Approval{}, ErrManifest
	}
	state := "APPROVED"
	if m.Revoked {
		state = "REVOKED"
	}
	return admincontext.Approval{SchemaVersion: 1, ID: ManifestContextID(m.Owner, m.Family, m.Scope, m.Start, m.End), Revision: m.Revision, Owner: m.Owner, Environment: m.Environment, State: state,
		ResourceKind: CoverageManifestKindPrefix + m.Family, ResourceRef: ManifestRef(m.Start, m.End), SourceContract: CoverageManifestKindPrefix + m.Family,
		BindingFingerprint: digest, Scope: m.Scope, Capabilities: []string{CoverageAttestCapability}, ProvenanceRef: m.ProvenanceRef, ApprovalRef: m.ApprovalRef,
		EffectiveAt: m.EffectiveAt.UTC(), ExpiresAt: m.ExpiresAt.UTC()}, nil
}

// Signer holds an owner's Ed25519 key. It lives only inside the owner process.
type Signer struct {
	KeyID string
	Key   ed25519.PrivateKey
}

// LoadSigner reads {"key_id": "...", "seed": "<base64url 32-byte seed>"}.
func LoadSigner(path string) (Signer, error) {
	f, err := os.Open(path)
	if err != nil {
		return Signer{}, ErrInvalid
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(raw) > 4096 {
		return Signer{}, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if unique(d, 0) != nil {
		return Signer{}, ErrInvalid
	}
	var doc struct {
		KeyID string `json:"key_id"`
		Seed  string `json:"seed"`
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&doc) != nil || !adminread.Opaque(doc.KeyID) {
		return Signer{}, ErrInvalid
	}
	seed, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(doc.Seed))
	if err != nil || len(seed) != ed25519.SeedSize {
		return Signer{}, ErrInvalid
	}
	return Signer{KeyID: doc.KeyID, Key: ed25519.NewKeyFromSeed(seed)}, nil
}

// SignManifest returns the publication body: signed approval plus members.
func (s Signer) SignManifest(approval admincontext.Approval, members []ManifestMember) ([]byte, error) {
	if len(s.Key) != ed25519.PrivateKeySize || !adminread.Opaque(s.KeyID) {
		return nil, ErrInvalid
	}
	start, end, err := ParseManifestRef(approval.ResourceRef)
	if err != nil {
		return nil, err
	}
	family := strings.TrimPrefix(approval.ResourceKind, CoverageManifestKindPrefix)
	canonical, err := CanonicalMembers(family, members, start, end)
	if err != nil {
		return nil, err
	}
	document, err := json.Marshal(approval)
	if err != nil {
		return nil, ErrInvalid
	}
	signature := ed25519.Sign(s.Key, append([]byte(admincontext.SigningDomain), document...))
	raw, err := json.Marshal(Manifest{Envelope: admincontext.Envelope{Document: document, KeyID: s.KeyID, Signature: base64.RawURLEncoding.EncodeToString(signature)}, Members: canonical})
	if err != nil || len(raw) > MaxManifestBodyBytes {
		return nil, ErrManifest
	}
	return raw, nil
}

// DecodeManifest strictly decodes a publication body. It does not verify the
// signature: the receiver does, with the owner's pinned authority.
func DecodeManifest(raw []byte) (Manifest, admincontext.Approval, error) {
	var out Manifest
	var approval admincontext.Approval
	if len(raw) == 0 || len(raw) > MaxManifestBodyBytes {
		return out, approval, ErrManifest
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if unique(d, 0) != nil {
		return out, approval, ErrManifest
	}
	if _, err := d.Token(); err != io.EOF {
		return out, approval, ErrManifest
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || len(out.Envelope.Document) == 0 || len(out.Members) > MaxManifestMembers {
		return Manifest{}, approval, ErrManifest
	}
	d = json.NewDecoder(bytes.NewReader(out.Envelope.Document))
	d.DisallowUnknownFields()
	if d.Decode(&approval) != nil {
		return Manifest{}, admincontext.Approval{}, ErrManifest
	}
	return out, approval, nil
}

// VerifiedManifest is a manifest whose signature, identity and digest have
// all been checked against a pinned owner authority.
type VerifiedManifest struct {
	Approval admincontext.Approval
	Checksum string
	Family   string
	Start    time.Time
	End      time.Time
	Members  []ManifestMember
}

// VerifyManifest checks the owner signature (environment, expiry, schema),
// manifest identity binding and that the digest covers exactly the members.
func VerifyManifest(authority admincontext.Authority, raw []byte, now time.Time) (VerifiedManifest, error) {
	var out VerifiedManifest
	manifest, approval, err := DecodeManifest(raw)
	if err != nil {
		return out, err
	}
	envelope, err := json.Marshal(manifest.Envelope)
	if err != nil {
		return out, ErrManifest
	}
	verified, checksum, err := authority.Verify(envelope, now)
	if err != nil {
		return out, ErrManifest
	}
	if verified.ID != approval.ID || verified.Revision != approval.Revision {
		return out, ErrManifest
	}
	family, ok := strings.CutPrefix(verified.ResourceKind, CoverageManifestKindPrefix)
	scoped, global := ScopedFamily(family)
	if !ok || scoped == "" || verified.SourceContract != verified.ResourceKind || len(verified.Capabilities) != 1 || verified.Capabilities[0] != CoverageAttestCapability || global != (verified.Scope == GlobalScope) || verified.Scope.Product != "karryam" {
		return out, ErrManifest
	}
	start, end, err := ParseManifestRef(verified.ResourceRef)
	if err != nil || verified.ID != ManifestContextID(verified.Owner, family, verified.Scope, start, end) {
		return out, ErrManifest
	}
	digest, err := ManifestDigest(verified.Owner, family, verified.Scope, start, end, manifest.Members)
	if err != nil || digest != verified.BindingFingerprint {
		return out, ErrManifest
	}
	canonical, _ := CanonicalMembers(family, manifest.Members, start, end)
	return VerifiedManifest{Approval: verified, Checksum: checksum, Family: family, Start: start, End: end, Members: canonical}, nil
}

// PublishManifest sends an already signed manifest to the fixed owner path.
// There is no automatic retry: an unknown outcome is retried by the producer's
// next run with an idempotent (same revision, same checksum) document.
func (c *PublisherClient) PublishManifest(ctx context.Context, raw []byte) error {
	_, approval, err := DecodeManifest(raw)
	if err != nil || approval.Owner != c.owner || !strings.HasPrefix(approval.ResourceKind, CoverageManifestKindPrefix) {
		return ErrInvalid
	}
	u := *c.transport.origin
	u.Path = CoverageManifestPath + c.owner
	u.RawPath = ""
	u.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(raw))
	if err != nil {
		return ErrInvalid
	}
	request.Header.Set("Content-Type", "application/json")
	if serviceauth.SignRequest(request, ProducerPublisherCaller, c.transport.secret, raw, time.Now()) != nil {
		return ErrUnavailable
	}
	response, err := c.transport.http.Do(request)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case 200:
	case 401, 403:
		return ErrDenied
	case 409:
		return ErrStaleManifest
	default:
		return ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1025))
	if err != nil || len(body) > 1024 {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if unique(d, 0) != nil {
		return ErrInvalid
	}
	var result struct {
		Status string `json:"status"`
	}
	d = json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&result) != nil || result.Status != "RECORDED" {
		return ErrInvalid
	}
	return nil
}
