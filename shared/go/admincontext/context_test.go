package admincontext

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	Authority
	Approval Approval
	Private  ed25519.PrivateKey
}

func newApproval(t *testing.T) fixture {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	return fixture{Authority: Authority{Owner: "customer-service", Environment: adminpolicy.TestOnly, Keys: map[string]ed25519.PublicKey{"source-owner-test-only": pub}}, Private: key, Approval: Approval{SchemaVersion: 1, ID: "test-context", Revision: 1, Owner: "customer-service", Environment: adminpolicy.TestOnly, State: "APPROVED", ResourceKind: "customer", ResourceRef: "customer-1", SourceContract: "customer.admin.v1", BindingFingerprint: Fingerprint("actual-owner-identity"), Scope: adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}, Capabilities: []string{"customer.directory.read"}, ProvenanceRef: "TEST_ONLY_OWNER_REMEDIATION", ApprovalRef: "TEST_ONLY_SOURCE_OWNER_REVIEW", EffectiveAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}}
}
func (f fixture) signed(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(f.Approval)
	if err != nil {
		t.Fatal(err)
	}
	envelope := Envelope{Document: raw, KeyID: "source-owner-test-only", Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(f.Private, append([]byte(SigningDomain), raw...)))}
	raw, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSourceOwnerContextRequiresIndependentAuthenticApproval(t *testing.T) {
	f := newApproval(t)
	raw := f.signed(t)
	if _, _, err := f.Verify(raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, test := range []string{"production", "key", "owner", "tamper", "expired", "draft", "empty-tenant", "duplicate-key", "case-alias"} {
		t.Run(test, func(t *testing.T) {
			g := newApproval(t)
			authority := g.Authority
			wire := g.signed(t)
			switch test {
			case "production":
				authority.Environment = adminpolicy.Production
			case "key":
				authority.Keys = nil
			case "owner":
				authority.Owner = "driver-dispatch-delivery-service"
			case "tamper":
				wire = []byte(strings.Replace(string(wire), "customer-1", "customer-2", 1))
			case "expired":
				g.Approval.ExpiresAt = time.Now().Add(-time.Second)
				wire = g.signed(t)
			case "draft":
				g.Approval.State = "DRAFT"
				wire = g.signed(t)
			case "empty-tenant":
				g.Approval.Scope.Tenant = ""
				wire = g.signed(t)
			case "duplicate-key":
				wire = append([]byte(`{"key_id":"forged",`), wire[1:]...)
			case "case-alias":
				wire = append([]byte(`{"Key_ID":"forged",`), wire[1:]...)
			}
			if _, _, err := authority.Verify(wire, time.Now()); err == nil {
				t.Fatal("unauthentic, ambiguous or unapproved context accepted")
			}
		})
	}
}

type recordReader struct{}

func (recordReader) Binding(ctx context.Context, tx pgx.Tx, kind, ref string) (RecordBinding, error) {
	var record RecordBinding
	record.SourceContract = "customer.admin.v1"
	err := tx.QueryRow(ctx, `SELECT fingerprint,organization_ref,tenant_ref FROM test_owner_records WHERE kind=$1 AND ref=$2`, kind, ref).Scan(&record.Fingerprint, &record.Canonical.Organization, &record.Canonical.Tenant)
	return record, err
}
func contextDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ADMIN_CONTEXT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ADMIN_CONTEXT_TEST_DATABASE_URL required for source-owner context tests")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.Contains(config.ConnConfig.Database, "test") {
		t.Fatal("disposable test database required")
	}
	root, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "owner_context_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = root.Exec(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); root.Close() })
	if _, err = pool.Exec(context.Background(), Schema+`; CREATE TABLE test_owner_records(kind text NOT NULL,ref text PRIMARY KEY,fingerprint text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestPostgresOwnerContextHistoryRevocationCanonicalLinksAndPolicyIntersection(t *testing.T) {
	pool := contextDB(t)
	ctx := context.Background()
	f := newApproval(t)
	store := &Store{DB: pool, Authority: f.Authority, Reader: recordReader{}}
	if err := store.Import(ctx, f.signed(t), time.Now()); !errors.Is(err, ErrUnmapped) {
		t.Fatalf("unmapped historical record accepted: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO test_owner_records VALUES('customer','customer-1',$1,'O1','O1')`, f.Approval.BindingFingerprint); err != nil {
		t.Fatal(err)
	}
	// A valid source-owner signature still cannot contradict canonical links.
	bad := f
	bad.Approval.Scope.Tenant = "O2"
	if err := store.Import(ctx, bad.signed(t), time.Now()); !errors.Is(err, ErrUnmapped) {
		t.Fatalf("canonical tenant overridden: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Import(ctx, f.signed(t), time.Now()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	resolve := func(scope adminpolicy.Scope) (adminpolicy.Facts, error) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return adminpolicy.Facts{}, err
		}
		defer tx.Rollback(ctx)
		return store.Resolve(ctx, tx, "customer", "customer-1", "customer.directory.read", scope, time.Now())
	}
	facts, err := resolve(f.Approval.Scope)
	if err != nil {
		t.Fatal(err)
	}
	policy := testfixture.New(t, f.Approval.Scope, f.Owner, "customer.directory.read", f.Approval.SourceContract)
	actor := adminpolicy.Actor{Product: "karryam", Staff: "staff-1", Roles: []string{"staff"}, Permissions: []string{"customer.directory.read"}, Markets: []string{"M1"}, Zones: []string{"Z1"}, Organizations: []string{"O1"}, Tenants: []string{"O1"}}
	if err := policy.Engine.AuthorizeOwner(actor, "customer.directory.read", "case-1", f.Approval.Scope, facts, time.Now()); err != nil {
		t.Fatal(err)
	}
	actor.Tenants = []string{"O2"}
	if err := policy.Engine.AuthorizeOwner(actor, "customer.directory.read", "case-1", f.Approval.Scope, facts, time.Now()); err == nil {
		t.Fatal("owner context overrode staff delegation")
	}
	foreign := f.Approval.Scope
	foreign.Organization = "O2"
	foreign.Tenant = "O2"
	if _, err := resolve(foreign); !errors.Is(err, ErrUnmapped) {
		t.Fatalf("foreign scope disclosed: %v", err)
	}
	rewrite := f
	rewrite.Approval.ApprovalRef = "DIFFERENT_REVIEW"
	if err := store.Import(ctx, rewrite.signed(t), time.Now()); !errors.Is(err, ErrApproval) {
		t.Fatalf("same revision rewritten: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE test_owner_records SET tenant_ref='O2' WHERE ref='customer-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(f.Approval.Scope); !errors.Is(err, ErrUnmapped) {
		t.Fatalf("stale canonical links authorized: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE test_owner_records SET tenant_ref='O1' WHERE ref='customer-1'`); err != nil {
		t.Fatal(err)
	}
	revoked := f
	revoked.Approval.Revision = 2
	revoked.Approval.State = "REVOKED"
	if err := store.Import(ctx, revoked.signed(t), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(f.Approval.Scope); !errors.Is(err, ErrUnmapped) {
		t.Fatalf("revoked context authorized: %v", err)
	}
	if err := store.Import(ctx, f.signed(t), time.Now()); !errors.Is(err, ErrApproval) {
		t.Fatalf("old approval reactivated revoked context: %v", err)
	}
	for _, sql := range []string{`UPDATE admin_owner_context_history SET revision=3`, `DELETE FROM admin_owner_context_history`, `TRUNCATE admin_owner_context_history`} {
		if _, err := pool.Exec(ctx, sql); err == nil {
			t.Fatal("owner approval history was mutable")
		}
	}
}
