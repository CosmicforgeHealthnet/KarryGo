// Package testfixture generates ephemeral TEST_ONLY approvals for integration
// tests. No private keys, production grants or thresholds are checked in.
package testfixture

import (
	"cosmicforge/logistics/shared/go/adminpolicy"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

type Fixture struct {
	Engine         *adminpolicy.Engine
	Document       adminpolicy.Document
	Envelope, Keys []byte
	Public         ed25519.PublicKey
	private        ed25519.PrivateKey
}

func NewTaxi(t testing.TB, scope adminpolicy.Scope) *Fixture {
	t.Helper()
	f := New(t, scope, "driver-taxi-service", "taxi.rides.read", "taxi.corporate-rides.v1")
	f.Document.Rules[0].CanonicalFields = map[string]string{"market": "markets.code via taxi_bookings.market_id", "zone": "operating_zones.code via taxi_bookings.operating_zone_id", "organization": "taxi_bookings.organization_id for Fleet-origin corporate execution", "tenant": "Fleet tenant.Context.OrganizationID frozen on Taxi corporate execution"}
	f.Sign(t)
	return f
}
func New(t testing.TB, scope adminpolicy.Scope, owner, capability, contract string) *Fixture {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &Fixture{Public: pub, private: key, Document: adminpolicy.Document{SchemaVersion: 1, ID: "test-only-policy", Version: 1, State: "APPROVED", Environment: adminpolicy.TestOnly, ApprovalReference: "TEST_ONLY_OWNER_APPROVAL", ApprovedOwners: []string{owner}, EffectiveAt: time.Now().UTC().Add(-time.Minute).Truncate(time.Second), ExpiresAt: time.Now().UTC().Add(time.Hour).Truncate(time.Second), Rules: []adminpolicy.Rule{{Capability: capability, Owner: owner, Permission: capability, Roles: []string{"staff"}, Purposes: []string{"case-1", "CASE-123"}, SourceContract: contract, Privacy: "TEST_ONLY_MINIMIZED", CanonicalFields: map[string]string{"market": "TEST_ONLY_OWNER_MARKET_FIELD", "zone": "TEST_ONLY_OWNER_ZONE_FIELD", "organization": "TEST_ONLY_OWNER_ORGANIZATION_FIELD", "tenant": "TEST_ONLY_OWNER_TENANT_FIELD"}, Scopes: []adminpolicy.Scope{scope}}}}}
	f.Sign(t)
	return f
}
func (f *Fixture) Sign(t testing.TB) {
	t.Helper()
	raw, err := json.Marshal(f.Document)
	if err != nil {
		t.Fatal(err)
	}
	signed := ed25519.Sign(f.private, append([]byte(adminpolicy.SigningDomain), raw...))
	f.Envelope, err = json.Marshal(adminpolicy.Envelope{Document: raw, KeyID: "test-only-key", Signature: base64.RawURLEncoding.EncodeToString(signed)})
	if err != nil {
		t.Fatal(err)
	}
	f.Keys, err = json.Marshal(map[string]string{"test-only-key": base64.RawURLEncoding.EncodeToString(f.Public)})
	if err != nil {
		t.Fatal(err)
	}
	f.Engine, err = adminpolicy.Verify(f.Envelope, map[string]ed25519.PublicKey{"test-only-key": f.Public}, adminpolicy.TestOnly, time.Now())
	if err != nil {
		t.Fatal(err)
	}
}
