package customerworkitemops

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/adminpolicy"
	fixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/customerreview"
	"github.com/google/uuid"
)

type fakeOwner struct {
	posts, gets          atomic.Int32
	mu                   sync.Mutex
	receipt              customerreview.WorkItemReceipt
	postError, readError error
	tamper               func(*customerreview.WorkItemReceipt)
	afterPost            func()
}

func (f *fakeOwner) TransitionWorkItem(_ context.Context, q adminread.Query, input customerreview.WorkItemCommand) (customerreview.WorkItemReceipt, error) {
	f.posts.Add(1)
	f.mu.Lock()
	f.receipt = customerreview.WorkItemReceipt{ContractVersion: customerreview.WorkItemContract, State: "ACKNOWLEDGED", Outcome: "REVIEW_ACKNOWLEDGED", ReviewRequestID: input.ReviewRequestID, OperationID: input.OperationID, CustomerID: input.CustomerID, IdempotencyKey: input.IdempotencyKey, IntentDigest: input.Digest(q.Staff, q.Purpose, scope(q)), RecordedVersion: input.ExpectedVersion + 1, Scope: scope(q), RecordedAt: time.Now().UTC()}
	out := f.receipt
	f.mu.Unlock()
	if f.afterPost != nil {
		f.afterPost()
	}
	return out, f.postError
}
func (f *fakeOwner) WorkItemReadback(_ context.Context, _ adminread.Query, _, _ string) (customerreview.WorkItemReceipt, error) {
	f.gets.Add(1)
	f.mu.Lock()
	out := f.receipt
	f.mu.Unlock()
	if f.readError != nil {
		return customerreview.WorkItemReceipt{}, f.readError
	}
	if out.OperationID == "" {
		return customerreview.WorkItemReceipt{}, customerreview.ErrAbsent
	}
	if f.tamper != nil {
		f.tamper(&out)
	}
	return out, nil
}

type testFixture struct {
	s     *Service
	actor adminpolicy.Actor
	q     adminread.Query
	input customerreview.WorkItemCommand
	owner *fakeOwner
}

func setup(t *testing.T) testFixture {
	t.Helper()
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	p := fixture.New(t, scope, customerreview.Owner, customerreview.WorkItemCapability, customerreview.WorkItemContract)
	r := p.Document.Rules[0]
	r.Capability = customerreview.WorkItemReadCapability
	r.Permission = customerreview.WorkItemReadCapability
	r.SourceContract = customerreview.WorkItemContract
	p.Document.Rules = append(p.Document.Rules, r)
	p.Sign(t)
	owner := &fakeOwner{}
	s := &Service{DB: testutil.Database(t), Owner: owner, Policy: p.Engine, WriteEnabled: true, ReadEnabled: true}
	return testFixture{s: s, actor: adminpolicy.Actor{Product: "karryam", Staff: "staff-1", Roles: []string{"staff"}, Permissions: []string{customerreview.WorkItemCapability, customerreview.WorkItemReadCapability}, Markets: []string{"M1"}, Zones: []string{"Z1"}, Organizations: []string{"org-1"}, Tenants: []string{"tenant-1"}}, q: adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: p.Engine.ID(), PolicyVersion: p.Engine.Version(), PolicyChecksum: p.Engine.Checksum(), Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}, input: customerreview.WorkItemCommand{ReviewRequestID: uuid.NewString(), Action: "ACKNOWLEDGE", ContractVersion: customerreview.WorkItemContract, OperationID: uuid.NewString(), CustomerID: uuid.NewString(), ExpectedVersion: 1, IdempotencyKey: uuid.NewString(), ReasonCode: "ACCOUNT_REVIEW", RequestedAt: time.Now().UTC().Add(-time.Second)}, owner: owner}
}
func TestPostgresCustomerWorkItemBridgeConcurrentSubmissionPostsOnce(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := f.s.Submit(ctx, f.actor, f.q, f.input)
			if err != nil && !errors.Is(err, customerreview.ErrAbsent) {
				t.Error(err)
			}
			if err == nil && out.Status != "CONFIRMED" {
				t.Error(out)
			}
		}()
	}
	wg.Wait()
	if f.owner.posts.Load() != 1 {
		t.Fatal("replayed POST", f.owner.posts.Load())
	}
	out, err := f.s.Reconcile(ctx, f.actor, f.q, f.input.OperationID)
	if err != nil || out.Receipt == nil || out.Status != "CONFIRMED" {
		t.Fatal(out, err)
	}
	changed := f.input
	changed.ReasonCode = "CHANGED"
	if _, err := f.s.Submit(ctx, f.actor, f.q, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("changed intent accepted", err)
	}
	changed = f.input
	changed.OperationID = uuid.NewString()
	changed.IdempotencyKey = uuid.NewString()
	if _, err := f.s.Submit(ctx, f.actor, f.q, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("fresh key retried same aggregate revision", err)
	}
	if f.owner.posts.Load() != 1 {
		t.Fatal("fresh key posted")
	}
}
func TestPostgresCustomerWorkItemBridgeUnknownOutcomeOnlyReadsOriginal(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.owner.postError = context.DeadlineExceeded
	out, err := f.s.Submit(ctx, f.actor, f.q, f.input)
	if err != nil || out.Status != "UNKNOWN_OUTCOME" || out.Receipt != nil {
		t.Fatal(out, err)
	}
	f.owner.readError = customerreview.ErrAbsent
	if _, err := f.s.Submit(ctx, f.actor, f.q, f.input); !errors.Is(err, customerreview.ErrAbsent) {
		t.Fatal("absence treated as proof", err)
	}
	var status string
	if err := f.s.DB.QueryRow(ctx, `SELECT status FROM admin_customer_work_item_operations`).Scan(&status); err != nil || status != "UNKNOWN_OUTCOME" {
		t.Fatal(status, err)
	}
	f.owner.readError = nil
	f.s.WriteEnabled = false
	// Recreate the process with writes off. Only original-identity GET can finish.
	restarted := &Service{DB: f.s.DB, Owner: f.owner, Policy: f.s.Policy, ReadEnabled: true}
	out, err = restarted.Reconcile(ctx, f.actor, f.q, f.input.OperationID)
	if err != nil || out.Status != "CONFIRMED" {
		t.Fatal(out, err)
	}
	if f.owner.posts.Load() != 1 || f.owner.gets.Load() != 2 {
		t.Fatal("recovery retried or changed identity")
	}
	// A committed receipt grants no enduring source access.
	f.owner.readError = customerreview.ErrRefused
	if _, err := restarted.Reconcile(ctx, f.actor, f.q, f.input.OperationID); !errors.Is(err, customerreview.ErrRefused) {
		t.Fatal("cached outcome bypassed current source authorization", err)
	}
}
func TestPostgresCustomerWorkItemBridgeCrashFenceAndMismatchedProof(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if err := f.s.authorize(ctx, f.actor, f.q, customerreview.WorkItemCapability); err != nil {
		t.Fatal(err)
	}
	r, won, err := f.s.begin(ctx, f.actor, f.q, f.input)
	if err != nil || !won {
		t.Fatal(err)
	}
	// Crash after durable claim, before POST: a new process may not replay it.
	if _, err := f.s.Submit(ctx, f.actor, f.q, f.input); !errors.Is(err, customerreview.ErrAbsent) {
		t.Fatal(err)
	}
	if f.owner.posts.Load() != 0 {
		t.Fatal("crashed submission retried")
	}
	f.owner.TransitionWorkItem(ctx, f.q, f.input) // Simulated original in-flight source completion.
	bad := []func(*customerreview.WorkItemReceipt){func(r *customerreview.WorkItemReceipt) { r.OperationID = uuid.NewString() }, func(r *customerreview.WorkItemReceipt) { r.CustomerID = uuid.NewString() }, func(r *customerreview.WorkItemReceipt) { r.IdempotencyKey = "different" }, func(r *customerreview.WorkItemReceipt) { r.IntentDigest = "wrong" }, func(r *customerreview.WorkItemReceipt) { r.RecordedVersion++ }, func(r *customerreview.WorkItemReceipt) { r.Scope.Tenant = "different" }, func(r *customerreview.WorkItemReceipt) { r.Scope.Zone = "different" }, func(r *customerreview.WorkItemReceipt) { r.State = "ACCOUNT_SUSPENDED" }}
	for _, tamper := range bad {
		f.owner.tamper = tamper
		if _, err := f.s.Reconcile(ctx, f.actor, f.q, r.OperationID); !errors.Is(err, ErrProof) {
			t.Fatal("mismatched original proof accepted", err)
		}
	}
	f.owner.tamper = nil
	if _, err := f.s.Reconcile(ctx, f.actor, f.q, r.OperationID); err != nil {
		t.Fatal(err)
	}
	f.owner.tamper = func(r *customerreview.WorkItemReceipt) { r.ReviewRequestID = uuid.NewString() }
	if _, err := f.s.Reconcile(ctx, f.actor, f.q, r.OperationID); !errors.Is(err, ErrProof) {
		t.Fatal("confirmed receipt replaced", err)
	}
}
func TestPostgresCustomerWorkItemBridgeScopeAndAuditFailureFence(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.s.DB.Exec(ctx, `CREATE FUNCTION fail_review_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$; CREATE TRIGGER fail_review_audit BEFORE INSERT ON admin_customer_work_item_operation_audit FOR EACH ROW EXECUTE FUNCTION fail_review_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Submit(ctx, f.actor, f.q, f.input); err == nil {
		t.Fatal("submission escaped audit failure")
	}
	if f.owner.posts.Load() != 0 {
		t.Fatal("POST before durable audit")
	}
	var count int
	f.s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_customer_work_item_operations`).Scan(&count)
	if count != 0 {
		t.Fatal("partial claim leaked")
	}
	if _, err := f.s.DB.Exec(ctx, `DROP TRIGGER fail_review_audit ON admin_customer_work_item_operation_audit`); err != nil {
		t.Fatal(err)
	}
	f.owner.afterPost = func() {
		if _, err := f.s.DB.Exec(ctx, `CREATE TRIGGER fail_review_audit BEFORE INSERT ON admin_customer_work_item_operation_audit FOR EACH ROW EXECUTE FUNCTION fail_review_audit()`); err != nil {
			t.Error(err)
		}
	}
	if _, err := f.s.Submit(ctx, f.actor, f.q, f.input); err == nil {
		t.Fatal("confirmation escaped failed evidence")
	}
	var status string
	f.s.DB.QueryRow(ctx, `SELECT status FROM admin_customer_work_item_operations`).Scan(&status)
	if status != "EXECUTING" {
		t.Fatal("failed confirmation committed", status)
	}
	if _, err := f.s.DB.Exec(ctx, `DROP TRIGGER fail_review_audit ON admin_customer_work_item_operation_audit`); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"staff", "purpose", "market", "zone", "organization", "tenant"} {
		q := f.q
		actor := f.actor
		switch field {
		case "staff":
			q.Staff = "staff-2"
			actor.Staff = "staff-2"
		case "purpose":
			q.Purpose = "CASE-123"
		case "market":
			q.Scope.Market = "M2"
		case "zone":
			q.Scope.Zone = "Z2"
		case "organization":
			q.Scope.Organization = "org-2"
		case "tenant":
			q.Scope.Tenant = "tenant-2"
		}
		if _, err := f.s.Reconcile(ctx, actor, q, f.input.OperationID); err == nil {
			t.Fatal("cross-boundary operation disclosed", field)
		}
	}
	if _, err := f.s.Reconcile(ctx, f.actor, f.q, f.input.OperationID); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{`UPDATE admin_customer_work_item_operations SET tenant_ref='forged'`, `UPDATE admin_customer_work_item_operations SET status='EXECUTING',receipt=NULL`, `DELETE FROM admin_customer_work_item_operations`, `TRUNCATE admin_customer_work_item_operations CASCADE`, `UPDATE admin_customer_work_item_operation_audit SET outcome='UNKNOWN_OUTCOME'`, `DELETE FROM admin_customer_work_item_operation_audit`, `TRUNCATE admin_customer_work_item_operation_audit`} {
		if _, err := f.s.DB.Exec(ctx, sql); err == nil {
			t.Fatal("durable evidence mutable", sql)
		}
	}
	if f.owner.posts.Load() != 1 {
		t.Fatal("confirmation failure retried POST")
	}
}
func TestCustomerReviewBridgeMissingConfigurationDenies(t *testing.T) {
	s := &Service{}
	if _, err := s.Submit(context.Background(), adminpolicy.Actor{}, adminread.Query{}, customerreview.WorkItemCommand{}); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if _, err := s.Reconcile(context.Background(), adminpolicy.Actor{}, adminread.Query{}, uuid.NewString()); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	s.WriteEnabled = true
	s.ReadEnabled = true
	if err := s.authorize(context.Background(), adminpolicy.Actor{}, adminread.Query{}, customerreview.WorkItemCapability); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatal(err)
	}
}
