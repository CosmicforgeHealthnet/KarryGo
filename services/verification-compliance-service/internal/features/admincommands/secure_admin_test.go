package admincommands_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/verification-compliance-service/internal/features/admincommands"
	"cosmicforge/logistics/services/verification-compliance-service/internal/features/adminresources"
	"cosmicforge/logistics/services/verification-compliance-service/internal/testdb"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
)

type harness struct {
	pool    *pgxpool.Pool
	service *admincommand.Service
	scope   adminpolicy.Scope
	issuer  *contextfixture.Fixture
	stores  *admincontext.Store
	spec    admincommand.Spec
}

func grant(t *testing.T, pool *pgxpool.Pool, staff string, permissions ...string) {
	t.Helper()
	for _, p := range permissions {
		if _, err := pool.Exec(context.Background(), `INSERT INTO verification_admin_staff_grants(id,staff_ref,permission,granted_by,approval_ref) VALUES($1,$2,$3,'test-owner','test-only-approval')`, uuid.NewString(), staff, p); err != nil {
			t.Fatal(err)
		}
	}
}

func seedCase(t *testing.T, pool *pgxpool.Pool, business, dimension string) string {
	t.Helper()
	key := "seed-" + uuid.NewString()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO vendor_verification_cases(client_reference,business_id,dimension,subject_type,subject_ref,document_refs,request_fingerprint,idempotency_key) VALUES($1,$2,$3,'BUSINESS',$2,'["doc-secret-ref"]','fp',$1) RETURNING id::text`, key, business, dimension).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO vendor_verification_case_history(case_id,source_version,status,observed_at) VALUES($1::uuid,1,'PENDING_REVIEW',now())`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newHarness(t *testing.T) *harness {
	pool := testdb.Fresh(t)
	scope := adminpolicy.Scope{Product: "karryam", Market: "TEST-MARKET", Zone: "TEST-ZONE", Organization: "TEST-ORGANIZATION", Tenant: "TEST-TENANT"}
	spec := admincommands.Spec()
	definition := adminowner.VerificationProfile().Definitions["vendor-cases"]
	policy := policyfixture.New(t, scope, spec.Owner, spec.Capability, spec.Contract)
	policy.Document.ID = "section18-" + uuid.NewString()
	for _, capability := range []string{spec.ReadCapability, definition.Capability} {
		rule := policy.Document.Rules[0]
		rule.Capability, rule.Permission = capability, capability
		if capability != spec.ReadCapability {
			rule.SourceContract = definition.SourceContract
		}
		policy.Document.Rules = append(policy.Document.Rules, rule)
	}
	policy.Sign(t)
	issuer := contextfixture.New(t, spec.Owner)
	contexts := &admincontext.Store{DB: pool, Authority: issuer.Authority, Reader: adminresources.Source()}
	service := &admincommand.Service{Spec: spec, DB: pool, Policy: policy.Engine, Contexts: contexts, Executor: admincommands.Executor{}, WriteEnabled: true, ReadEnabled: true}
	return &harness{pool: pool, service: service, scope: scope, issuer: issuer, stores: contexts, spec: spec}
}

// authorize publishes the independently signed owner context for one case.
func (h *harness) authorize(t *testing.T, caseID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := adminresources.Source().Binding(ctx, tx, "vendor-cases", caseID)
	_ = tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := h.issuer.Approval("section18-"+uuid.NewString(), "vendor-cases", caseID, h.spec.Capability, binding, h.scope)
	approval.Capabilities = append(approval.Capabilities, h.spec.ReadCapability, adminowner.VerificationProfile().Definitions["vendor-cases"].Capability)
	if err = h.stores.Import(ctx, h.issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
}

func actor(staff string, spec admincommand.Spec, scope adminpolicy.Scope) adminpolicy.Actor {
	return adminpolicy.Actor{Product: "karryam", Staff: staff, Roles: []string{"staff"}, Permissions: []string{spec.Capability, spec.ReadCapability}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}}
}

func (h *harness) command(caseID, action string, version int64, params map[string]string) admincommand.Command {
	if params == nil {
		params = map[string]string{}
	}
	return admincommand.Command{ContractVersion: h.spec.Contract, OperationID: uuid.NewString(), SubjectKind: "vendor-cases", SubjectID: caseID, Action: action, ExpectedVersion: version, IdempotencyKey: uuid.NewString(), ReasonCode: "MANUAL_REVIEW", Params: params, RequestedAt: time.Now().UTC().Add(-time.Second)}
}

func approve(c admincommand.Command, requester, approver string, scope adminpolicy.Scope) admincommand.Command {
	c.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: approver, IntentDigest: c.IntentDigest(requester, "case-1", scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
	return c
}

func TestVerificationSecureAdminOwnerBoundary(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	reviewer := "verification-staff-" + uuid.NewString()
	grant(t, h.pool, reviewer, "verification.vendor_cases.read", adminresources.PermissionReview, adminresources.PermissionDecide)
	reviewerActor := actor(reviewer, h.spec, h.scope)
	caseID := seedCase(t, h.pool, "biz-admin", "FOOD_KITCHEN")
	h.authorize(t, caseID)

	// Missing grant: an otherwise fully policy-authorized staff member with no
	// native grant is denied before any transition.
	outsider := actor("verification-outsider-"+uuid.NewString(), h.spec, h.scope)
	if _, err := h.service.Execute(ctx, outsider, "case-1", h.scope, h.command(caseID, "START_REVIEW", 1, nil)); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatalf("staff without grant accepted: %v", err)
	}
	readOnly := "verification-reader-" + uuid.NewString()
	grant(t, h.pool, readOnly, "verification.vendor_cases.read")
	if _, err := h.service.Execute(ctx, actor(readOnly, h.spec, h.scope), "case-1", h.scope, h.command(caseID, "START_REVIEW", 1, nil)); !errors.Is(err, admincommand.ErrNotFound) {
		t.Fatalf("read-only staff mutated: %v", err)
	}

	// Two reviewers decide the same version concurrently: exactly one wins.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, action := range []string{"START_REVIEW", "REQUEST_REMEDIATION"} {
		params := map[string]string{}
		if action == "REQUEST_REMEDIATION" {
			params["remediation"] = "RESUBMIT_DOCUMENT"
		}
		cmd := h.command(caseID, action, 1, params)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.service.Execute(ctx, reviewerActor, "case-1", h.scope, cmd)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var applied, refused int
	for err := range results {
		switch {
		case err == nil:
			applied++
		case errors.Is(err, admincommand.ErrConflict), errors.Is(err, admincommand.ErrRejected):
			refused++
		default:
			t.Fatalf("unexpected concurrent outcome: %v", err)
		}
	}
	if applied != 1 || refused != 1 {
		t.Fatalf("concurrent reviewers applied=%d refused=%d", applied, refused)
	}
	var version int64
	var status string
	if err := h.pool.QueryRow(ctx, `SELECT source_version,status FROM vendor_verification_cases WHERE id=$1::uuid`, caseID).Scan(&version, &status); err != nil || version != 2 {
		t.Fatalf("version=%d err=%v", version, err)
	}
	if status != "IN_PROGRESS" {
		// The remediation request won the race; take the review back.
		if _, err := h.service.Execute(ctx, reviewerActor, "case-1", h.scope, h.command(caseID, "START_REVIEW", 2, nil)); err != nil {
			t.Fatal(err)
		}
		version = 3
	}

	// Stale version.
	if _, err := h.service.Execute(ctx, reviewerActor, "case-1", h.scope, h.command(caseID, "START_REVIEW", 1, nil)); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}

	// APPROVE_CASE needs an independent approval and a future expiry.
	unapproved := h.command(caseID, "APPROVE_CASE", version, map[string]string{"expires_on": time.Now().AddDate(1, 0, 0).Format("2006-01-02")})
	if _, err := h.service.Execute(ctx, reviewerActor, "case-1", h.scope, unapproved); !errors.Is(err, admincommand.ErrInvalid) {
		t.Fatalf("unapproved approval accepted: %v", err)
	}
	past := approve(h.command(caseID, "APPROVE_CASE", version, map[string]string{"expires_on": time.Now().AddDate(0, 0, -1).Format("2006-01-02")}), reviewer, "independent-reviewer", h.scope)
	if _, err := h.service.Execute(ctx, reviewerActor, "case-1", h.scope, past); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("past expiry accepted: %v", err)
	}
	approval := approve(h.command(caseID, "APPROVE_CASE", version, map[string]string{"expires_on": time.Now().AddDate(1, 0, 0).Format("2006-01-02")}), reviewer, "independent-reviewer", h.scope)
	receipt, err := h.service.Execute(ctx, reviewerActor, "case-1", h.scope, approval)
	if err != nil || receipt.PriorState != "IN_PROGRESS" || receipt.ResultingState != "VERIFIED" || receipt.RecordedVersion != version+1 {
		t.Fatalf("approval failed: %+v %v", receipt, err)
	}
	if replay, err := h.service.Execute(ctx, reviewerActor, "case-1", h.scope, approval); err != nil || replay.DecisionID != receipt.DecisionID {
		t.Fatalf("replay did not return the original receipt: %+v %v", replay, err)
	}
	version++
	var decidedBy, kind string
	if err := h.pool.QueryRow(ctx, `SELECT decided_by,decided_by_kind FROM vendor_verification_case_history WHERE case_id=$1::uuid AND source_version=$2`, caseID, version).Scan(&decidedBy, &kind); err != nil || decidedBy != reviewer || kind != "STAFF" {
		t.Fatalf("history must name the human reviewer: %s %s %v", decidedBy, kind, err)
	}
	var callbacks int
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM verification_outbox WHERE aggregate_id=$1::uuid AND event_type='vendor.verification.status_changed' AND event_version=$2`, caseID, version).Scan(&callbacks); err != nil || callbacks != 1 {
		t.Fatalf("vendor callback rows=%d err=%v", callbacks, err)
	}

	// Illegal transition is a domain refusal, not a conflict.
	reject := approve(h.command(caseID, "REJECT_CASE", version, map[string]string{"reason": "DOCUMENT_MISMATCH"}), reviewer, "independent-reviewer", h.scope)
	if _, err := h.service.Execute(ctx, reviewerActor, "case-1", h.scope, reject); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("VERIFIED -> REJECTED accepted: %v", err)
	}
	suspend := approve(h.command(caseID, "SUSPEND_CASE", version, map[string]string{"reason": "COMPLIANCE_LAPSE"}), reviewer, "independent-reviewer", h.scope)
	if receipt, err = h.service.Execute(ctx, reviewerActor, "case-1", h.scope, suspend); err != nil || receipt.ResultingState != "SUSPENDED" {
		t.Fatalf("suspend failed: %+v %v", receipt, err)
	}
	version++

	// Appeal: self-approval is refused; an independent approval reopens and
	// records the appeal exactly once.
	selfApproved := approve(h.command(caseID, "REOPEN_CASE", version, map[string]string{"appeal_ref": "appeal-001"}), reviewer, reviewer, h.scope)
	if _, err := h.service.Execute(ctx, reviewerActor, "case-1", h.scope, selfApproved); err == nil {
		t.Fatal("self approval accepted")
	}
	reopen := approve(h.command(caseID, "REOPEN_CASE", version, map[string]string{"appeal_ref": "appeal-001"}), reviewer, "independent-reviewer", h.scope)
	if receipt, err = h.service.Execute(ctx, reviewerActor, "case-1", h.scope, reopen); err != nil || receipt.PriorState != "SUSPENDED" || receipt.ResultingState != "PENDING_REVIEW" {
		t.Fatalf("reopen failed: %+v %v", receipt, err)
	}
	var appealStatus, operation string
	if err := h.pool.QueryRow(ctx, `SELECT status,decided_operation_ref FROM vendor_case_appeals WHERE case_id=$1::uuid AND appeal_ref='appeal-001'`, caseID).Scan(&appealStatus, &operation); err != nil || appealStatus != "REOPENED" || operation != reopen.OperationID {
		t.Fatalf("appeal record=%s op=%s err=%v", appealStatus, operation, err)
	}
	if _, err := h.pool.Exec(ctx, `UPDATE vendor_case_appeals SET status='REOPENED'`); err == nil {
		t.Fatal("appeal decision was mutable")
	}

	// History via the command readback, and immutable decision evidence.
	history, err := h.service.History(ctx, reviewerActor, "case-1", h.scope, "vendor-cases", caseID)
	if err != nil || history.CurrentState != "PENDING_REVIEW" || int64(len(history.Decisions)) != version || history.CurrentVersion != version+1 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if _, err := h.pool.Exec(ctx, `DELETE FROM admin_owner_command_decisions WHERE subject_id=$1`, caseID); err == nil {
		t.Fatal("command decisions were deletable")
	}

	// Cross-tenant scope is refused.
	wrong := h.scope
	wrong.Tenant = "OTHER-TENANT"
	if _, err := h.service.Execute(ctx, reviewerActor, "case-1", wrong, h.command(caseID, "START_REVIEW", version+1, nil)); err == nil {
		t.Fatal("cross-tenant command accepted")
	}
}

func TestVerificationAdminResourcesReadMinimizedNativeRows(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	caseID := seedCase(t, h.pool, "biz-read", "BUSINESS")
	if _, err := h.pool.Exec(ctx, `INSERT INTO vendor_case_automation_findings(case_id,case_version,check_code,outcome,severity,evaluated_at) VALUES($1::uuid,1,'EXTERNAL_AUTHORITY','SOURCE_UNVERIFIED','WARN',now())`, caseID); err != nil {
		t.Fatal(err)
	}
	var finding, asset, org string
	if err := h.pool.QueryRow(ctx, `SELECT id::text FROM vendor_case_automation_findings WHERE case_id=$1::uuid`, caseID).Scan(&finding); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx, `INSERT INTO asset_capability_cases(authority,provider_id,asset_id,capability_code,product_code,policy_version,idempotency_key,request_fingerprint) VALUES('TAXI','prov-1','asset-1','WHEELCHAIR_ACCESSIBLE','TAXI_ACCESS','3',$1,'fp') RETURNING id::text`, uuid.NewString()).Scan(&asset); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx, `INSERT INTO organization_verification_submissions(organization_id,submission_id,idempotency_key,request_fingerprint,snapshot,document_file_ids) VALUES(gen_random_uuid(),gen_random_uuid(),$1,'fp','{}','[]') RETURNING id::text`, uuid.NewString()).Scan(&org); err != nil {
		t.Fatal(err)
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for kind, ref := range map[string]string{"vendor-cases": caseID, "vendor-case-findings": finding, "asset-capability-cases": asset, "organization-submissions": org} {
		resource, err := adminresources.Source().Read(ctx, tx, kind, ref, true)
		if err != nil || resource.Reference != ref || resource.SnapshotETag == "" {
			t.Fatalf("%s: %+v %v", kind, resource, err)
		}
		if _, err := adminresources.Source().Binding(ctx, tx, kind, ref); err != nil {
			t.Fatalf("binding %s: %v", kind, err)
		}
		for _, dim := range resource.Dimensions {
			if dim.Value != nil && *dim.Value == "doc-secret-ref" {
				t.Fatalf("%s discloses a document reference", kind)
			}
		}
		for _, r := range resource.References {
			if r.Reference == "doc-secret-ref" {
				t.Fatalf("%s discloses a document reference", kind)
			}
		}
		if kind == "vendor-cases" {
			counts := map[string]int64{}
			for _, c := range resource.Counters {
				if c.Value != nil {
					counts[c.Name] = *c.Value
				}
			}
			if counts["document_count"] != 1 || counts["finding_count"] != 1 || counts["appeal_count"] != 0 || len(resource.Timeline) != 1 {
				t.Fatalf("vendor case projection=%+v", resource)
			}
		}
		if kind == "asset-capability-cases" {
			if len(resource.Counters) != 1 || resource.Counters[0].Value == nil || *resource.Counters[0].Value != 3 {
				t.Fatalf("policy version counter=%+v", resource.Counters)
			}
		}
	}
	if err := adminresources.Source().AuthorizeStaff(ctx, tx, "nobody", "vendor-cases", caseID); err == nil {
		t.Fatal("read without a native grant authorized")
	}
}
