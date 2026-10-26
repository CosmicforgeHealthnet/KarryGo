package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/admincommands"
	"cosmicforge/logistics/services/vendor-service/internal/features/adminresources"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type vendorAdminFixture struct {
	f        section17EFixture
	svc      *admincommand.Service
	actor    adminpolicy.Actor
	scope    adminpolicy.Scope
	contexts *admincontext.Store
	issuer   *contextfixture.Fixture
}

func newVendorAdminFixture(t *testing.T) vendorAdminFixture {
	t.Helper()
	return vendorAdminFixtureFor(t, newSection17EFixture(t, true))
}
func vendorAdminFixtureFor(t *testing.T, f section17EFixture) vendorAdminFixture {
	t.Helper()
	ctx := context.Background()
	scope := adminpolicy.Scope{Product: "karryam", Market: "TEST-MARKET", Zone: "TEST-ZONE", Organization: "test-approved-org", Tenant: "test-approved-tenant"}
	spec := admincommands.Spec()
	policy := policyfixture.New(t, scope, spec.Owner, spec.Capability, spec.Contract)
	policy.Document.ID = "section11-" + uuid.NewString()
	rule := policy.Document.Rules[0]
	rule.Capability, rule.Permission = spec.ReadCapability, spec.ReadCapability
	policy.Document.Rules = append(policy.Document.Rules, rule)
	for _, definition := range adminowner.VendorProfile().Definitions {
		resourceRule := policy.Document.Rules[0]
		resourceRule.Capability, resourceRule.Permission, resourceRule.SourceContract = definition.Capability, definition.Capability, definition.SourceContract
		policy.Document.Rules = append(policy.Document.Rules, resourceRule)
	}
	policy.Sign(t)
	issuer := contextfixture.New(t, spec.Owner)
	store := &admincontext.Store{DB: f.env.pool, Authority: issuer.Authority, Reader: adminresources.Source()}
	// Explicit test-only native enrollment. No runtime permission/role is seeded.
	role := "section11-test-role-" + f.env.suffix
	f.env.exec(ctx, `INSERT INTO vendor_roles(id,vendor_id,code,name) VALUES($1,$2,$1,'Test only native administration')`, role, f.env.businessA)
	f.env.exec(ctx, `INSERT INTO vendor_role_permissions(role_id,permission_id) SELECT $1,id FROM vendor_permissions`, role)
	f.env.exec(ctx, `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id) VALUES($1,$2,$3)`, uuid.NewString(), f.staffA, role)
	branchRole := "section11-branch-reader-" + f.env.suffix
	f.env.exec(ctx, `INSERT INTO vendor_roles(id,vendor_id,code,name) VALUES($1,$2,$1,'Test-only branch read')`, branchRole, f.env.businessB)
	f.env.exec(ctx, `INSERT INTO vendor_role_permissions(role_id,permission_id) SELECT $1,id FROM vendor_permissions WHERE code='vendor.branches.read'`, branchRole)
	f.env.exec(ctx, `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id) VALUES($1,$2,$3,$4)`, uuid.NewString(), f.staffB, branchRole, f.env.branchB)
	var capability string
	if err := f.env.pool.QueryRow(ctx, `INSERT INTO vendor_capabilities(id,business_id,capability_type,status) VALUES($1,$2,'SHOP_GROCERY','ACTIVE') ON CONFLICT(business_id,capability_type) DO UPDATE SET status='ACTIVE' RETURNING id`, "section11-cap-"+uuid.NewString(), f.env.businessA).Scan(&capability); err != nil {
		t.Fatal(err)
	}
	f.env.exec(ctx, `INSERT INTO vendor_branch_capabilities(id,business_id,branch_id,vendor_capability_id,capability_type,status) VALUES($1,$2,$3,$4,'SHOP_GROCERY','ACTIVE') ON CONFLICT DO NOTHING`, "section11-branch-cap-"+uuid.NewString(), f.env.businessA, f.env.branchA, capability)
	evaluator := staffusecases.NewAuthorizationEvaluator(staffrepositories.NewAssignmentRepository(f.env.pool))
	native := vendors.NewService(f.env.pool, vendorAuthorizationAdapter{evaluator: evaluator}, nil, events.NewDomainPublisher(events.NewOutboxRepository(f.env.pool)))
	executor := admincommands.Executor{Vendor: native}
	actor := adminpolicy.Actor{Product: "karryam", Staff: f.staffA, Roles: []string{"staff"}, Permissions: []string{spec.Capability, spec.ReadCapability}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}}
	svc := &admincommand.Service{Spec: spec, DB: f.env.pool, Policy: policy.Engine, Contexts: store, Executor: executor, WriteEnabled: true, ReadEnabled: true}
	return vendorAdminFixture{f, svc, actor, scope, store, issuer}
}
func (e vendorAdminFixture) enroll(t *testing.T, kind, id string) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.f.env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := adminresources.Source().Binding(ctx, tx, kind, id)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a := e.issuer.Approval("section11-"+uuid.NewString(), kind, id, e.svc.Spec.Capability, binding, e.scope)
	a.Capabilities = append(a.Capabilities, e.svc.Spec.ReadCapability)
	a.Capabilities = append(a.Capabilities, adminowner.VendorProfile().Definitions[kind].Capability)
	if err = e.contexts.Import(ctx, e.issuer.Sign(t, a), time.Now()); err != nil {
		t.Fatal(err)
	}
}
func (e vendorAdminFixture) cmd(action, kind, id, caseRef string, version int64, params map[string]string) admincommand.Command {
	c := admincommand.Command{ContractVersion: e.svc.Spec.Contract, OperationID: uuid.NewString(), SubjectKind: kind, SubjectID: id, CaseRef: caseRef, Action: action, ExpectedVersion: version, IdempotencyKey: uuid.NewString(), ReasonCode: "OPERATIONAL_REVIEW", Params: params, RequestedAt: time.Now().UTC().Add(-time.Second)}
	if e.svc.Spec.Actions[action].RequiresApproval {
		c.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: "test-independent-reviewer", IntentDigest: c.IntentDigest(e.actor.Staff, "case-1", e.scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
	}
	return c
}
func TestVendorSecureAdminCasesAndNativeMutations(t *testing.T) {
	e := newVendorAdminFixture(t)
	ctx := context.Background()
	e.enroll(t, "branches", e.f.env.branchA)
	c := e.cmd("PAUSE_BRANCH", "branches", e.f.env.branchA, "", 1, map[string]string{})
	concurrentCtx, cancelConcurrent := context.WithTimeout(ctx, 30*time.Second)
	defer cancelConcurrent()
	var wg sync.WaitGroup
	receipts := make(chan admincommand.Receipt, 4)
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := e.svc.Execute(concurrentCtx, e.actor, "case-1", e.scope, c)
			receipts <- r
			failures <- err
		}()
	}
	wg.Wait()
	close(receipts)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for r := range receipts {
		if r.ResultingState != "PAUSED" {
			t.Fatal("native pause missing", r)
		}
	}
	var state string
	var version int64
	if err := e.f.env.pool.QueryRow(ctx, `SELECT status,version FROM vendor_branches WHERE id=$1`, e.f.env.branchA).Scan(&state, &version); err != nil || state != "PAUSED" || version != 2 {
		t.Fatal("duplicate native effect", state, version, err)
	}
	stale := e.cmd("PAUSE_BRANCH", "branches", e.f.env.branchA, "", 1, map[string]string{})
	if _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, stale); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("stale source version accepted", err)
	}
	caseID := "section11-case-" + uuid.NewString()
	for i, step := range []struct {
		action string
		params map[string]string
		state  string
	}{
		{"OPEN_INVESTIGATION", map[string]string{"category": "BRANCH_ACTIVATION_REVIEW", "severity": "HIGH"}, "OPEN"},
		{"ESCALATE_INVESTIGATION", map[string]string{"destination": "VENDOR_OPERATIONS"}, "ESCALATED"},
		{"RECORD_FINDING", map[string]string{"finding": "SUBSTANTIATED", "evidence_ref": "test-modern-evidence"}, "REVIEWED"},
		{"RECORD_SELLER_APPEAL", map[string]string{"appeal_reference": "native-seller-appeal"}, "APPEALED"},
		{"DECIDE_SELLER_APPEAL", map[string]string{"decision": "UPHELD", "evidence_ref": "test-review-record"}, "APPEAL_DECIDED"},
		{"CLOSE_INVESTIGATION", map[string]string{"closure": "REFERRED_TO_OWNER"}, "CLOSED"},
	} {
		cmd := e.cmd(step.action, "branches", e.f.env.branchA, caseID, int64(i), step.params)
		r, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, cmd)
		if err != nil || r.ResultingState != step.state {
			t.Fatal(step.action, r, err)
		}
	}
	if err := e.f.env.pool.QueryRow(ctx, `SELECT status FROM vendor_branches WHERE id=$1`, e.f.env.branchA).Scan(&state); err != nil || state != "PAUSED" {
		t.Fatal("appeal manufactured reopening", state, err)
	}
	if _, err := e.f.env.pool.Exec(ctx, `DELETE FROM vendor_admin_case_history WHERE case_ref=$1`, caseID); err == nil {
		t.Fatal("mutable case history")
	}
	bad := e.cmd("REOPEN_BRANCH", "branches", e.f.env.branchA, "", 2, map[string]string{})
	bad.Approval.ApproverRef = e.actor.Staff
	if _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, bad); err == nil {
		t.Fatal("self approval accepted")
	}
	reopen := e.cmd("REOPEN_BRANCH", "branches", e.f.env.branchA, "", 2, map[string]string{})
	if r, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, reopen); err != nil || r.ResultingState != "ACTIVE" {
		t.Fatal("native approved reopening", r, err)
	}
	// A formal onboarding process with no current verification denies reopening.
	pause := e.cmd("PAUSE_BRANCH", "branches", e.f.env.branchA, "", 3, map[string]string{})
	if _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, pause); err != nil {
		t.Fatal(err)
	}
	e.f.env.exec(ctx, `INSERT INTO vendor_onboarding_profiles(business_id,status) VALUES($1,'IN_PROGRESS') ON CONFLICT(business_id) DO UPDATE SET status='IN_PROGRESS'`, e.f.env.businessA)
	restricted := e.cmd("REOPEN_BRANCH", "branches", e.f.env.branchA, "", 4, map[string]string{})
	if _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, restricted); err == nil {
		t.Fatal("unverified reopening allowed")
	}
	// Current native branch membership is independently rechecked on readback.
	e.f.env.exec(ctx, `UPDATE vendor_staff SET status='suspended' WHERE id=$1`, e.actor.Staff)
	if _, err := e.svc.Readback(ctx, e.actor, "case-1", e.scope, "branches", e.f.env.branchA, c.IdempotencyKey); err == nil {
		t.Fatal("revoked native actor readback accepted")
	}
}
func TestVendorSecureAdminNativeRowsAndBranchAuthorization(t *testing.T) {
	e := newVendorAdminFixture(t)
	ctx := context.Background()
	source := adminresources.Source()
	tx, err := e.f.env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, r := range []struct{ kind, id string }{{"merchants", e.f.env.businessA}, {"branches", e.f.env.branchA}, {"shop-products", e.f.env.productA}} {
		item, err := source.Read(ctx, tx, r.kind, r.id, true)
		if err != nil {
			t.Fatal(r, err)
		}
		p := adminowner.VendorProfile()
		if err = adminowner.ValidateResource(item, p.Definitions[r.kind], true); err != nil {
			t.Fatal(r, err)
		}
		if r.kind == "merchants" || r.kind == "branches" || r.kind == "shop-products" {
			if item.Label == nil || *item.Label == "" {
				t.Fatal("missing commercial directory label")
			}
			copy := item
			bad := "unsafe\nlabel"
			copy.Label = &bad
			if adminowner.ValidateResource(copy, p.Definitions[r.kind], true) == nil {
				t.Fatal("control character label accepted")
			}
			definition := p.Definitions[r.kind]
			definition.Label = false
			if adminowner.ValidateResource(item, definition, true) == nil {
				t.Fatal("label bypassed source field allowlist")
			}
		}

		binding, err := source.Binding(ctx, tx, r.kind, r.id)
		if err != nil {
			t.Fatal(err)
		}
		if binding.Canonical.Organization != "" || binding.Canonical.Tenant != "" {
			t.Fatal("invented merchant-to-organization mapping")
		}
	}
	// The complete finite SQL dictionary compiles against actual native migrations.
	for kind := range adminowner.VendorProfile().Definitions {
		_, err := source.Read(ctx, tx, kind, "missing-section11-record", false)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal(kind, err)
		}
	}
	if err = source.AuthorizeStaff(ctx, tx, e.f.staffB, "branches", e.f.env.branchA); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cross merchant actor allowed", err)
	}
	if err = source.AuthorizeStaff(ctx, tx, e.f.staffB, "merchants", e.f.env.businessB); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("branch-only actor gained merchant-wide access", err)
	}
	if err = source.AuthorizeStaff(ctx, tx, e.f.staffB, "branches", e.f.env.branchB); err != nil {
		t.Fatal("own branch read missing", err)
	}
	// Signed native HTTP/typed client, with current owner allocation and audit.
	tx.Rollback(ctx)
	e.enroll(t, "branches", e.f.env.branchA)
	resourceKey := []byte(strings.Repeat("s", 32))
	resourceRouter := gin.New()
	(&adminowner.Handler{Owner: adminresources.Owner, Path: adminowner.VendorProfile().Path, DB: e.f.env.pool, Source: source, Contexts: e.contexts, Policy: e.svc.Policy, Secret: resourceKey, Enabled: true, Definitions: adminowner.VendorProfile().Definitions}).Register(resourceRouter)
	query := adminread.Query{Scope: adminread.Scope{Market: e.scope.Market, Zone: e.scope.Zone, Organization: e.scope.Organization, Tenant: e.scope.Tenant}, Staff: e.actor.Staff, Role: "staff", Purpose: "case-1", PolicyID: e.svc.Policy.ID(), PolicyVersion: e.svc.Policy.Version(), PolicyChecksum: e.svc.Policy.Checksum(), Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Minute), Limit: 1}
	request := httptest.NewRequest("GET", "/internal/vendor/v1/admin/resources/branches/"+e.f.env.branchA+"?"+query.Values().Encode(), nil)
	if err = serviceauth.SignRequest(request, adminread.Caller, resourceKey, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	resourceRouter.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal("native resource boundary", response.Code, response.Body.String())
	}
	var snapshot adminowner.Snapshot
	if err = json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil || len(snapshot.Items) != 1 || snapshot.Items[0].Reference != e.f.env.branchA {
		t.Fatal("native snapshot contract", snapshot, err)
	}
	if snapshot.Items[0].Label == nil || *snapshot.Items[0].Label == "" {
		t.Fatal("directory label missing")
	}
	captureVendorAdminWire(t, "vendor-resources.json", snapshot)
	replay := httptest.NewRecorder()
	replayRequest := httptest.NewRequest("GET", request.URL.String(), nil)
	replayRequest.Header = request.Header.Clone()
	resourceRouter.ServeHTTP(replay, replayRequest)
	if replay.Code != 409 {
		t.Fatal("native resource replay", replay.Code)
	}
	write, read := []byte(strings.Repeat("w", 32)), []byte(strings.Repeat("r", 32))
	router := gin.New()
	(&admincommand.Handler{Service: e.svc, WriteSecret: write, ReadSecret: read}).Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := admincommand.NewClient(e.svc.Spec.Owner, server.URL, write, read)
	if err != nil {
		t.Fatal(err)
	}
	q := adminread.Query{Scope: adminread.Scope{Market: e.scope.Market, Zone: e.scope.Zone, Organization: e.scope.Organization, Tenant: e.scope.Tenant}, Staff: e.actor.Staff, Role: "staff", Purpose: "case-1", PolicyID: e.svc.Policy.ID(), PolicyVersion: e.svc.Policy.Version(), PolicyChecksum: e.svc.Policy.Checksum(), Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1}
	c := e.cmd("PAUSE_BRANCH", "branches", e.f.env.branchA, "", 1, map[string]string{})
	r, err := client.Execute(ctx, q, c)
	if err != nil || r.ResultingState != "PAUSED" {
		t.Fatal("native signed command", r, err)
	}
	captureVendorAdminWire(t, "vendor-command-receipt.json", r)
}

func TestVendorSecureAdminAuditFailureRollsBackNativeEffect(t *testing.T) {
	e := newVendorAdminFixture(t)
	ctx := context.Background()
	e.enroll(t, "branches", e.f.env.branchA)
	e.f.env.exec(ctx, `CREATE OR REPLACE FUNCTION section11_test_ledger_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'TEST_ONLY_LEDGER_FAILURE'; END $$;CREATE TRIGGER section11_test_ledger_failure BEFORE INSERT ON admin_owner_command_decisions FOR EACH STATEMENT EXECUTE FUNCTION section11_test_ledger_failure()`)
	defer e.f.env.pool.Exec(ctx, `DROP TRIGGER IF EXISTS section11_test_ledger_failure ON admin_owner_command_decisions;DROP FUNCTION IF EXISTS section11_test_ledger_failure()`)
	c := e.cmd("PAUSE_BRANCH", "branches", e.f.env.branchA, "", 1, map[string]string{})
	if _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, c); err == nil {
		t.Fatal("audit failure reported success")
	}
	var state string
	var version int64
	if err := e.f.env.pool.QueryRow(ctx, `SELECT status,version FROM vendor_branches WHERE id=$1`, e.f.env.branchA).Scan(&state, &version); err != nil || state != "ACTIVE" || version != 1 {
		t.Fatal("native effect survived missing receipt", state, version, err)
	}
}
