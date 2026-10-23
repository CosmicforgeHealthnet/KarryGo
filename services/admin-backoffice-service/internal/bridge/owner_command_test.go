package bridge

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/ownercommandops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type standingReader struct{ contract string }

func (r standingReader) Binding(ctx context.Context, tx pgx.Tx, kind, ref string) (admincontext.RecordBinding, error) {
	contract := standingReaderContract(r.contract)
	b := admincontext.RecordBinding{SourceContract: contract}
	err := tx.QueryRow(ctx, `SELECT fingerprint,tenant_ref FROM e2e_subjects WHERE kind=$1 AND id=$2`, kind, ref).Scan(&b.Fingerprint, &b.Canonical.Tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, admincontext.ErrUnmapped
	}
	return b, err
}

// standingExecutor is a minimal provider standing machine for the Bridge
// transport test only; real owner state machines are certified in each owner.
type standingExecutor struct{ vendor bool }

func (e standingExecutor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	var version int64
	var state string
	if err := tx.QueryRow(ctx, `SELECT version,state FROM e2e_subjects WHERE kind=$1 AND id=$2 FOR UPDATE`, in.SubjectKind, in.SubjectID).Scan(&version, &state); err != nil {
		return admincommand.Transition{}, admincommand.ErrNotFound
	}
	if version != in.ExpectedVersion {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	next := map[string]string{"RESTRICT_PROVIDER": "RESTRICTED", "LIFT_PROVIDER_RESTRICTION": "ACTIVE", "REQUEST_REVERIFICATION": "REVERIFICATION_REQUIRED"}[in.Action]
	if e.vendor {
		next = map[string]string{"PAUSE_BRANCH": "PAUSED", "REOPEN_BRANCH": "ACTIVE"}[in.Action]
	}
	if next == "" || next == state {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if _, err := tx.Exec(ctx, `UPDATE e2e_subjects SET version=version+1,state=$3 WHERE kind=$1 AND id=$2`, in.SubjectKind, in.SubjectID, next); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: version, PriorState: state, ResultingState: next, Outcome: "PROVIDER_" + next}, nil
}
func (standingExecutor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	var v int64
	var s string
	err := tx.QueryRow(ctx, `SELECT version,state FROM e2e_subjects WHERE kind=$1 AND id=$2`, kind, id).Scan(&v, &s)
	return v, s, err
}

func ownerSchema(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ADMIN_BRIDGE_TEST_DATABASE_URL")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	root, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := "owner_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); root.Close() })
	if _, err = pool.Exec(ctx, admincontext.Schema+";"+adminowner.Schema+";"+admincommand.Schema+`;CREATE TABLE e2e_subjects(kind text,id text,fingerprint text,tenant_ref text,version bigint,state text,PRIMARY KEY(kind,id))`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func standingReaderContract(value string) string {
	if value != "" {
		return value
	}
	return "taxi.admin-resources.v1"
}
func TestPostgresOwnerCommandBridgeMTLSRoundTrip(t *testing.T) {
	ownerCommandBridgeMTLSRoundTrip(t, false)
}
func TestPostgresVendorCommandBridgeMTLSRoundTrip(t *testing.T) {
	ownerCommandBridgeMTLSRoundTrip(t, true)
}
func ownerCommandBridgeMTLSRoundTrip(t *testing.T, vendor bool) {
	db := testutil.Database(t)
	owner := ownerSchema(t)
	ctx := context.Background()
	spec, _ := admincommand.ForOwner("driver-taxi-service")
	kind, contract, action, resultState := "providers", "taxi.admin-resources.v1", "RESTRICT_PROVIDER", "RESTRICTED"
	params := map[string]string{"restriction": "SUSPEND_ONLINE"}
	if vendor {
		spec = admincommand.VendorSpec()
		kind, contract, action, resultState = "branches", "vendor.admin-resources.v1", "PAUSE_BRANCH", "PAUSED"
		params = map[string]string{}
	}
	sc := adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}
	p := policyfixture.New(t, sc, spec.Owner, spec.Capability, spec.Contract)
	for _, cap := range []string{spec.ReadCapability, spec.ApproveCapability} {
		rule := p.Document.Rules[0]
		rule.Capability, rule.Permission = cap, cap
		p.Document.Rules = append(p.Document.Rules, rule)
	}
	p.Sign(t)
	subject := uuid.NewString()
	fingerprint := strings.Repeat("cd", 32)
	if _, err := owner.Exec(ctx, `INSERT INTO e2e_subjects VALUES($1,$2,$3,'org-1',0,'ACTIVE')`, kind, subject, fingerprint); err != nil {
		t.Fatal(err)
	}
	issuer := contextfixture.New(t, spec.Owner)
	store := &admincontext.Store{DB: owner, Authority: issuer.Authority, Reader: standingReader{contract: contract}}
	approval := issuer.Approval("ctx-"+subject, kind, subject, spec.Capability, admincontext.RecordBinding{SourceContract: contract, Fingerprint: fingerprint}, sc)
	approval.Capabilities = []string{spec.Capability, spec.ReadCapability}
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	writeKey, readKey := []byte(strings.Repeat("W", 32)), []byte(strings.Repeat("R", 32))
	handler := &admincommand.Handler{Service: &admincommand.Service{Spec: spec, DB: owner, Policy: p.Engine, Contexts: store, Executor: standingExecutor{vendor: vendor}, WriteEnabled: true, ReadEnabled: true}, WriteSecret: writeKey, ReadSecret: readKey}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler.Register(router)
	var ownerPosts atomic.Int32
	ownerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-HQ-Key-ID") != "" {
			t.Error("HQ credentials forwarded to owner")
		}
		if r.Method == "POST" {
			ownerPosts.Add(1)
		}
		router.ServeHTTP(w, r)
	}))
	defer ownerServer.Close()
	client, err := admincommand.NewClient(spec.Owner, ownerServer.URL, writeKey, readKey)
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	f.claims.Permissions = []string{spec.Capability, spec.ReadCapability, spec.ApproveCapability}
	f.b.ConfigureDomainFlags(config.DomainFlags{RemoteWrites: true, TaxiWrites: true, VendorWrites: true})
	f.claims.Zones, f.claims.Tenants = []string{"Z1"}, []string{"org-1"}
	pair, leaf, roots := workloadCertificate(t, f.b.trust.WorkloadURI, false)
	f.cert = leaf
	f.b.repo = &persistence.Store{Pool: db}
	sum := sha256.Sum256(leaf.Raw)
	if _, err := db.Exec(ctx, `INSERT INTO admin_remote_principals(principal,certificate_sha256,workload_uri,product,audience,expires_at) VALUES($1,$2,$3,'karryam',$4,$5)`, config.HQPrincipal, hex.EncodeToString(sum[:]), f.b.trust.WorkloadURI, f.b.trust.Audience, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	service := &ownercommandops.Service{DB: db, Owner: client, Policy: p.Engine, WriteEnabled: true, ReadEnabled: true}
	if err := f.b.ConfigureOwnerCommands(service); err != nil {
		t.Fatal(err)
	}
	if err := f.b.ConfigureOwnerCommands(service); err == nil {
		t.Fatal("owner command service configured twice")
	}
	server := httptest.NewUnstartedServer(f.b)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()
	hq := server.Client()
	hq.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{pair}
	call := func(staff, method, path string, body []byte) (int, map[string]any) {
		t.Helper()
		f.claims.Subject = staff
		request := f.request(t, method, path, body)
		request.URL.Scheme, request.URL.Host, request.RequestURI = "https", strings.TrimPrefix(server.URL, "https://"), ""
		response, err := hq.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		var out map[string]any
		json.Unmarshal(raw, &out)
		return response.StatusCode, out
	}
	result := func(out map[string]any) map[string]any { r, _ := out["result"].(map[string]any); return r }
	q := adminread.Query{Scope: adminread.Scope{Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}, Purpose: "case-1", Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}
	values := q.Values()
	for _, key := range []string{"staff", "role", "policy_id", "policy_version", "policy_checksum"} {
		values.Del(key)
	}
	suffix := "?" + values.Encode()

	approvalID := uuid.NewString()
	proposal, _ := json.Marshal(admincommand.Proposal{ContractVersion: spec.Contract, ApprovalID: approvalID, SubjectKind: kind, SubjectID: subject, Action: action, ExpectedVersion: 0, ReasonCode: "SAFETY_REVIEW", Params: params})
	if code, out := call("staff-1", "POST", "/"+spec.Domain+"/command-approvals"+suffix, proposal); code != 200 || result(out)["status"] != "PENDING" {
		t.Fatal("proposal", code, out)
	}
	decision, _ := json.Marshal(admincommand.ApprovalDecision{Decision: "APPROVE", ReasonCode: "EVIDENCE_REVIEWED"})
	if code, _ := call("staff-1", "POST", "/"+spec.Domain+"/command-approvals/"+approvalID+"/decisions"+suffix, decision); code != 403 {
		t.Fatal("requester approved own proposal", code)
	}
	if code, out := call("staff-2", "POST", "/"+spec.Domain+"/command-approvals/"+approvalID+"/decisions"+suffix, decision); code != 200 || result(out)["status"] != "APPROVED" {
		t.Fatal("independent approval", code, out)
	}
	submission := admincommand.Submission{ContractVersion: spec.Contract, OperationID: uuid.NewString(), SubjectKind: kind, SubjectID: subject, Action: action, ExpectedVersion: 0, IdempotencyKey: uuid.NewString(), ReasonCode: "SAFETY_REVIEW", Params: params, ApprovalID: approvalID, RequestedAt: time.Now().UTC().Add(-time.Second)}
	body, _ := json.Marshal(submission)
	if code, _ := call("staff-2", "POST", "/"+spec.Domain+"/commands"+suffix, body); code != 409 {
		t.Fatal("approver executed the requester's approval", code)
	}
	code, out := call("staff-1", "POST", "/"+spec.Domain+"/commands"+suffix, body)
	if code != 200 || result(out)["status"] != "CONFIRMED" {
		t.Fatal("approved command", code, out)
	}
	var state string
	owner.QueryRow(ctx, `SELECT state FROM e2e_subjects WHERE id=$1`, subject).Scan(&state)
	if state != resultState {
		t.Fatal("owner state not changed by the owner", state)
	}
	if code, out := call("staff-1", "POST", "/"+spec.Domain+"/commands"+suffix, body); code != 200 || result(out)["status"] != "CONFIRMED" || ownerPosts.Load() != 1 {
		t.Fatal("replay re-posted", code, out, ownerPosts.Load())
	}
	if code, out := call("staff-1", "GET", "/"+spec.Domain+"/commands/"+submission.OperationID+suffix, nil); code != 200 || result(out)["status"] != "CONFIRMED" {
		t.Fatal("readback", code, out)
	}
	code, out = call("staff-1", "GET", "/"+spec.Domain+"/command-history/"+kind+"/"+subject+suffix, nil)
	if code != 200 || result(out)["current_state"] != resultState || len(result(out)["decisions"].([]any)) != 1 {
		t.Fatal("history", code, out)
	}
	var staleBody []byte
	if !vendor {
		stale := admincommand.Submission{ContractVersion: spec.Contract, OperationID: uuid.NewString(), SubjectKind: kind, SubjectID: subject, Action: "REQUEST_REVERIFICATION", ExpectedVersion: 0, IdempotencyKey: uuid.NewString(), ReasonCode: "DOCUMENT_REVIEW", Params: map[string]string{"document": "INSURANCE"}, RequestedAt: time.Now().UTC().Add(-time.Second)}
		staleBody, _ = json.Marshal(stale)
		if code, _ := call("staff-1", "POST", "/"+spec.Domain+"/commands"+suffix, staleBody); code != 409 {
			t.Fatal("Bridge did not hold the confirmed version slot", code)
		}
		stale.OperationID, stale.IdempotencyKey, stale.ExpectedVersion = uuid.NewString(), uuid.NewString(), 5
		staleBody, _ = json.Marshal(stale)
		if code, out := call("staff-1", "POST", "/"+spec.Domain+"/commands"+suffix, staleBody); code != 200 || result(out)["status"] != "REJECTED" || result(out)["rejection_code"] != "owner_refused" {
			t.Fatal("stale version not settled as owner refusal", code, out)
		}
	} else {
		staleBody = body
	}

	forged := strings.Replace(string(body), `"approval_id"`, `"approval":{"approval_id":"x"},"approval_id"`, 1)
	if code, _ := call("staff-1", "POST", "/"+spec.Domain+"/commands"+suffix, []byte(forged)); code != 400 {
		t.Fatal("HQ-supplied approval object accepted", code)
	}
	f.claims.Zones = []string{"Z2"}
	if code, _ := call("staff-1", "GET", "/"+spec.Domain+"/commands/"+submission.OperationID+suffix, nil); code != 403 {
		t.Fatal("zone escape", code)
	}
	f.claims.Zones = []string{"Z1"}
	f.b.ConfigureDomainFlags(config.DomainFlags{TaxiWrites: true, VendorWrites: true})
	if code, _ := call("staff-1", "POST", "/"+spec.Domain+"/commands"+suffix, staleBody); code != 503 {
		t.Fatal("global write kill ignored", code)
	}
	if code, _ := call("staff-1", "GET", "/"+spec.Domain+"/commands/"+submission.OperationID+suffix, nil); code != 200 {
		t.Fatal("readback must survive the write kill", code)
	}
	if code, _ := call("staff-1", "POST", "/hauling/commands"+suffix, staleBody); code != 503 {
		t.Fatal("unconfigured owner reachable", code)
	}
	availability := map[string]bool{}
	f.b.ownerCommandAvailability(availability, f.claims.Roles, f.claims.Zones, f.claims.Tenants)
	if availability[spec.Capability] || !availability[spec.ReadCapability] || availability["hauling.admin.command.read"] {
		t.Fatal("discovery availability wrong", availability)
	}
	var audits int
	db.QueryRow(ctx, `SELECT count(*) FROM admin_owner_command_operation_audit`).Scan(&audits)
	if audits < 4 {
		t.Fatal("orchestration not audited", audits)
	}
}
