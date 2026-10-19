package adminowner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ownerFixture struct {
	DB        *pgxpool.Pool
	Handler   *Handler
	Engine    *gin.Engine
	Issuer    *contextfixture.Fixture
	Query     adminread.Query
	Refs      []string
	Approvals []admincontext.Approval
}

func ownerDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ADMIN_OWNER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ADMIN_OWNER_TEST_DATABASE_URL required for real owner HTTP/SQL tests")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.Contains(config.ConnConfig.Database, "test") {
		t.Fatal("disposable test database required")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "owner_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); root.Close() })
	if _, err := pool.Exec(ctx, admincontext.Schema+Schema+`;CREATE TABLE owner_test_resources(id text PRIMARY KEY,organization_id text NOT NULL,status text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),onboarding_status text NOT NULL DEFAULT 'complete',verification_status text NOT NULL DEFAULT 'verified',analytics_consent boolean,marketing_consent boolean,private_notes text)`); err != nil {
		t.Fatal(err)
	}
	return pool
}
func newOwnerFixture(t *testing.T) *ownerFixture {
	t.Helper()
	db := ownerDB(t)
	profile, _ := ForOwner("customer-service")
	definition := profile.Definitions["customers"]
	source := &SQLSource{Specs: map[string]SQLSpec{"customers": {Definition: definition, Table: "owner_test_resources", StateColumn: "status", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, Dimensions: map[string]string{"onboarding": "onboarding_status", "verification": "verification_status", "analytics_consent": "analytics_consent", "marketing_consent": "marketing_consent"}}}}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}
	policy := policyfixture.New(t, scope, profile.Owner, definition.Capability, definition.SourceContract)
	issuer := contextfixture.New(t, profile.Owner)
	store := &admincontext.Store{DB: db, Authority: issuer.Authority, Reader: source}
	h := &Handler{Owner: profile.Owner, Path: profile.Path, DB: db, Source: source, Contexts: store, Policy: policy.Engine, Secret: bytes.Repeat([]byte{41}, 32), Enabled: true, Definitions: map[string]Definition{"customers": definition}, PublishEnabled: true, PublishSecret: bytes.Repeat([]byte{42}, 32)}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	h.Register(engine)
	f := &ownerFixture{DB: db, Handler: h, Engine: engine, Issuer: issuer, Query: adminread.Query{Scope: adminread.Scope{Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}}
	for _, ref := range []string{"customer-a", "customer-b"} {
		if _, err := db.Exec(context.Background(), `INSERT INTO owner_test_resources(id,organization_id,status,created_at,analytics_consent,marketing_consent,private_notes) VALUES($1,'O1','active',now()-interval '5 minutes',true,false,'PRIVATE_PHONE_NOTES')`, ref); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		binding, err := source.Binding(context.Background(), tx, "customers", ref)
		tx.Rollback(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		approval := issuer.Approval("approval-"+ref, "customers", ref, definition.Capability, binding, scope)
		if err := store.Import(context.Background(), issuer.Sign(t, approval), time.Now()); err != nil {
			t.Fatal(err)
		}
		f.Refs = append(f.Refs, ref)
		f.Approvals = append(f.Approvals, approval)
	}
	return f
}
func (f *ownerFixture) request(t *testing.T, q adminread.Query, ref string) *http.Request {
	t.Helper()
	path := f.Handler.Path + "/resources/customers"
	if ref != "" {
		path += "/" + ref
	}
	req := httptest.NewRequest(http.MethodGet, path+"?"+q.Values().Encode(), nil)
	if err := serviceauth.SignRequest(req, adminread.Caller, f.Handler.Secret, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	return req
}
func (f *ownerFixture) read(t *testing.T, q adminread.Query, ref string) *httptest.ResponseRecorder {
	t.Helper()
	res := httptest.NewRecorder()
	f.Engine.ServeHTTP(res, f.request(t, q, ref))
	return res
}

func TestPostgresOwnerReadScopedPaginationRevocationAndMinimization(t *testing.T) {
	f := newOwnerFixture(t)
	res := f.read(t, f.Query, "")
	if res.Code != 200 {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var first Snapshot
	if json.Unmarshal(res.Body.Bytes(), &first) != nil || len(first.Items) != 1 || first.Items[0].Reference != "customer-a" || first.NextCursor == nil {
		t.Fatalf("bad page: %s", res.Body.String())
	}
	if strings.Contains(res.Body.String(), "PRIVATE_PHONE_NOTES") || first.Items[0].SourceVersion != nil || first.Items[0].Dimensions[3].Flag == nil || *first.Items[0].Dimensions[3].Flag {
		t.Fatal("private field, fabricated version or false/unknown conflated")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(*first.NextCursor)
	if err != nil || bytes.Contains(ciphertext, []byte("customer-a")) || bytes.Contains(ciphertext, []byte(`"after"`)) {
		t.Fatal("cursor exposes owner references")
	}
	next := f.Query
	next.Cursor = *first.NextCursor
	res = f.read(t, next, "")
	var second Snapshot
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &second) != nil || len(second.Items) != 1 || second.Items[0].Reference != "customer-b" {
		t.Fatalf("pagination failed: %d %s", res.Code, res.Body.String())
	}
	changed := next
	changed.Staff = "another-staff"
	if res := f.read(t, changed, ""); res.Code != 400 {
		t.Fatalf("cursor rebound to staff: %d", res.Code)
	}
	changed = next
	changed.Scope.Tenant = "O2"
	if res := f.read(t, changed, ""); res.Code != 403 {
		t.Fatalf("tenant changed authority: %d", res.Code)
	}
	revoked := f.Approvals[0]
	revoked.Revision++
	revoked.State = "REVOKED"
	if err := f.Handler.Contexts.Import(context.Background(), f.Issuer.Sign(t, revoked), time.Now()); err != nil {
		t.Fatal(err)
	}
	if res := f.read(t, f.Query, f.Refs[0]); res.Code != 404 {
		t.Fatalf("revoked context remained readable: %d", res.Code)
	}
	if _, err := f.DB.Exec(context.Background(), `UPDATE owner_test_resources SET organization_id='O2' WHERE id='customer-b'`); err != nil {
		t.Fatal(err)
	}
	if res := f.read(t, f.Query, f.Refs[1]); res.Code != 404 {
		t.Fatalf("canonical tenant reassignment ignored: %d", res.Code)
	}
}

func TestPostgresOwnerReadNonceAndAuditFailureAcrossReplicas(t *testing.T) {
	f := newOwnerFixture(t)
	request := f.request(t, f.Query, f.Refs[0])
	var wg sync.WaitGroup
	statuses := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			replica := *f.Handler
			engine := gin.New()
			replica.Register(engine)
			req := request.Clone(context.Background())
			res := httptest.NewRecorder()
			engine.ServeHTTP(res, req)
			statuses <- res.Code
		}()
	}
	wg.Wait()
	close(statuses)
	wins := 0
	for status := range statuses {
		if status == 200 {
			wins++
		} else if status != 409 {
			t.Fatalf("unexpected nonce status %d", status)
		}
	}
	if wins != 1 {
		t.Fatalf("nonce accepted %d times", wins)
	}
	if _, err := f.DB.Exec(context.Background(), `CREATE FUNCTION test_owner_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test-only'; END $$;CREATE TRIGGER test_owner_audit_failure BEFORE INSERT ON admin_owner_read_audit FOR EACH ROW EXECUTE FUNCTION test_owner_audit_failure()`); err != nil {
		t.Fatal(err)
	}
	res := f.read(t, f.Query, f.Refs[0])
	if res.Code != 503 || strings.Contains(res.Body.String(), f.Refs[0]) {
		t.Fatalf("audit failure disclosed data: %d %s", res.Code, res.Body.String())
	}
	for _, sql := range []string{`UPDATE admin_owner_read_audit SET staff_ref='rewrite'`, `DELETE FROM admin_owner_read_audit`, `TRUNCATE admin_owner_read_audit`} {
		if _, err := f.DB.Exec(context.Background(), sql); err == nil {
			t.Fatal("audit evidence was mutable")
		}
	}
}

func TestPostgresOwnerContextPublisherCannotSelfApproveOrOverrideCanonicalTenant(t *testing.T) {
	f := newOwnerFixture(t)
	post := func(caller string, secret, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, f.Handler.Path+"/authorization-contexts", bytesReader(body))
		req.Header.Set("Content-Type", "application/json")
		if err := serviceauth.SignRequest(req, caller, secret, body, time.Now()); err != nil {
			t.Fatal(err)
		}
		res := httptest.NewRecorder()
		f.Engine.ServeHTTP(res, req)
		return res
	}
	a := f.Approvals[0]
	a.Revision++
	a.ApprovalRef = "TEST_ONLY_REVIEWED_REVISION"
	if res := post(adminread.Caller, f.Handler.Secret, f.Issuer.Sign(t, a)); res.Code != 401 {
		t.Fatalf("bridge acted as owner publisher: %d", res.Code)
	}
	if res := post(ContextPublisher, f.Handler.PublishSecret, []byte(`{"document":{"state":"APPROVED"},"key_id":"chosen-by-staff","signature":"forged"}`)); res.Code != 403 {
		t.Fatalf("publisher self-approved: %d", res.Code)
	}
	wrong := a
	wrong.Scope.Tenant = "O2"
	if res := post(ContextPublisher, f.Handler.PublishSecret, f.Issuer.Sign(t, wrong)); res.Code != 403 {
		t.Fatalf("signed tenant overrode owner record: %d", res.Code)
	}
	if res := post(ContextPublisher, f.Handler.PublishSecret, f.Issuer.Sign(t, a)); res.Code != 200 {
		t.Fatalf("approved owner context not recorded: %d %s", res.Code, res.Body.String())
	}
	if res := post(ContextPublisher, f.Handler.PublishSecret, f.Issuer.Sign(t, f.Approvals[0])); res.Code != 403 {
		t.Fatalf("old revision replayed: %d", res.Code)
	}
}

func TestPostgresContextPreparationRequiresOwnerWorkloadAndImmutableAudit(t *testing.T) {
	f := newOwnerFixture(t)
	prepare := func(caller string, secret []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", f.Handler.Path+"/authorization-contexts/bindings/customers/"+f.Refs[0]+"?purpose=case-1", nil)
		if err := serviceauth.SignRequest(req, caller, secret, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		res := httptest.NewRecorder()
		f.Engine.ServeHTTP(res, req)
		return res
	}
	if res := prepare(adminread.Caller, f.Handler.Secret); res.Code != 401 {
		t.Fatalf("HQ read caller prepared source context: %d", res.Code)
	}
	res := prepare(ContextPublisher, f.Handler.PublishSecret)
	if res.Code != 200 || strings.Contains(res.Body.String(), "PRIVATE_") || !strings.Contains(res.Body.String(), "DRAFT_PENDING_OWNER_APPROVAL") {
		t.Fatalf("source preparation: %d %s", res.Code, res.Body.String())
	}
	for _, sql := range []string{`UPDATE admin_owner_context_preparation_audit SET purpose_code='rewrite'`, `DELETE FROM admin_owner_context_preparation_audit`, `TRUNCATE admin_owner_context_preparation_audit`} {
		if _, err := f.DB.Exec(context.Background(), sql); err == nil {
			t.Fatal("source preparation evidence mutable")
		}
	}
	if _, err := f.DB.Exec(context.Background(), `CREATE FUNCTION test_preparation_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test-only'; END $$;CREATE TRIGGER test_preparation_audit_failure BEFORE INSERT ON admin_owner_context_preparation_audit FOR EACH ROW EXECUTE FUNCTION test_preparation_audit_failure()`); err != nil {
		t.Fatal(err)
	}
	if res := prepare(ContextPublisher, f.Handler.PublishSecret); res.Code != 503 || strings.Contains(res.Body.String(), "fingerprint") {
		t.Fatalf("audit failure disclosed preparation: %d %s", res.Code, res.Body.String())
	}
}
