// Package testfixture creates ephemeral TEST_ONLY source-owner context approvals.
// It never installs production keys, grants or a signing endpoint.
package testfixture

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
)

type Fixture struct {
	Authority admincontext.Authority
	private   ed25519.PrivateKey
}

func New(t testing.TB, owner string) *Fixture {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &Fixture{Authority: admincontext.Authority{Owner: owner, Environment: adminpolicy.TestOnly, Keys: map[string]ed25519.PublicKey{"test-only-source-owner": pub}}, private: key}
}
func (f *Fixture) Approval(id, kind, ref, capability string, binding admincontext.RecordBinding, scope adminpolicy.Scope) admincontext.Approval {
	now := time.Now().UTC().Truncate(time.Second)
	return admincontext.Approval{SchemaVersion: 1, ID: id, Revision: 1, Owner: f.Authority.Owner, Environment: adminpolicy.TestOnly, State: "APPROVED", ResourceKind: kind, ResourceRef: ref, SourceContract: binding.SourceContract, BindingFingerprint: binding.Fingerprint, Scope: scope, Capabilities: []string{capability}, ProvenanceRef: "TEST_ONLY_OWNER_CASE", ApprovalRef: "TEST_ONLY_OWNER_APPROVAL", EffectiveAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
}
func (f *Fixture) Sign(t testing.TB, a admincontext.Approval) []byte {
	t.Helper()
	document, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(admincontext.Envelope{Document: document, KeyID: "test-only-source-owner", Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(f.private, append([]byte(admincontext.SigningDomain), document...)))})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
