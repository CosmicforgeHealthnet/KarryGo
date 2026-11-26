package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	financeopsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	financeopsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/repositories"
	financeopsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/apperrors"
)

func appCode(err error) string {
	var ae *apperrors.Error
	if errors.As(err, &ae) {
		return string(ae.Code)
	}
	return ""
}

// TestSection52FinanceAnalysis covers the analytical finance additions:
// invoice submission (58.9), receivables ageing, append-only cost allocation
// that sums exactly to its source, profitability that separates revenue,
// direct and allocated cost without double counting or mixing currencies,
// immutable snapshots, and the payroll calculation fingerprint that blocks
// approval when inputs change after calculation (FIN-001, FIN-003).
func TestSection52FinanceAnalysis(t *testing.T) {
	pool := testPool(t)
	scope, branch, customer := section2223Scope(t, pool)
	ctx := context.Background()
	svc := financeopsusecases.NewService(financeopsusecases.Options{Store: financeopsrepos.NewRepository(pool), Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Media: section2223Media{}, Wallet: &sectionPosting{}, Flags: financeopsmodels.Flags{Invoices: true, Receivables: true, Expenses: true, Budgets: true, Compensation: true, Payroll: true, Settlements: true, Claims: true, Posting: true}})
	approver := scope
	approver.ActorID = ids.New("usr")
	approver.OrganizationUserID = approver.ActorID

	// --- Invoice submit -> approve -> issue ------------------------------------
	invoice, err := svc.CreateInvoice(ctx, scope, financeopsmodels.InvoiceInput{BranchID: branch, CustomerID: customer, Currency: "NGN", DueDate: time.Now().UTC().AddDate(0, 0, -45), Lines: []financeopsmodels.InvoiceLineInput{{SourceType: "SHIPMENT", SourceReference: "s-1", Description: "Haulage", Quantity: 2, UnitAmountMinor: 5000}}})
	if err != nil {
		t.Fatal(err)
	}
	id := invoice["id"].(string)
	if invoice, err = svc.SubmitInvoice(ctx, scope, id, v64(invoice, "version")); err != nil || invoice["status"] != "PENDING_APPROVAL" {
		t.Fatalf("submit = %v %v", invoice["status"], err)
	}
	if _, err = svc.SubmitInvoice(ctx, scope, id, v64(invoice, "version")); err == nil {
		t.Fatal("pending invoice was submitted twice")
	}
	if invoice, err = svc.ApproveInvoice(ctx, approver, id, v64(invoice, "version")); err != nil {
		t.Fatal(err)
	}
	if invoice, err = svc.IssueInvoice(ctx, approver, id, v64(invoice, "version")); err != nil {
		t.Fatal(err)
	}
	var pendingEvents int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND event_type IN('invoice.pending_approval.v1','invoice.issued.v1')`, scope.OrganizationID).Scan(&pendingEvents)
	if pendingEvents != 2 {
		t.Fatalf("invoice lifecycle events = %d, want 2", pendingEvents)
	}
	// Section 26.2: issuing also produces the canonical core event, in the
	// same transaction, with the minimized contract payload.
	var canonical int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND event_type='corporate.invoice.issued.v1' AND aggregate_id=$2 AND payload->'payload'->>'status'='ISSUED'`, scope.OrganizationID, id).Scan(&canonical)
	if canonical != 1 {
		t.Fatalf("corporate.invoice.issued.v1 rows = %d, want 1", canonical)
	}

	// --- Receivables ageing: the 45-days-past-due invoice is in 31-60 --------
	ageing, err := svc.ReceivablesAgeing(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(ageing.Currencies) != 1 || ageing.Currencies[0].Currency != "NGN" || ageing.Currencies[0].OutstandingMinor != 10000 || ageing.Currencies[0].Buckets[2].Bucket != "DAYS_31_60" || ageing.Currencies[0].Buckets[2].OutstandingMinor != 10000 {
		t.Fatalf("ageing = %+v", ageing)
	}

	// --- Cost allocation --------------------------------------------------------
	expense, err := svc.CreateExpense(ctx, scope, financeopsmodels.ExpenseInput{BranchID: branch, Category: "rent", CostCentre: "HQ", AmountMinor: 3001, Currency: "NGN", ReceiptMediaID: "media-rent", Description: "Depot rent"})
	if err != nil {
		t.Fatal(err)
	}
	exp := expense["id"].(string)
	allocation := financeopsmodels.CostAllocationInput{SourceType: "EXPENSE", SourceID: exp, Method: "MANUAL", Reason: "Shared depot", Targets: []financeopsmodels.CostAllocationTarget{{TargetType: "BRANCH", TargetID: branch, AmountMinor: 2000}, {TargetType: "CUSTOMER", TargetID: customer, AmountMinor: 1001}}}
	if _, err = svc.AllocateCost(ctx, scope, allocation); appCode(err) != "state_transition_invalid" {
		t.Fatalf("unapproved expense allocated: %v", err)
	}
	if expense, err = svc.ExpenseCommand(ctx, scope, exp, "submit", "", v64(expense, "version")); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ExpenseCommand(ctx, approver, exp, "approve", "", v64(expense, "version")); err != nil {
		t.Fatal(err)
	}
	bad := allocation
	bad.Targets = []financeopsmodels.CostAllocationTarget{{TargetType: "BRANCH", TargetID: branch, AmountMinor: 3000}}
	if _, err = svc.AllocateCost(ctx, scope, bad); appCode(err) != string(apperrors.CodeValidationFailed) {
		t.Fatalf("sum mismatch accepted: %v", err)
	}
	if _, err = svc.AllocateCost(ctx, scope, allocation); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.AllocateCost(ctx, scope, allocation); appCode(err) != "duplicate_financial_source" {
		t.Fatalf("duplicate allocation: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_cost_allocations SET amount_minor=1 WHERE source_id=$1`, exp); err == nil {
		t.Fatal("cost allocation was mutable")
	}
	// A plain approved expense in another currency, unallocated.
	usd, err := svc.CreateExpense(ctx, scope, financeopsmodels.ExpenseInput{BranchID: branch, Category: "tolls", CostCentre: "OPS", AmountMinor: 40, Currency: "USD", ReceiptMediaID: "media-toll", Description: "Toll"})
	if err != nil {
		t.Fatal(err)
	}
	usd, _ = svc.ExpenseCommand(ctx, scope, usd["id"].(string), "submit", "", v64(usd, "version"))
	if _, err = svc.ExpenseCommand(ctx, approver, usd["id"].(string), "approve", "", v64(usd, "version")); err != nil {
		t.Fatal(err)
	}

	// --- Profitability ----------------------------------------------------------
	report, err := svc.Profitability(ctx, scope, financeopsmodels.ProfitabilityInput{Dimension: "BRANCH"})
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]financeopsmodels.ProfitabilityLine{}
	for _, l := range report.Lines {
		lines[l.DimensionID+"/"+l.Currency] = l
	}
	ngn := lines[branch+"/NGN"]
	// Revenue 10000; the allocated rent counts once, as allocated (2000), not also as direct cost.
	if ngn.RevenueMinor != 10000 || ngn.DirectCostMinor != 0 || ngn.AllocatedCostMinor != 2000 || ngn.MarginMinor != 8000 {
		t.Fatalf("NGN branch line = %+v", ngn)
	}
	if u := lines[branch+"/USD"]; u.DirectCostMinor != 40 || u.MarginMinor != -40 || u.RevenueMinor != 0 {
		t.Fatalf("USD branch line = %+v (currencies must not mix)", u)
	}
	cust, err := svc.Profitability(ctx, scope, financeopsmodels.ProfitabilityInput{Dimension: "customer"})
	if err != nil || len(cust.Lines) != 1 || cust.Lines[0].AllocatedCostMinor != 1001 || cust.Lines[0].MarginMinor != 8999 {
		t.Fatalf("customer profitability = %+v %v", cust, err)
	}
	if _, err = svc.Profitability(ctx, scope, financeopsmodels.ProfitabilityInput{Dimension: "PLANET"}); appCode(err) != string(apperrors.CodeValidationFailed) {
		t.Fatalf("unsupported dimension: %v", err)
	}
	snap, err := svc.SnapshotProfitability(ctx, scope, financeopsmodels.ProfitabilityInput{Dimension: "BRANCH"})
	if err != nil || snap.SnapshotSetID == "" {
		t.Fatalf("snapshot = %+v %v", snap, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_profitability_snapshots SET margin_minor=0 WHERE snapshot_set_id=$1`, snap.SnapshotSetID); err == nil {
		t.Fatal("profitability snapshot was mutable")
	}

	// --- FIN-003: payroll inputs changed after calculation ----------------------
	membership, provider := ids.New("mem"), ids.New("pro")
	if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-dispatch-delivery-service','DISPATCH',$3,'EXISTING_PROVIDER_LINKED','ACTIVE')`, membership, scope.OrganizationID, provider); err != nil {
		t.Fatal(err)
	}
	rule, err := svc.CreateCompensationRule(ctx, scope, financeopsmodels.CompensationRuleInput{Name: "Monthly", RuleType: "SALARY", Currency: "NGN", CalculationConfiguration: json.RawMessage(`{"amountMinor":90000}`), EffectiveFrom: time.Now().UTC().AddDate(0, 0, -7)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.CompensationRuleCommand(ctx, approver, rule["id"].(string), "approve", v64(rule, "version")); err != nil {
		t.Fatal(err)
	}
	period, err := svc.CreatePayrollPeriod(ctx, scope, financeopsmodels.PayrollPeriodInput{BranchID: branch, Currency: "NGN", PeriodStart: time.Now().UTC().AddDate(0, 0, -1), PeriodEnd: time.Now().UTC().AddDate(0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	pid := period["id"].(string)
	if period, err = svc.CalculatePayroll(ctx, scope, pid, v64(period, "version")); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err = pool.QueryRow(ctx, `SELECT calculation_hash FROM fleet_partner_payroll_periods WHERE id=$1`, pid).Scan(&hash); err != nil || len(hash) != 64 {
		t.Fatalf("calculation hash = %q %v", hash, err)
	}
	// A second active member joins after calculation: approval must refuse.
	if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-dispatch-delivery-service','DISPATCH',$3,'EXISTING_PROVIDER_LINKED','ACTIVE')`, ids.New("mem"), scope.OrganizationID, ids.New("pro")); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ApprovePayroll(ctx, approver, pid, v64(period, "version")); appCode(err) != "payroll_inputs_changed" {
		t.Fatalf("approval after input change = %v, want payroll_inputs_changed", err)
	}
	if period, err = svc.CalculatePayroll(ctx, scope, pid, v64(period, "version")); err != nil {
		t.Fatal(err)
	}
	var recalculated string
	_ = pool.QueryRow(ctx, `SELECT calculation_hash FROM fleet_partner_payroll_periods WHERE id=$1`, pid).Scan(&recalculated)
	if recalculated == hash {
		t.Fatal("recalculation did not change the fingerprint")
	}
	if period, err = svc.ApprovePayroll(ctx, approver, pid, v64(period, "version")); err != nil || period["status"] != "APPROVED" {
		t.Fatalf("approval after recalculation = %v %v", period["status"], err)
	}
}
