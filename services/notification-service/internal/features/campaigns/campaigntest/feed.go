package campaigntest

import (
	"context"
	"cosmicforge/logistics/services/notification-service/internal/features/adminfeed"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

// FeedAuthority is test-only, ephemeral signing material. Production receives
// public keys and signed owner decisions through its native publication route.
type FeedAuthority struct {
	T         *testing.T
	Guard     adminfeed.ScopeGuard
	Key       ed25519.PrivateKey
	Scope     adminpolicy.Scope
	Approvals map[string]admincontext.Approval
}

func NewFeedAuthority(t *testing.T, db *pgxpool.Pool) *FeedAuthority {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "NG-LAG", Zone: "NG-LAG-MAIN", Organization: "karryam-staging", Tenant: "karryam-staging-operations"}
	store := &admincontext.Store{DB: db, Reader: adminfeed.SourceReader{}, Authority: admincontext.Authority{Owner: adminfeed.Source, Environment: adminpolicy.TestOnly, Keys: map[string]ed25519.PublicKey{"test-owner": pub}}}
	policy := testfixture.New(t, scope, adminfeed.Source, "hq.campaigns.readback", "karryam.campaign-intent.v1")
	policy.Document.Rules[0].Purposes = []string{"hq_event_sync"}
	policy.Sign(t)
	return &FeedAuthority{T: t, Key: key, Scope: scope, Approvals: map[string]admincontext.Approval{}, Guard: adminfeed.ScopeGuard{Owner: &adminowner.Handler{Owner: adminfeed.Source, DB: db, Contexts: store, Policy: policy.Engine}}}
}
func (f *FeedAuthority) importApproval(a admincontext.Approval) {
	f.T.Helper()
	raw, err := json.Marshal(a)
	if err != nil {
		f.T.Fatal(err)
	}
	wire, err := json.Marshal(admincontext.Envelope{Document: raw, KeyID: "test-owner", Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(f.Key, append([]byte(admincontext.SigningDomain), raw...)))})
	if err != nil {
		f.T.Fatal(err)
	}
	if err := f.Guard.Owner.Contexts.Import(context.Background(), wire, time.Now()); err != nil {
		f.T.Fatal(err)
	}
	f.Approvals[a.ResourceRef] = a
}
func (f *FeedAuthority) Approve(ref string) {
	f.T.Helper()
	ctx := context.Background()
	tx, err := f.Guard.Owner.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		f.T.Fatal(err)
	}
	binding, err := (adminfeed.SourceReader{}).Binding(ctx, tx, "campaign-revisions", ref)
	tx.Rollback(ctx)
	if err != nil {
		f.T.Fatal(err)
	}
	f.importApproval(admincontext.Approval{SchemaVersion: 1, ID: "test-" + ref, Revision: 1, Owner: adminfeed.Source, Environment: adminpolicy.TestOnly, State: "APPROVED", ResourceKind: "campaign-revisions", ResourceRef: ref, SourceContract: binding.SourceContract, BindingFingerprint: binding.Fingerprint, Scope: f.Scope, Capabilities: []string{"hq.campaigns.readback"}, ProvenanceRef: "TEST_ONLY_ALLOCATION", ApprovalRef: "TEST_ONLY_OWNER_APPROVAL", EffectiveAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour)})
}
func (f *FeedAuthority) Revoke(ref string) {
	a := f.Approvals[ref]
	a.Revision++
	a.State = "REVOKED"
	f.importApproval(a)
}
