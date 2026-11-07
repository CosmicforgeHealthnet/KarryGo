package adminsnapshot

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/services/verification-compliance-service/internal/features/adminresources"
	"cosmicforge/logistics/services/verification-compliance-service/internal/testdb"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSnapshotDisabledByDefaultAndRequiresDedicatedSecret(t *testing.T) {
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_ENABLED", "")
	h, err := FromEnvironment(nil)
	if err != nil || h.Enabled {
		t.Fatalf("snapshot must default off: %+v %v", h, err)
	}
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_ENABLED", "true")
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE", filepath.Join(t.TempDir(), "missing"))
	if _, err := FromEnvironment(nil); err == nil {
		t.Fatal("enabled snapshot without secret accepted")
	}
	router := gin.New()
	(&Handler{}).Register(router)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, Path, nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled snapshot served %d", w.Code)
	}
}

func signed(t *testing.T, router http.Handler, caller string, secret []byte, query, nonce string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, Path+query, nil)
	req.Header.Set(serviceauth.HeaderNonce, nonce)
	if err := serviceauth.SignRequest(req, caller, secret, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestSnapshotBackfillPagesValidatedMinimizedEvents(t *testing.T) {
	pool := testdb.Fresh(t)
	ctx := context.Background()
	for _, business := range []string{"biz-s1", "biz-s2", "biz-s3", "biz with a space"} {
		if _, err := pool.Exec(ctx, `INSERT INTO vendor_verification_cases(client_reference,business_id,dimension,subject_type,subject_ref,document_refs,request_fingerprint,idempotency_key,reason_code) VALUES($1,$2,'FOOD_KITCHEN','BUSINESS','subject-secret','["doc-secret"]','fp',$1,'free text reason')`, uuid.NewString(), business); err != nil {
			t.Fatal(err)
		}
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	file := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(file, []byte(base64.RawURLEncoding.EncodeToString(raw)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_ENABLED", "true")
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE", file)
	guard, revoke := approvedGuard(t, pool)
	if _, err := FromEnvironment(pool); err == nil {
		t.Fatal("snapshot enabled without owner authority")
	}
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SECRET_FILE", file)
	if _, err := FromEnvironment(pool, guard); err == nil {
		t.Fatal("snapshot reused the event relay key")
	}
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SECRET_FILE", "")
	h, err := FromEnvironment(pool, guard)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	h.Register(router)

	if w := signed(t, router, "vendor-service", raw, "", uuid.NewString()); w.Code == http.StatusOK {
		t.Fatal("non-admin caller served")
	}
	other := []byte(strings.Repeat("x", 32))
	h.Verifier = serviceauth.NewVerifier(serviceauth.Secrets{Caller: raw, "vendor-service": other}, 5*time.Minute)
	if w := signed(t, router, "vendor-service", other, "", uuid.NewString()); w.Code != http.StatusForbidden {
		t.Fatalf("wrong caller=%d", w.Code)
	}
	if w := signed(t, router, Caller, raw, "?limit=201", uuid.NewString()); w.Code != http.StatusBadRequest {
		t.Fatalf("limit above 200=%d", w.Code)
	}
	seen := map[string]bool{}
	cursor := ""
	for pages := 0; pages < 5; pages++ {
		query := "?limit=2"
		if cursor != "" {
			query += "&cursor=" + cursor
		}
		nonce := uuid.NewString()
		w := signed(t, router, Caller, raw, query, nonce)
		if w.Code != http.StatusOK {
			t.Fatalf("snapshot=%d %s", w.Code, w.Body.String())
		}
		if replay := signed(t, router, Caller, raw, query, nonce); replay.Code != http.StatusUnauthorized {
			t.Fatalf("replayed nonce=%d", replay.Code)
		}
		if strings.Contains(w.Body.String(), "doc-secret") || strings.Contains(w.Body.String(), "subject-secret") || strings.Contains(w.Body.String(), "free text") {
			t.Fatalf("snapshot leaks: %s", w.Body.String())
		}
		var page adminevents.SnapshotPage
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if err := page.Validate(Source); err != nil || page.EntityType != EntityType {
			t.Fatalf("page invalid: %v", err)
		}
		for _, ev := range page.Events {
			encoded, _ := json.Marshal(ev)
			decoded, err := adminevents.Decode(encoded)
			if err != nil {
				t.Fatal(err)
			}
			adapter, _ := adminevents.Lookup(decoded.SourceService, decoded.EventType)
			if _, err := adapter.Minimize(decoded); err != nil {
				t.Fatalf("minimize: %v", err)
			}
			if ev.Scope.Tenant != "T1" {
				t.Fatal("approved allocation scope missing")
			}
			if err := guard.Export(ctx, outbox.Record{ID: ev.EventID, EventType: ev.EventType, Payload: encoded}); err != nil {
				t.Fatalf("authorized relay: %v", err)
			}
			forged := ev
			forged.Scope.Tenant = "foreign"
			if err := checkGuard(ctx, pool, guard, forged); err == nil {
				t.Fatal("foreign allocation accepted")
			}
			seen[ev.EntityRef] = true
		}
		if page.Complete {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatalf("snapshot covered %d exportable cases, want 3", len(seen))
	}
	// Ordinary source updates emit the allocated scope without altering owner identity.
	var first string
	for ref := range seen {
		first = ref
		break
	}
	if _, err := pool.Exec(ctx, `UPDATE vendor_verification_cases SET reason_code='RECHECK' WHERE id=$1::uuid`, first); err != nil {
		t.Fatal(err)
	}
	var tenant string
	if err := pool.QueryRow(ctx, `SELECT payload->'scope'->>'tenant' FROM admin_event_outbox WHERE aggregate_id=$1 ORDER BY created_at DESC LIMIT 1`, first).Scan(&tenant); err != nil || tenant != "T1" {
		t.Fatalf("live trigger lost approved scope: %q %v", tenant, err)
	}
	deniedPolicy := testfixture.New(t, adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "T1"}, Source, "verification.vendor_cases.read", "verification.admin-resources.v1")
	original := guard.Owner.Policy
	guard.Owner.Policy = deniedPolicy.Engine
	if _, err := h.Page(ctx, "", 100); err == nil {
		t.Fatal("feed without signed hq_event_sync purpose accepted")
	}
	guard.Owner.Policy = original
	revoke()
	if _, err := h.Page(ctx, "", 100); err == nil {
		t.Fatal("revoked allocation snapshot accepted")
	}

}

// This fixture uses actual source records, Ed25519 signed owner contexts,
// append-only context import and a separately signed feed-purpose policy.
func approvedGuard(t *testing.T, pool *pgxpool.Pool) (ScopeGuard, func()) {
	t.Helper()
	ctx := context.Background()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "T1"}
	authority := admincontext.Authority{Owner: Source, Environment: adminpolicy.TestOnly, Keys: map[string]ed25519.PublicKey{"owner-test": pub}}
	source := adminresources.Source()
	store := &admincontext.Store{DB: pool, Authority: authority, Reader: source}
	policy := testfixture.New(t, scope, Source, "verification.vendor_cases.read", "verification.admin-resources.v1")
	policy.Document.Rules[0].Purposes = []string{"hq_event_sync"}
	policy.Sign(t)
	guard := ScopeGuard{Owner: &adminowner.Handler{Owner: Source, DB: pool, Contexts: store, Policy: policy.Engine}}
	rows, err := pool.Query(ctx, `SELECT id::text FROM vendor_verification_cases`)
	if err != nil {
		t.Fatal(err)
	}
	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var approvals []admincontext.Approval
	sign := func(a admincontext.Approval) []byte {
		raw, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(admincontext.Envelope{Document: raw, KeyID: "owner-test", Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, append([]byte(admincontext.SigningDomain), raw...)))})
		if err != nil {
			t.Fatal(err)
		}
		return envelope
	}
	for _, ref := range refs {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		binding, err := source.Binding(ctx, tx, "vendor-cases", ref)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		a := admincontext.Approval{SchemaVersion: 1, ID: "test-" + ref, Revision: 1, Owner: Source, Environment: adminpolicy.TestOnly, State: "APPROVED", ResourceKind: "vendor-cases", ResourceRef: ref, SourceContract: "verification.admin-resources.v1", BindingFingerprint: binding.Fingerprint, Scope: scope, Capabilities: []string{"verification.vendor_cases.read"}, ProvenanceRef: "TEST_ONLY_ALLOCATION", ApprovalRef: "TEST_ONLY_OWNER_APPROVAL", EffectiveAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour)}
		if err := store.Import(ctx, sign(a), time.Now()); err != nil {
			t.Fatal(err)
		}
		approvals = append(approvals, a)
	}
	return guard, func() {
		for _, a := range approvals {
			a.Revision++
			a.State = "REVOKED"
			if err := store.Import(ctx, sign(a), time.Now()); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func checkGuard(ctx context.Context, pool *pgxpool.Pool, guard ScopeGuard, ev adminevents.SourceEvent) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	return guard.Check(ctx, tx, ev)
}

func TestScopeGuardAndPageFailClosedWithoutOwner(t *testing.T) {
	guard := ScopeGuard{}
	if guard.Configured() {
		t.Fatal("unconfigured owner accepted")
	}
	if !errors.Is(guard.Check(context.Background(), nil, adminevents.SourceEvent{}), ErrScope) {
		t.Fatal("missing owner accepted")
	}
	if !errors.Is(guard.Export(context.Background(), outbox.Record{}), ErrScope) {
		t.Fatal("missing owner relay accepted")
	}
	if _, err := (&Handler{}).Page(context.Background(), "", 10); !errors.Is(err, ErrConfiguration) {
		t.Fatal("missing guard snapshot accepted")
	}
}
