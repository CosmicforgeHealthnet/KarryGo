package integration_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/events"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/admincommands"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/adminresources"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestPostgresWalletAdminResourcesAndNativeFinancialCommands(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `ALTER TABLE wallet_admin_case_history DISABLE TRIGGER wallet_admin_case_history_immutable; TRUNCATE wallet_admin_case_history,wallet_admin_cases; ALTER TABLE wallet_admin_case_history ENABLE TRIGGER wallet_admin_case_history_immutable`); err != nil {
			t.Errorf("clean disposable Admin evidence: %v", err)
		}
	})
	organization := "org-" + uuid.NewString()
	requester, approver := "staff-"+uuid.NewString(), "staff-"+uuid.NewString()
	for _, row := range []struct{ id, staff, role string }{{uuid.NewString(), requester, "frol_finance_admin"}, {uuid.NewString(), approver, "frol_finance_admin"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO finance_role_grants(id,organization_id,subject_type,subject_id,role_id,granted_by) VALUES($1,$2,'USER',$3,$4,'test')`, row.id, organization, row.staff, row.role); err != nil {
			t.Fatal(err)
		}
	}
	ledger := repositories.NewLedgerRepository(pool)
	accounts := repositories.NewAccountRepository(pool, ledger)
	operations := repositories.NewOperationsRepository(pool)
	executor := admincommands.Executor{Accounts: accounts, Ledger: ledger, Operations: operations, Reversals: repositories.NewReversalRepository(pool), ProviderOps: repositories.NewProviderOperationsRepository(pool), Marketplace: repositories.NewMarketplaceRepository(pool), Audit: audit.NewRecorder(), Events: events.NewDomainPublisher(events.NewOutboxRepository(pool))}

	var walletID, journalID string
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		wallet, _, err := accounts.EnsureWallet(ctx, tx, organization, "NGN")
		if err != nil {
			return err
		}
		walletID = wallet.ID
		counterparty, err := ledger.EnsureAccount(ctx, tx, organization, models.OwnerPlatform, organization, models.AccountPlatformReceivable, "NGN", models.SideDebit)
		if err != nil {
			return err
		}
		journal, err := ledger.Post(ctx, tx, repositories.Posting{OrganizationID: organization, JournalType: "PAYMENT", Currency: "NGN", Reference: "payment-" + uuid.NewString(), SourceService: "test", SourceReference: uuid.NewString(), IdempotencyKey: uuid.NewString(), Fingerprint: "test", ActorType: "SYSTEM", ActorID: "test", Lines: []repositories.PostingLine{{LedgerAccountID: counterparty.ID, Side: models.SideDebit, AmountMinor: 5000}, {LedgerAccountID: wallet.LedgerAccountID, Side: models.SideCredit, AmountMinor: 5000}}})
		if err == nil {
			journalID = journal.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	apply := func(staff, action, kind, id, caseRef string, version int64, params map[string]string, approval bool) (admincommand.Transition, error) {
		var out admincommand.Transition
		err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
			spec := admincommands.Spec().Actions[action]
			in := admincommand.Execution{Command: admincommand.Command{ContractVersion: admincommands.Spec().Contract, OperationID: uuid.NewString(), SubjectKind: kind, SubjectID: id, CaseRef: caseRef, Action: action, ExpectedVersion: version, IdempotencyKey: "idem-" + uuid.NewString(), ReasonCode: "ADMIN_REVIEW", Params: params, RequestedAt: time.Now().UTC()}, Spec: spec, DecisionID: uuid.NewString(), Staff: staff, Purpose: "financial-review", Scope: adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: organization, Tenant: organization}, Now: time.Now().UTC()}
			if approval {
				in.Command.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: "hq-approver", IntentDigest: "unused-by-executor"}
			}
			var err error
			out, err = executor.Apply(ctx, tx, in)
			return err
		})
		return out, err
	}

	if out, err := apply(requester, "RESTRICT_ACCOUNT", "organization-wallets", walletID, "", 1, map[string]string{"restriction": "MANUAL_REVIEW"}, true); err != nil || out.ResultingState != models.StatusRestricted {
		t.Fatalf("restrict wallet: %#v %v", out, err)
	}
	resourceTx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	// Every Section 13 projection is prepared by PostgreSQL, so a renamed or
	// fabricated owner column fails this test instead of reaching deployment.
	for _, kind := range []string{
		"customer-accounts", "payment-intents", "customer-refunds", "provider-payouts",
		"provider-webhook-events", "commerce-payment-intents", "commerce-allocations", "commerce-operations",
		"subscriptions", "invoices", "payments", "commission-rules",
		"earning-allocations", "statements", "financial-audit",
	} {
		if _, readErr := adminresources.Source().Read(ctx, resourceTx, kind, uuid.NewString(), false); !errors.Is(readErr, pgx.ErrNoRows) {
			resourceTx.Rollback(ctx)
			t.Fatalf("validate %s projection: expected no rows, got %v", kind, readErr)
		}
	}
	resource, err := adminresources.Source().Read(ctx, resourceTx, "organization-wallets", walletID, true)
	_ = resourceTx.Rollback(ctx)
	if err != nil || resource.Status == nil || *resource.Status != models.StatusRestricted || resource.SourceVersion == nil || *resource.SourceVersion != 2 {
		t.Fatalf("wallet readback: %#v %v", resource, err)
	}

	caseRef := "case-" + uuid.NewString()
	if _, err = apply(requester, "OPEN_FINANCIAL_INVESTIGATION", "journals", journalID, caseRef, 0, map[string]string{"category": "REFUND_REVIEW", "severity": "HIGH"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err = apply(requester, "RECORD_FINANCIAL_FINDING", "journals", journalID, caseRef, 1, map[string]string{"finding": "SUBSTANTIATED", "evidence_ref": "evidence-1"}, false); err != nil {
		t.Fatal(err)
	}
	if out, err := apply(requester, "REQUEST_REFUND", "journals", journalID, caseRef, 2, map[string]string{"amount_minor": strconv.FormatInt(2000, 10)}, false); err != nil || out.Outcome != "REFUND_REQUESTED_FROM_WALLET" {
		t.Fatalf("request refund: %#v %v", out, err)
	}
	var refundID string
	if err = pool.QueryRow(ctx, `SELECT owner_operation_ref FROM wallet_admin_cases WHERE case_ref=$1`, caseRef).Scan(&refundID); err != nil || refundID == "" {
		t.Fatalf("refund link: %q %v", refundID, err)
	}
	if _, err = apply(approver, "APPROVE_REFUND", "refunds", refundID, "", 1, nil, true); err != nil {
		t.Fatalf("approve refund: %v", err)
	}
	refund, err := operations.GetRefund(ctx, nil, organization, refundID)
	if err != nil || refund.Status != models.RefundApproved || refund.RequestedByID != requester || refund.ApprovedByID != approver {
		t.Fatalf("authoritative refund readback: %#v %v", refund, err)
	}
	if _, err = apply(requester, "APPROVE_REFUND", "refunds", refundID, "", 2, nil, true); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("requester self-approval did not fail closed: %v", err)
	}
	reversalCase := "case-" + uuid.NewString()
	if _, err = apply(requester, "OPEN_FINANCIAL_INVESTIGATION", "journals", journalID, reversalCase, 0, map[string]string{"category": "REVERSAL_REVIEW", "severity": "HIGH"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err = apply(requester, "RECORD_FINANCIAL_FINDING", "journals", journalID, reversalCase, 1, map[string]string{"finding": "SUBSTANTIATED", "evidence_ref": "evidence-2"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err = apply(requester, "REQUEST_REVERSAL", "journals", journalID, reversalCase, 2, map[string]string{"amount_minor": "1000"}, false); err != nil {
		t.Fatalf("request reversal: %v", err)
	}
	var reversalID string
	if err = pool.QueryRow(ctx, `SELECT owner_operation_ref FROM wallet_admin_cases WHERE case_ref=$1`, reversalCase).Scan(&reversalID); err != nil || reversalID == "" {
		t.Fatalf("reversal link: %q %v", reversalID, err)
	}
	if _, err = apply(approver, "APPROVE_REVERSAL", "reversals", reversalID, "", 1, nil, true); err != nil {
		t.Fatalf("approve reversal: %v", err)
	}
	reversal, err := repositories.NewReversalRepository(pool).Get(ctx, nil, organization, reversalID)
	if err != nil || reversal.Status != models.ReversalApproved || reversal.CreatedByActorID != requester || reversal.ApprovedByActorID != approver {
		t.Fatalf("authoritative reversal readback: %#v %v", reversal, err)
	}
	var audits, decisions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM finance_audit_records WHERE resource_id IN ($1,$2,$3)`, walletID, refundID, reversalID).Scan(&audits); err != nil || audits < 5 {
		t.Fatalf("native financial audit missing: %d %v", audits, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM wallet_admin_case_history WHERE case_ref=$1`, caseRef).Scan(&decisions); err != nil || decisions != 3 {
		t.Fatalf("case history mismatch: %d %v", decisions, err)
	}
}
