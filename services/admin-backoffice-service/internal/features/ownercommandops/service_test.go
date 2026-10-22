package ownercommandops

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminpolicy"
	fixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/google/uuid"
)

// fakeOwner mimics the owner contract's observable semantics: a version fence,
// an immutable receipt per idempotency key and a refusal that commits nothing.
type fakeOwner struct {
	mu            sync.Mutex
	version       int64
	receipts      map[string]admincommand.Receipt
	posts, gets   atomic.Int32
	postErr       error
	commitThenErr bool
	tamper        func(*admincommand.Receipt)
}

func (f *fakeOwner) Spec() admincommand.Spec {
	s, _ := admincommand.ForOwner("driver-taxi-service")
	return s
}
func (f *fakeOwner) Execute(_ context.Context, q adminread.Query, c admincommand.Command) (admincommand.Receipt, error) {
	f.posts.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.postErr != nil && !f.commitThenErr {
		return admincommand.Receipt{}, f.postErr
	}
	if c.ExpectedVersion != f.version {
		return admincommand.Receipt{}, admincommand.ErrRefused
	}
	approval := ""
	if c.Approval != nil {
		approval = c.Approval.ApprovalID
	}
	r := admincommand.Receipt{ContractVersion: c.ContractVersion, Owner: "driver-taxi-service", OperationID: c.OperationID, DecisionID: uuid.NewString(), SubjectKind: c.SubjectKind, SubjectID: c.SubjectID, CaseRef: c.CaseRef, Action: c.Action, IdempotencyKey: c.IdempotencyKey, IntentDigest: c.IntentDigest(q.Staff, q.Purpose, admincommand.QueryScope(q)), ApprovalID: approval, PriorState: "ACTIVE", ResultingState: "RESTRICTED", RecordedVersion: c.ExpectedVersion + 1, Outcome: "PROVIDER_RESTRICTED", Scope: admincommand.QueryScope(q), RecordedAt: time.Now().UTC().Truncate(time.Microsecond)}
	f.version++
	f.receipts[c.IdempotencyKey] = r
	if f.tamper != nil {
		f.tamper(&r)
	}
	if f.commitThenErr {
		return admincommand.Receipt{}, f.postErr
	}
	return r, nil
}
func (f *fakeOwner) Readback(_ context.Context, _ adminread.Query, _, _, key string) (admincommand.Receipt, error) {
	f.gets.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.receipts[key]
	if !ok {
		return admincommand.Receipt{}, admincommand.ErrAbsent
	}
	return r, nil
}
func (f *fakeOwner) History(_ context.Context, q adminread.Query, kind, id string) (admincommand.History, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return admincommand.History{ContractVersion: "taxi.admin-command.v1", Owner: "driver-taxi-service", SubjectKind: kind, SubjectID: id, CurrentVersion: f.version, CurrentState: "ACTIVE", Decisions: []admincommand.Receipt{}, Scope: admincommand.QueryScope(q), AsOf: time.Now().UTC()}, nil
}

type harness struct {
	s            *Service
	owner        *fakeOwner
	alice, bob   adminpolicy.Actor
	qAlice, qBob adminread.Query
	subject      string
}

func setup(t *testing.T) harness {
	db := testutil.Database(t)
	spec, _ := admincommand.ForOwner("driver-taxi-service")
	scope := adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	p := fixture.New(t, scope, spec.Owner, spec.Capability, spec.Contract)
	for _, cap := range []string{spec.ReadCapability, spec.ApproveCapability} {
		rule := p.Document.Rules[0]
		rule.Capability, rule.Permission = cap, cap
		p.Document.Rules = append(p.Document.Rules, rule)
	}
	p.Sign(t)
	owner := &fakeOwner{receipts: map[string]admincommand.Receipt{}}
	actor := func(staff string) adminpolicy.Actor {
		return adminpolicy.Actor{Product: "karryam", Staff: staff, Roles: []string{"staff"}, Permissions: []string{spec.Capability, spec.ReadCapability, spec.ApproveCapability}, Markets: []string{"NG"}, Zones: []string{"Z1"}, Organizations: []string{"org-1"}, Tenants: []string{"tenant-1"}}
	}
	query := func(staff string) adminread.Query {
		return adminread.Query{Scope: adminread.Scope{Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}, Staff: staff, Role: "staff", Purpose: "case-1", PolicyID: p.Engine.ID(), PolicyVersion: p.Engine.Version(), PolicyChecksum: p.Engine.Checksum(), Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}
	}
	return harness{s: &Service{DB: db, Owner: owner, Policy: p.Engine, WriteEnabled: true, ReadEnabled: true}, owner: owner, alice: actor("staff-a"), bob: actor("staff-b"), qAlice: query("staff-a"), qBob: query("staff-b"), subject: uuid.NewString()}
}
func (h harness) submission(action string, expected int64, approval string) admincommand.Submission {
	s := admincommand.Submission{ContractVersion: "taxi.admin-command.v1", OperationID: uuid.NewString(), SubjectKind: "providers", SubjectID: h.subject, Action: action, ExpectedVersion: expected, IdempotencyKey: uuid.NewString(), ReasonCode: "SAFETY_REVIEW", Params: map[string]string{}, ApprovalID: approval, RequestedAt: time.Now().UTC().Add(-time.Second)}
	if action == "RESTRICT_PROVIDER" {
		s.Params["restriction"] = "SUSPEND_ONLINE"
	}
	return s
}
func (h harness) approved(t *testing.T, expected int64) string {
	t.Helper()
	id := uuid.NewString()
	p := admincommand.Proposal{ContractVersion: "taxi.admin-command.v1", ApprovalID: id, SubjectKind: "providers", SubjectID: h.subject, Action: "RESTRICT_PROVIDER", ExpectedVersion: expected, ReasonCode: "SAFETY_REVIEW", Params: map[string]string{"restriction": "SUSPEND_ONLINE"}}
	if _, err := h.s.Propose(context.Background(), h.alice, h.qAlice, p); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.Decide(context.Background(), h.bob, h.qBob, id, admincommand.ApprovalDecision{Decision: "APPROVE", ReasonCode: "VERIFIED_EVIDENCE"}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPostgresOwnerCommandTwoPersonApprovalIsBoundSingleUseAndRequesterOnly(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	if _, err := h.s.Submit(ctx, h.alice, h.qAlice, h.submission("RESTRICT_PROVIDER", 0, "")); !errors.Is(err, admincommand.ErrInvalid) {
		t.Fatal("approval-required command accepted without approval", err)
	}
	if _, err := h.s.Submit(ctx, h.alice, h.qAlice, h.submission("REQUEST_REVERIFICATION", 0, uuid.NewString())); !errors.Is(err, admincommand.ErrInvalid) {
		t.Fatal("approval attached to an action that does not declare one")
	}
	id := uuid.NewString()
	proposal := admincommand.Proposal{ContractVersion: "taxi.admin-command.v1", ApprovalID: id, SubjectKind: "providers", SubjectID: h.subject, Action: "RESTRICT_PROVIDER", ExpectedVersion: 0, ReasonCode: "SAFETY_REVIEW", Params: map[string]string{"restriction": "SUSPEND_ONLINE"}}
	if _, err := h.s.Propose(ctx, h.alice, h.qAlice, proposal); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.Decide(ctx, h.alice, h.qAlice, id, admincommand.ApprovalDecision{Decision: "APPROVE", ReasonCode: "SELF"}); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatal("requester approved own proposal", err)
	}
	if _, err := h.s.Submit(ctx, h.alice, h.qAlice, h.submission("RESTRICT_PROVIDER", 0, id)); !errors.Is(err, ErrApproval) {
		t.Fatal("pending approval executed", err)
	}
	if _, err := h.s.Decide(ctx, h.bob, h.qBob, id, admincommand.ApprovalDecision{Decision: "APPROVE", ReasonCode: "VERIFIED_EVIDENCE"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.Decide(ctx, h.bob, h.qBob, id, admincommand.ApprovalDecision{Decision: "REJECT", ReasonCode: "CHANGED_MIND"}); !errors.Is(err, ErrApproval) {
		t.Fatal("approval decision changed", err)
	}
	if _, err := h.s.Submit(ctx, h.bob, h.qBob, h.submission("RESTRICT_PROVIDER", 0, id)); !errors.Is(err, ErrApproval) {
		t.Fatal("approver executed requester's approval", err)
	}
	other := h.submission("RESTRICT_PROVIDER", 0, id)
	other.Params["restriction"] = "NO_NEW_ASSIGNMENTS"
	if _, err := h.s.Submit(ctx, h.alice, h.qAlice, other); !errors.Is(err, ErrApproval) {
		t.Fatal("approval reused for a different intent", err)
	}
	out, err := h.s.Submit(ctx, h.alice, h.qAlice, h.submission("RESTRICT_PROVIDER", 0, id))
	if err != nil || out.Status != "CONFIRMED" || out.Receipt == nil || out.Receipt.ApprovalID != id || out.ApprovalID != id {
		t.Fatal("approved command not confirmed", err, out)
	}
	if _, err := h.s.Submit(ctx, h.alice, h.qAlice, h.submission("RESTRICT_PROVIDER", 1, id)); !errors.Is(err, ErrApproval) {
		t.Fatal("approval consumed twice", err)
	}
	view, err := h.s.Approval(ctx, h.bob, h.qBob, id)
	if err != nil || view.Status != "CONSUMED" {
		t.Fatal("approval state not visible", err, view)
	}
	var events int
	h.s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_command_approval_audit WHERE approval_id=$1`, id).Scan(&events)
	if events != 3 {
		t.Fatal("approval lifecycle not audited", events)
	}
	for _, sql := range []string{`UPDATE admin_command_approvals SET intent_digest=repeat('0',64)`, `UPDATE admin_command_approvals SET status='APPROVED'`, `DELETE FROM admin_command_approvals`, `TRUNCATE admin_command_approvals CASCADE`, `DELETE FROM admin_command_approval_audit`} {
		if _, err := h.s.DB.Exec(ctx, sql); err == nil {
			t.Fatal("approval evidence mutable", sql)
		}
	}
	expired := uuid.NewString()
	proposal.ApprovalID = expired
	proposal.ExpectedVersion = 1
	h.s.Propose(ctx, h.alice, h.qAlice, proposal)
	h.s.Now = func() time.Time { return time.Now().Add(ApprovalWindow + time.Minute) }
	if _, err := h.s.Decide(ctx, h.bob, h.qBob, expired, admincommand.ApprovalDecision{Decision: "APPROVE", ReasonCode: "LATE"}); err == nil {
		t.Fatal("expired proposal approved")
	}
}

func TestPostgresOwnerCommandUncertainOutcomeReconcilesWithoutResubmit(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	h.owner.postErr, h.owner.commitThenErr = admincommand.ErrUnavailable, true
	in := h.submission("REQUEST_REVERIFICATION", 0, "")
	in.Params["document"] = "DRIVER_LICENSE"
	out, err := h.s.Submit(ctx, h.alice, h.qAlice, in)
	if err != nil || out.Status != "UNKNOWN_OUTCOME" {
		t.Fatal("dropped response not recorded as unknown", err, out)
	}
	if _, err := h.s.Submit(ctx, h.alice, h.qAlice, h.submission("REQUEST_REVERIFICATION", 0, "")); err == nil {
		t.Fatal("new command raced an unresolved slot")
	}
	again, err := h.s.Submit(ctx, h.alice, h.qAlice, in)
	if err != nil || again.Status != "CONFIRMED" || h.owner.posts.Load() != 1 {
		t.Fatal("replay re-posted instead of reconciling", err, again, h.owner.posts.Load())
	}
	recon, err := h.s.Reconcile(ctx, h.alice, h.qAlice, in.OperationID)
	if err != nil || recon.Status != "CONFIRMED" || *recon.Receipt != *again.Receipt {
		t.Fatal("reconcile drifted", err)
	}
	if _, err := h.s.Reconcile(ctx, h.bob, h.qBob, in.OperationID); !errors.Is(err, ErrNotFound) {
		t.Fatal("another staff member read the operation", err)
	}
	if _, err := h.s.DB.Exec(ctx, `UPDATE admin_owner_command_operations SET status='EXECUTING'`); err == nil {
		t.Fatal("settled operation reopened")
	}
}

func TestPostgresOwnerCommandRefusalAndSupersessionReleaseOnlyProvenSlots(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	stale := h.submission("REQUEST_REVERIFICATION", 3, "")
	stale.Params["document"] = "INSURANCE"
	out, err := h.s.Submit(ctx, h.alice, h.qAlice, stale)
	if err != nil || out.Status != "REJECTED" || out.RejectionCode != "owner_refused" {
		t.Fatal("definitive owner refusal not settled", err, out)
	}
	// Uncertain outcome where nothing committed: slot stays held until the
	// owner's own version moves past it, which proves the original can't commit.
	h.owner.postErr, h.owner.commitThenErr = admincommand.ErrUnavailable, false
	lost := h.submission("REQUEST_REVERIFICATION", 0, "")
	lost.Params["document"] = "INSURANCE"
	out, err = h.s.Submit(ctx, h.alice, h.qAlice, lost)
	if err != nil || out.Status != "UNKNOWN_OUTCOME" {
		t.Fatal(err, out)
	}
	if out, _ = h.s.Reconcile(ctx, h.alice, h.qAlice, lost.OperationID); out.Status != "UNKNOWN_OUTCOME" {
		t.Fatal("absence alone treated as rejection", out)
	}
	h.owner.mu.Lock()
	h.owner.version = 1 // another authorized change committed at version 0
	h.owner.mu.Unlock()
	out, err = h.s.Reconcile(ctx, h.alice, h.qAlice, lost.OperationID)
	if err != nil || out.Status != "REJECTED" || out.RejectionCode != "superseded_by_owner_version" {
		t.Fatal("version-fence supersession not proven", err, out)
	}
	h.owner.postErr = nil
	next := h.submission("REQUEST_REVERIFICATION", 1, "")
	next.Params["document"] = "INSURANCE"
	if out, err := h.s.Submit(ctx, h.alice, h.qAlice, next); err != nil || out.Status != "CONFIRMED" {
		t.Fatal(err, out)
	}
}

func TestPostgresOwnerCommandConcurrentIdentitySinglePostAndTamperedProof(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	in := h.submission("REQUEST_REVERIFICATION", 0, "")
	in.Params["document"] = "ROADWORTHINESS"
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); h.s.Submit(ctx, h.alice, h.qAlice, in) }()
	}
	wg.Wait()
	if h.owner.posts.Load() != 1 {
		t.Fatal("concurrent identical submissions posted more than once", h.owner.posts.Load())
	}
	h.owner.tamper = func(r *admincommand.Receipt) { r.Scope.Tenant = "tenant-2" }
	bad := h.submission("REQUEST_REVERIFICATION", 1, "")
	bad.Params["document"] = "ROADWORTHINESS"
	if _, err := h.s.Submit(ctx, h.alice, h.qAlice, bad); !errors.Is(err, ErrProof) {
		t.Fatal("tampered receipt accepted", err)
	}
	var status string
	h.s.DB.QueryRow(ctx, `SELECT status FROM admin_owner_command_operations WHERE id=$1`, bad.OperationID).Scan(&status)
	if status != "UNKNOWN_OUTCOME" {
		t.Fatal("proof failure not fenced as uncertain", status)
	}
	h.s.WriteEnabled = false
	if _, err := h.s.Submit(ctx, h.alice, h.qAlice, h.submission("REQUEST_REVERIFICATION", 2, "")); !errors.Is(err, ErrDisabled) {
		t.Fatal("kill switch ignored")
	}
	if _, err := h.s.Reconcile(ctx, h.alice, h.qAlice, in.OperationID); err != nil {
		t.Fatal("readback must survive the write kill switch", err)
	}
	foreign := h.qAlice
	foreign.Scope.Tenant = "tenant-2"
	if _, err := h.s.Reconcile(ctx, h.alice, foreign, in.OperationID); err == nil {
		t.Fatal("foreign tenant reconcile accepted")
	}
}
