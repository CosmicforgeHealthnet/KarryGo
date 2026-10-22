package persistence

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/ownercommandops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type commandReceiptOwner struct{ receipt admincommand.Receipt }

func (o *commandReceiptOwner) Spec() admincommand.Spec {
	s, _ := admincommand.ForOwner("customer-service")
	return s
}
func (o *commandReceiptOwner) Execute(_ context.Context, q adminread.Query, c admincommand.Command) (admincommand.Receipt, error) {
	approval := ""
	if c.Approval != nil {
		approval = c.Approval.ApprovalID
	}
	o.receipt = admincommand.Receipt{ContractVersion: c.ContractVersion, Owner: "customer-service", OperationID: c.OperationID, DecisionID: uuid.NewString(), SubjectKind: c.SubjectKind, SubjectID: c.SubjectID, CaseRef: c.CaseRef, Action: c.Action, IdempotencyKey: c.IdempotencyKey, IntentDigest: c.IntentDigest(q.Staff, q.Purpose, admincommand.QueryScope(q)), ApprovalID: approval, PriorState: "ACTIVE", ResultingState: "RESTRICTED", RecordedVersion: c.ExpectedVersion + 1, Outcome: "ACCOUNT_RESTRICTED", Scope: admincommand.QueryScope(q), RecordedAt: time.Now().UTC().Truncate(time.Microsecond)}
	return o.receipt, nil
}
func (o *commandReceiptOwner) Readback(context.Context, adminread.Query, string, string, string) (admincommand.Receipt, error) {
	return o.receipt, nil
}
func (o *commandReceiptOwner) History(context.Context, adminread.Query, string, string) (admincommand.History, error) {
	return admincommand.History{}, admincommand.ErrAbsent
}

func TestOwnerCommandRuntimeRoleGrantsAndEvidenceImmutability(t *testing.T) {
	root := testutil.Database(t)
	ctx := context.Background()
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
	spec, _ := admincommand.ForOwner("customer-service")
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	p := policyfixture.New(t, scope, spec.Owner, spec.Capability, spec.Contract)
	for _, cap := range []string{spec.ReadCapability, spec.ApproveCapability} {
		rule := p.Document.Rules[0]
		rule.Capability, rule.Permission = cap, cap
		p.Document.Rules = append(p.Document.Rules, rule)
	}
	p.Sign(t)
	svc := &ownercommandops.Service{DB: runtime, Owner: &commandReceiptOwner{}, Policy: p.Engine, WriteEnabled: true, ReadEnabled: true}
	actor := func(staff string) adminpolicy.Actor {
		return adminpolicy.Actor{Product: "karryam", Staff: staff, Roles: []string{"staff"}, Permissions: []string{spec.Capability, spec.ReadCapability, spec.ApproveCapability}, Markets: []string{"M1"}, Zones: []string{"Z1"}, Organizations: []string{"org-1"}, Tenants: []string{"tenant-1"}}
	}
	query := func(staff string) adminread.Query {
		return adminread.Query{Scope: adminread.Scope{Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}, Staff: staff, Role: "staff", Purpose: "case-1", PolicyID: p.Engine.ID(), PolicyVersion: p.Engine.Version(), PolicyChecksum: p.Engine.Checksum(), Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}
	}
	customer := uuid.NewString()
	approval := uuid.NewString()
	if _, err := svc.Propose(ctx, actor("staff-1"), query("staff-1"), admincommand.Proposal{ContractVersion: spec.Contract, ApprovalID: approval, SubjectKind: "customers", SubjectID: customer, Action: "RESTRICT_ACCOUNT", ReasonCode: "FRAUD_REVIEW", Params: map[string]string{"restriction": "LOGIN_BLOCKED"}}); err != nil {
		t.Fatal("runtime cannot propose", err)
	}
	if _, err := svc.Decide(ctx, actor("staff-2"), query("staff-2"), approval, admincommand.ApprovalDecision{Decision: "APPROVE", ReasonCode: "EVIDENCE_REVIEWED"}); err != nil {
		t.Fatal("runtime cannot decide", err)
	}
	in := admincommand.Submission{ContractVersion: spec.Contract, OperationID: uuid.NewString(), SubjectKind: "customers", SubjectID: customer, Action: "RESTRICT_ACCOUNT", IdempotencyKey: uuid.NewString(), ReasonCode: "FRAUD_REVIEW", Params: map[string]string{"restriction": "LOGIN_BLOCKED"}, ApprovalID: approval, RequestedAt: time.Now().UTC().Add(-time.Second)}
	if out, err := svc.Submit(ctx, actor("staff-1"), query("staff-1"), in); err != nil || out.Status != "CONFIRMED" {
		t.Fatal("least privilege command orchestration failed", out, err)
	}
	if out, err := svc.Reconcile(ctx, actor("staff-1"), query("staff-1"), in.OperationID); err != nil || out.Status != "CONFIRMED" {
		t.Fatal("least privilege readback failed", out, err)
	}
	for _, sql := range []string{`SELECT * FROM admin_owner_command_operation_audit`, `SELECT * FROM admin_command_approval_audit`, `UPDATE admin_owner_command_operations SET intent_digest=repeat('0',64)`, `UPDATE admin_command_approvals SET intent_digest=repeat('0',64)`, `DELETE FROM admin_owner_command_operations`, `DELETE FROM admin_command_approvals`, `TRUNCATE admin_owner_command_operations CASCADE`, `TRUNCATE admin_command_approvals CASCADE`, `DELETE FROM admin_owner_command_operation_audit`, `DELETE FROM admin_command_approval_audit`, `UPDATE admin_command_approvals SET status='PENDING'`} {
		if _, err := runtime.Exec(ctx, sql); err == nil {
			t.Fatal("command runtime evidence privilege escaped", sql)
		}
	}
}
