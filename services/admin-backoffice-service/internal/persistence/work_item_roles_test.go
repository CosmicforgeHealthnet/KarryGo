package persistence

import (
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/customerworkitemops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/customerreview"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
	"time"
)

type workItemReceiptOwner struct {
	receipt customerreview.WorkItemReceipt
}

func (o *workItemReceiptOwner) TransitionWorkItem(_ context.Context, q adminread.Query, input customerreview.WorkItemCommand) (customerreview.WorkItemReceipt, error) {
	scope := adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
	o.receipt = customerreview.WorkItemReceipt{ContractVersion: customerreview.WorkItemContract, State: "ACKNOWLEDGED", Outcome: "REVIEW_ACKNOWLEDGED", ReviewRequestID: input.ReviewRequestID, OperationID: input.OperationID, CustomerID: input.CustomerID, IdempotencyKey: input.IdempotencyKey, IntentDigest: input.Digest(q.Staff, q.Purpose, scope), RecordedVersion: input.ExpectedVersion + 1, Scope: scope, RecordedAt: time.Now().UTC()}
	return o.receipt, nil
}
func (o *workItemReceiptOwner) WorkItemReadback(context.Context, adminread.Query, string, string) (customerreview.WorkItemReceipt, error) {
	return o.receipt, nil
}

func TestWorkItemRuntimeRoleGuardAndGrants(t *testing.T) {
	root := testutil.Database(t)
	ctx := context.Background()
	owner := &Store{Pool: root}
	if owner.ValidateRuntime(ctx) == nil {
		t.Fatal("migration owner accepted as runtime")
	}
	role := "admin_runtime_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := root.Exec(ctx, `CREATE ROLE `+role+` NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(ctx, `DROP OWNED BY `+role); _, _ = root.Exec(ctx, `DROP ROLE `+role) })
	var schema string
	if err := root.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	grantBytes, err := os.ReadFile("../../../../deploy/platform/bin/admin-db-grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	grants := strings.ReplaceAll(string(grantBytes), `:"runtime_role"`, pgx.Identifier{role}.Sanitize())
	grants = strings.ReplaceAll(grants, "SCHEMA public", "SCHEMA "+pgx.Identifier{schema}.Sanitize())
	if _, err = root.Exec(ctx, grants); err != nil {
		t.Fatal(err)
	}
	cfg := root.Config()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error { _, err := c.Exec(ctx, `SET ROLE `+role); return err }
	runtime, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	s := &Store{Pool: runtime}
	if err = s.ValidateRuntime(ctx); err != nil {
		t.Fatal("least privilege role refused", err)
	}
	if err = s.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Audit(ctx, Audit{Correlation: uuid.NewString(), Outcome: "TEST"}); err != nil {
		t.Fatal("runtime cannot append audit", err)
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	p := policyfixture.New(t, scope, customerreview.Owner, customerreview.WorkItemCapability, customerreview.WorkItemContract)
	read := p.Document.Rules[0]
	read.Capability = customerreview.WorkItemReadCapability
	read.Permission = customerreview.WorkItemReadCapability
	read.SourceContract = customerreview.WorkItemContract
	p.Document.Rules = append(p.Document.Rules, read)
	p.Sign(t)
	workflow := &customerworkitemops.Service{DB: runtime, Owner: &workItemReceiptOwner{}, Policy: p.Engine, WriteEnabled: true, ReadEnabled: true}
	actor := adminpolicy.Actor{Product: "karryam", Staff: "staff-1", Roles: []string{"staff"}, Permissions: []string{customerreview.WorkItemCapability, customerreview.WorkItemReadCapability}, Markets: []string{"M1"}, Zones: []string{"Z1"}, Organizations: []string{"org-1"}, Tenants: []string{"tenant-1"}}
	query := adminread.Query{Scope: adminread.Scope{Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: p.Engine.ID(), PolicyVersion: p.Engine.Version(), PolicyChecksum: p.Engine.Checksum(), Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}
	input := customerreview.WorkItemCommand{ReviewRequestID: uuid.NewString(), Action: "ACKNOWLEDGE", ContractVersion: customerreview.WorkItemContract, OperationID: uuid.NewString(), CustomerID: uuid.NewString(), ExpectedVersion: 1, IdempotencyKey: uuid.NewString(), ReasonCode: "REVIEW", RequestedAt: time.Now().UTC().Add(-time.Second)}
	if out, err := workflow.Submit(ctx, actor, query, input); err != nil || out.Status != "CONFIRMED" {
		t.Fatal("least privilege review orchestration failed", out, err)
	}
	workflow.WriteEnabled = false
	if out, err := workflow.Reconcile(ctx, actor, query, input.OperationID); err != nil || out.Status != "CONFIRMED" {
		t.Fatal("least privilege original readback failed", out, err)
	}
	for _, sql := range []string{`SELECT * FROM admin_customer_work_item_operation_audit`, `UPDATE admin_customer_work_item_operations SET intent_digest=repeat('0',64)`, `DELETE FROM admin_customer_work_item_operations`, `TRUNCATE admin_customer_work_item_operations CASCADE`, `UPDATE admin_customer_work_item_operation_audit SET outcome='EXECUTING'`, `DELETE FROM admin_customer_work_item_operation_audit`, `TRUNCATE admin_customer_work_item_operation_audit`} {
		if _, err := runtime.Exec(ctx, sql); err == nil {
			t.Fatal("review runtime evidence privilege escaped", sql)
		}
	}
	for _, sql := range []string{`UPDATE admin_audit_entries SET outcome='forged'`, `DELETE FROM admin_audit_entries`, `TRUNCATE admin_audit_entries`, `ALTER TABLE admin_audit_entries DISABLE TRIGGER ALL`, `INSERT INTO admin_revocations(kind,reference) VALUES('staff','forged')`, `SELECT * FROM admin_audit_entries`, `CREATE TABLE forged_domain_state(id int)`} {
		if _, err = runtime.Exec(ctx, sql); err == nil {
			t.Fatalf("runtime privilege escaped: %s", sql)
		}
	}
}
