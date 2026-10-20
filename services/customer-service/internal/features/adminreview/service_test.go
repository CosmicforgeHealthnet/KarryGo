package adminreview

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/customer-service/internal/features/adminresources"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/customerreview"
	"github.com/google/uuid"
)

type reviewFixture struct {
	service  *Service
	actor    adminpolicy.Actor
	scope    adminpolicy.Scope
	input    customerreview.Request
	issuer   *contextfixture.Fixture
	approval admincontext.Approval
}

func setupReview(t *testing.T) reviewFixture {
	t.Helper()
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	customer := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO customers(id,phone) VALUES($1,'PRIVATE_PHONE')`, customer); err != nil {
		t.Fatal(err)
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	policy := policyfixture.New(t, scope, customerreview.Owner, customerreview.RequestCapability, customerreview.Contract)
	read := policy.Document.Rules[0]
	read.Capability = customerreview.ReadCapability
	read.Permission = customerreview.ReadCapability
	read.SourceContract = customerreview.StateContract
	policy.Document.Rules = append(policy.Document.Rules, read)
	policy.Sign(t)
	issuer := contextfixture.New(t, customerreview.Owner)
	store := &admincontext.Store{DB: db, Reader: adminresources.Source(), Authority: issuer.Authority}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := store.Reader.Binding(ctx, tx, "review-states", customer)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := issuer.Approval("review-owner-allocation", "review-states", customer, customerreview.RequestCapability, binding, scope)
	approval.Capabilities = append(approval.Capabilities, customerreview.ReadCapability)
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	return reviewFixture{service: &Service{DB: db, Policy: policy.Engine, Contexts: store, WriteEnabled: true, ReadEnabled: true}, actor: adminpolicy.Actor{Product: "karryam", Staff: "staff-1", Roles: []string{"staff"}, Permissions: []string{customerreview.RequestCapability, customerreview.ReadCapability}, Markets: []string{"M1"}, Zones: []string{"Z1"}, Organizations: []string{"org-1"}, Tenants: []string{"tenant-1"}}, scope: scope, input: customerreview.Request{ContractVersion: customerreview.Contract, RequestID: uuid.NewString(), CustomerID: customer, ExpectedReviewRevision: 1, IdempotencyKey: uuid.NewString(), ReasonCode: "ACCOUNT_REVIEW", RequestedAt: time.Now().UTC().Add(-time.Second)}, issuer: issuer, approval: approval}
}

func TestPostgresCustomerReviewRecordsOwnerRequestWithImmutableReadback(t *testing.T) {
	f := setupReview(t)
	ctx := context.Background()
	out, err := f.service.Request(ctx, f.actor, "case-1", f.scope, f.input)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != customerreview.Recorded || out.RecordedReviewRevision != 2 || out.Scope != f.scope || out.CustomerID != f.input.CustomerID {
		t.Fatal(out)
	}
	again, err := f.service.Request(ctx, f.actor, "case-1", f.scope, f.input)
	if err != nil || again != out {
		t.Fatal("original identity did not return immutable outcome", again, err)
	}
	read, err := f.service.Receipt(ctx, f.actor, "case-1", f.input.CustomerID, f.input.IdempotencyKey, f.scope)
	if err != nil || read != out {
		t.Fatal(read, err)
	}
	var version, count, audits int
	var accountStatus, reviewStatus string
	if err := f.service.DB.QueryRow(ctx, `SELECT c.status,s.version,s.status FROM customers c JOIN customer_admin_review_state s ON s.customer_id=c.id WHERE c.id=$1`, f.input.CustomerID).Scan(&accountStatus, &version, &reviewStatus); err != nil {
		t.Fatal(err)
	}
	if accountStatus != "active" || version != 2 || reviewStatus != "PENDING_REVIEW" {
		t.Fatal("request changed account standing or lost aggregate guard", accountStatus, version, reviewStatus)
	}
	f.service.DB.QueryRow(ctx, `SELECT count(*) FROM customer_admin_review_requests`).Scan(&count)
	f.service.DB.QueryRow(ctx, `SELECT count(*) FROM customer_admin_review_audit`).Scan(&audits)
	if count != 1 || audits != 1 {
		t.Fatal("duplicate source effect", count, audits)
	}
	for _, sql := range []string{`UPDATE customer_admin_review_requests SET intent_digest=repeat('0',64)`, `DELETE FROM customer_admin_review_requests`, `TRUNCATE customer_admin_review_requests CASCADE`, `DELETE FROM customer_admin_review_audit`} {
		if _, err := f.service.DB.Exec(ctx, sql); err == nil {
			t.Fatal("source evidence could be rewritten", sql)
		}
	}
	f.service.WriteEnabled = false
	if _, err := f.service.Request(ctx, f.actor, "case-1", f.scope, f.input); !errors.Is(err, ErrDisabled) {
		t.Fatal("write kill switch failed", err)
	}
	if got, err := f.service.Receipt(ctx, f.actor, "case-1", f.input.CustomerID, f.input.IdempotencyKey, f.scope); err != nil || got != out {
		t.Fatal("write kill switch disabled authoritative readback", got, err)
	}
}

func TestPostgresCustomerReviewConcurrentIdentityAndFreshKeysCannotDuplicate(t *testing.T) {
	f := setupReview(t)
	ctx := context.Background()
	const workers = 12
	var wg sync.WaitGroup
	results := make(chan customerreview.Receipt, workers)
	failures := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := f.service.Request(ctx, f.actor, "case-1", f.scope, f.input)
			if err != nil {
				failures <- err
				return
			}
			results <- out
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	id := ""
	count := 0
	for out := range results {
		if id != "" && out.ReviewRequestID != id {
			t.Fatal("same identity produced multiple owner requests")
		}
		id = out.ReviewRequestID
		count++
	}
	if count != workers {
		t.Fatal("missing worker result", count)
	}
	for _, expected := range []int64{1, 2} {
		changed := f.input
		changed.RequestID = uuid.NewString()
		changed.IdempotencyKey = uuid.NewString()
		changed.ExpectedReviewRevision = expected
		if _, err := f.service.Request(ctx, f.actor, "case-1", f.scope, changed); !errors.Is(err, ErrConflict) {
			t.Fatalf("fresh key or pending case bypassed native guard: %v", err)
		}
	}
	changed := f.input
	changed.ReasonCode = "OTHER_REVIEW"
	if _, err := f.service.Request(ctx, f.actor, "case-1", f.scope, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("same key changed original intent", err)
	}
}

func TestPostgresCustomerReviewAuditFailureRollsBackAndCurrentAccessIsRechecked(t *testing.T) {
	f := setupReview(t)
	ctx := context.Background()
	if _, err := f.service.DB.Exec(ctx, `CREATE FUNCTION deny_review_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test audit unavailable'; END $$; CREATE TRIGGER deny_review_audit BEFORE INSERT ON customer_admin_review_audit FOR EACH ROW EXECUTE FUNCTION deny_review_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Request(ctx, f.actor, "case-1", f.scope, f.input); err == nil {
		t.Fatal("audit failure returned success")
	}
	var count, version int
	f.service.DB.QueryRow(ctx, `SELECT count(*) FROM customer_admin_review_requests`).Scan(&count)
	f.service.DB.QueryRow(ctx, `SELECT version FROM customer_admin_review_state WHERE customer_id=$1`, f.input.CustomerID).Scan(&version)
	if count != 0 || version != 1 {
		t.Fatal("failed audit committed domain work", count, version)
	}
	if _, err := f.service.DB.Exec(ctx, `DROP TRIGGER deny_review_audit ON customer_admin_review_audit`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Request(ctx, f.actor, "case-1", f.scope, f.input); err != nil {
		t.Fatal(err)
	}
	revoked := f.approval
	revoked.Revision = 2
	revoked.State = "REVOKED"
	if err := f.service.Contexts.Import(ctx, f.issuer.Sign(t, revoked), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Receipt(ctx, f.actor, "case-1", f.input.CustomerID, f.input.IdempotencyKey, f.scope); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatal("known key bypassed current source authorization", err)
	}
}

func TestCustomerReviewDefaultDisabledWithoutPolicyOrStorage(t *testing.T) {
	service := &Service{}
	if _, err := service.Request(context.Background(), adminpolicy.Actor{}, "", adminpolicy.Scope{}, customerreview.Request{}); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if _, err := service.Receipt(context.Background(), adminpolicy.Actor{}, "", "", "", adminpolicy.Scope{}); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
}
