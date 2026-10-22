// Package adminpolicy evaluates owner-approved disclosure policy after the
// existing workload and delegated-staff authentication boundaries. It does not
// authenticate HTTP callers, resolve domain ownership or publish approvals.
package adminpolicy

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"cosmicforge/logistics/shared/go/adminread"
)

var ErrBlocked = errors.New("BLOCKED_POLICY")
var ErrDenied = errors.New("policy access denied")
var ErrInvalid = errors.New("invalid approval policy")

const Production = "PRODUCTION"
const TestOnly = "TEST_ONLY"
const SigningDomain = "karryam-admin-owner-policy-v1\n"

type Scope struct {
	Product      string `json:"product"`
	Market       string `json:"market"`
	Zone         string `json:"zone"`
	Organization string `json:"organization"`
	Tenant       string `json:"tenant"`
}
type Rule struct {
	Capability      string            `json:"capability"`
	Owner           string            `json:"owner"`
	Permission      string            `json:"permission"`
	Roles           []string          `json:"roles"`
	Purposes        []string          `json:"purposes"`
	SourceContract  string            `json:"source_contract"`
	CanonicalFields map[string]string `json:"canonical_fields"`
	Privacy         string            `json:"privacy"`
	Scopes          []Scope           `json:"scopes"`
}
type Disclosure struct {
	MaxLookbackSeconds       int                 `json:"max_lookback_seconds"`
	PrivacyDomain            string              `json:"privacy_domain"`
	AllowedCellKeys          []string            `json:"allowed_cell_keys"`
	AllowedFilterValues      map[string][]string `json:"allowed_filter_values"`
	MinimumCohort            int64               `json:"minimum_cohort"`
	RoundingMultiple         int64               `json:"rounding_multiple"`
	MaxQueriesPerStaffWindow int                 `json:"max_queries_per_staff_window"`
	QueryWindowSeconds       int                 `json:"query_window_seconds"`
	ReleaseBucketSeconds     int                 `json:"release_bucket_seconds"`
	FreshnessSeconds         int                 `json:"freshness_seconds"`
	FilterSets               [][]string          `json:"filter_sets"`
}

// MediaDisclosure has no defaults. A signed owner-approved document must
// explicitly bound grant lifetime and eligible source-case permissions.
type MediaDisclosure struct {
	MaxGrantSeconds    int      `json:"max_grant_seconds"`
	SourceCapabilities []string `json:"source_capabilities"`
}

type Document struct {
	SchemaVersion     int              `json:"schema_version"`
	ID                string           `json:"id"`
	Version           int64            `json:"version"`
	State             string           `json:"state"`
	Environment       string           `json:"environment"`
	ApprovalReference string           `json:"approval_reference"`
	ApprovedOwners    []string         `json:"approved_owners"`
	EffectiveAt       time.Time        `json:"effective_at"`
	ExpiresAt         time.Time        `json:"expires_at"`
	Rules             []Rule           `json:"rules"`
	Analytics         *Disclosure      `json:"analytics,omitempty"`
	Media             *MediaDisclosure `json:"media,omitempty"`
}
type Envelope struct {
	Document  json.RawMessage `json:"document"`
	KeyID     string          `json:"key_id"`
	Signature string          `json:"signature"`
}
type Engine struct {
	document Document
	checksum string
}
type Actor struct {
	Product, Staff                                             string
	Roles, Permissions, Markets, Zones, Organizations, Tenants []string
}

// Facts must be resolved from an authoritative owner record/contract. Query
// identifiers are never facts. A policy document cannot supply missing facts.
type Facts struct {
	Owner, Reference, SourceContract string
	Scope                            Scope
}

func strict(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := unique(d); err != nil {
		return ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrInvalid
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
			key, e := d.Token()
			if e != nil {
				return e
			}
			k, ok := key.(string)
			if !ok || seen[k] {
				return ErrInvalid
			}
			seen[k] = true
			if e = unique(d); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if err = unique(d); err != nil {
				return err
			}
		}
	default:
		return ErrInvalid
	}
	_, err = d.Token()
	return err
}
func validList(values []string) bool {
	if len(values) == 0 || len(values) > 128 {
		return false
	}
	seen := map[string]bool{}
	for _, v := range values {
		if !adminread.Opaque(v) || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (s Scope) valid() bool {
	return s.Product == "karryam" && adminread.Opaque(s.Market) && adminread.Opaque(s.Zone) && adminread.Opaque(s.Organization) && adminread.Opaque(s.Tenant)
}

// Verify accepts only a signed approved document for the exact expected
// environment. Production entry points always pass Production; TEST_ONLY
// documents can never be enabled by production startup configuration.
func Verify(raw []byte, keys map[string]ed25519.PublicKey, environment string, now time.Time) (*Engine, error) {
	if len(raw) == 0 {
		return nil, ErrBlocked
	}
	if len(raw) > 256*1024 || (environment != Production && environment != TestOnly) {
		return nil, ErrInvalid
	}
	var envelope Envelope
	if strict(raw, &envelope) != nil {
		return nil, ErrInvalid
	}
	var document Document
	if strict(envelope.Document, &document) != nil {
		return nil, ErrInvalid
	}
	if document.State != "APPROVED" {
		return nil, ErrBlocked
	}
	key := keys[envelope.KeyID]
	sig, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, append([]byte(SigningDomain), envelope.Document...), sig) {
		return nil, ErrInvalid
	}
	if document.SchemaVersion != 1 || !adminread.Opaque(document.ID) || document.Version < 1 || document.Environment != environment || !adminread.Opaque(document.ApprovalReference) || !validList(document.ApprovedOwners) || document.EffectiveAt.IsZero() || now.Before(document.EffectiveAt) || !document.ExpiresAt.After(now) || len(document.Rules) == 0 || len(document.Rules) > 128 {
		return nil, ErrBlocked
	}
	seen := map[string]bool{}
	for _, r := range document.Rules {
		if !adminread.Opaque(r.Capability) || !adminread.Opaque(r.Permission) || !adminread.Opaque(r.Owner) || !contains(document.ApprovedOwners, r.Owner) || !validList(r.Roles) || !validList(r.Purposes) || !adminread.Opaque(r.SourceContract) || !adminread.Opaque(r.Privacy) || len(r.Scopes) == 0 || len(r.Scopes) > 1024 || seen[r.Capability] {
			return nil, ErrInvalid
		}
		seen[r.Capability] = true
		for _, field := range []string{"market", "zone", "organization", "tenant"} {
			if r.CanonicalFields[field] == "" || len(r.CanonicalFields[field]) > 256 {
				return nil, ErrBlocked
			}
		}
		scopes := map[Scope]bool{}
		for _, scope := range r.Scopes {
			if !scope.valid() || scopes[scope] {
				return nil, ErrInvalid
			}
			scopes[scope] = true
		}
	}
	if p := document.Media; p != nil {
		if p.MaxGrantSeconds < 1 || p.MaxGrantSeconds > 86400 || !validList(p.SourceCapabilities) {
			return nil, ErrInvalid
		}
	}
	if p := document.Analytics; p != nil {
		if p.MaxLookbackSeconds < p.ReleaseBucketSeconds || p.MaxLookbackSeconds > 10*366*86400 || !adminread.Opaque(p.PrivacyDomain) || !validList(p.AllowedCellKeys) || p.MinimumCohort < 1 || p.RoundingMultiple < 1 || p.RoundingMultiple > 1_000_000 || p.MaxQueriesPerStaffWindow < 1 || p.MaxQueriesPerStaffWindow > 10000 || p.QueryWindowSeconds < 1 || p.QueryWindowSeconds > 86400 || p.ReleaseBucketSeconds < 1 || p.ReleaseBucketSeconds > 31*86400 || p.FreshnessSeconds < 1 || len(p.FilterSets) == 0 || len(p.FilterSets) > 32 {
			return nil, ErrInvalid
		}
		for _, set := range p.FilterSets {
			if len(set) > 8 || !validList(set) {
				return nil, ErrInvalid
			}
			for _, field := range set {
				if !validList(p.AllowedFilterValues[field]) {
					return nil, ErrInvalid
				}
			}
		}
	}
	hash := sha256.Sum256(envelope.Document)
	return &Engine{document: document, checksum: hex.EncodeToString(hash[:])}, nil
}
func LoadFiles(policyFile, keysFile, environment string, now time.Time) (*Engine, error) {
	if policyFile == "" || keysFile == "" {
		return nil, ErrBlocked
	}
	raw, err := readBounded(policyFile, 256*1024)
	if err != nil {
		return nil, ErrInvalid
	}
	keyRaw, err := readBounded(keysFile, 32768)
	if err != nil {
		return nil, ErrInvalid
	}
	var encoded map[string]string
	if strict(keyRaw, &encoded) != nil {
		return nil, ErrInvalid
	}
	keys := map[string]ed25519.PublicKey{}
	for id, value := range encoded {
		key, e := base64.RawURLEncoding.DecodeString(value)
		if !adminread.Opaque(id) || e != nil || len(key) != ed25519.PublicKeySize {
			return nil, ErrInvalid
		}
		keys[id] = ed25519.PublicKey(key)
	}
	return Verify(raw, keys, environment, now)
}
func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrInvalid
	}
	return raw, nil
}
func (e *Engine) Version() int64 {
	if e == nil {
		return 0
	}
	return e.document.Version
}
func (e *Engine) ID() string {
	if e == nil {
		return ""
	}
	return e.document.ID
}
func (e *Engine) Checksum() string {
	if e == nil {
		return ""
	}
	return e.checksum
}
func (e *Engine) Rule(capability string, now time.Time) (Rule, error) {
	if e == nil || e.document.State != "APPROVED" || now.Before(e.document.EffectiveAt) || !e.document.ExpiresAt.After(now) {
		return Rule{}, ErrBlocked
	}
	for _, r := range e.document.Rules {
		if r.Capability == capability {
			return cloneRule(r), nil
		}
	}
	return Rule{}, ErrDenied
}
func cloneRule(r Rule) Rule {
	r.Roles = append([]string(nil), r.Roles...)
	r.Purposes = append([]string(nil), r.Purposes...)
	r.Scopes = append([]Scope(nil), r.Scopes...)
	fields := map[string]string{}
	for k, v := range r.CanonicalFields {
		fields[k] = v
	}
	r.CanonicalFields = fields
	return r
}
func (e *Engine) Disclosure(now time.Time) (Disclosure, error) {
	if e == nil || !e.document.ExpiresAt.After(now) || now.Before(e.document.EffectiveAt) || e.document.Analytics == nil {
		return Disclosure{}, ErrBlocked
	}
	p := *e.document.Analytics
	p.FilterSets = append([][]string(nil), p.FilterSets...)
	for i := range p.FilterSets {
		p.FilterSets[i] = append([]string(nil), p.FilterSets[i]...)
	}
	p.AllowedCellKeys = append([]string(nil), p.AllowedCellKeys...)
	p.AllowedFilterValues = map[string][]string{}
	for k, values := range e.document.Analytics.AllowedFilterValues {
		p.AllowedFilterValues[k] = append([]string(nil), values...)
	}
	return p, nil
}
func (e *Engine) AuthorizeDelegation(actor Actor, capability, purpose string, scope Scope, now time.Time) error {
	r, err := e.Rule(capability, now)
	if err != nil {
		return err
	}
	if !scope.valid() || actor.Product != scope.Product || !adminread.Opaque(actor.Staff) || !contains(actor.Permissions, r.Permission) || !contains(r.Purposes, purpose) || !contains(actor.Markets, scope.Market) || !contains(actor.Zones, scope.Zone) || !contains(actor.Organizations, scope.Organization) || !contains(actor.Tenants, scope.Tenant) {
		return ErrDenied
	}
	role := false
	for _, v := range actor.Roles {
		role = role || contains(r.Roles, v)
	}
	if !role {
		return ErrDenied
	}
	for _, grant := range r.Scopes {
		if grant == scope {
			return nil
		}
	}
	return ErrDenied
}
func (e *Engine) AuthorizeOwner(actor Actor, capability, purpose string, scope Scope, facts Facts, now time.Time) error {
	if err := e.AuthorizeDelegation(actor, capability, purpose, scope, now); err != nil {
		return err
	}
	r, _ := e.Rule(capability, now)
	if facts.Owner != r.Owner || !adminread.Opaque(facts.Reference) || facts.SourceContract != r.SourceContract || !facts.Scope.valid() || facts.Scope != scope {
		return ErrDenied
	}
	return nil
}

func (e *Engine) MediaDisclosure(now time.Time) (MediaDisclosure, error) {
	if e == nil || !e.document.ExpiresAt.After(now) || now.Before(e.document.EffectiveAt) || e.document.Media == nil {
		return MediaDisclosure{}, ErrBlocked
	}
	out := *e.document.Media
	out.SourceCapabilities = append([]string{}, out.SourceCapabilities...)
	return out, nil
}

// ExpiresAt is signed policy metadata; it never supplies a grant or a default.
func (e *Engine) ExpiresAt() time.Time {
	if e == nil {
		return time.Time{}
	}
	return e.document.ExpiresAt
}
