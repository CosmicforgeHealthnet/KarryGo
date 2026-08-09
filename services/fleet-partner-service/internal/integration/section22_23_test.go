package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type section2223Media struct{}

func (section2223Media) Configured() bool { return true }
func (section2223Media) GetFile(_ context.Context, meta platformclients.CallMeta, id string) (platformclients.FileMetadata, error) {
	return platformclients.FileMetadata{FileID: id, OwnerID: meta.OrganizationID, UploadState: "COMPLETE", ScanState: "CLEAN", SourceVersion: 1}, nil
}

type sectionPosting struct {
	mu              sync.Mutex
	submitted, read int
}

func (*sectionPosting) Configured(string) bool { return true }
func (p *sectionPosting) Submit(_ context.Context, _ platformclients.CallMeta, r financeops.PostingRequest) (financeops.PostingOutcome, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.submitted++
	return financeops.PostingOutcome{Reference: "wallet-" + r.RecordID, Status: "POSTED", SourceVersion: 1}, &platformclients.Error{Service: "wallet-test", Outcome: platformclients.OutcomeUnknown}
}
func (p *sectionPosting) ReadByKey(_ context.Context, _ platformclients.CallMeta, r financeops.PostingRequest) (financeops.PostingOutcome, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.read++
	return financeops.PostingOutcome{Reference: "wallet-" + r.RecordID, Status: "POSTED", SourceVersion: 1}, nil
}

type sectionSupport struct {
	mu      sync.Mutex
	created int
	cases   map[string]engagement.SupportProjection
}

func (s *sectionSupport) Configured() bool { return true }
func (s *sectionSupport) Create(_ context.Context, org, key string, _ engagement.SupportCreateInput) (engagement.SupportProjection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.cases[org+":"+key]; ok {
		return v, nil
	}
	s.created++
	v := engagement.SupportProjection{CaseID: ids.New("sup"), Status: "open", Priority: "high", SourceVersion: 1}
	s.cases[org+":"+key] = v
	return v, nil
}
func (s *sectionSupport) Read(_ context.Context, org, id string) (engagement.SupportProjection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.cases {
		if len(k) > len(org) && v.CaseID == id {
			return v, nil
		}
	}
	return engagement.SupportProjection{}, errors.New("not found")
}

type sectionNotifications struct {
	mu        sync.Mutex
	submitted map[string]engagement.DeliveryOutcome
}

func (n *sectionNotifications) Configured() bool { return true }
func (n *sectionNotifications) Submit(_ context.Context, org, key string, _ json.RawMessage) (engagement.DeliveryOutcome, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	v := engagement.DeliveryOutcome{Reference: "notification-" + key, Status: "queued", SourceVersion: 1}
	n.submitted[org+":"+key] = v
	return v, nil
}
func (n *sectionNotifications) Read(_ context.Context, org, key string) (engagement.DeliveryOutcome, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	v, ok := n.submitted[org+":"+key]
	if !ok {
		return engagement.DeliveryOutcome{}, errors.New("not found")
	}
	return v, nil
}

func section2223Scope(t *testing.T, pool *pgxpool.Pool) (tenant.Context, string, string) {
	t.Helper()
	org := createOrganization(t, pool, "Sections 22 23 "+ids.New("org"), "S2223-"+ids.New("reg"))
	branch := ids.New("brn")
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,$3,'HQ','Europe/Rome')`, branch, org.ID, "B"+branch[len(branch)-6:]); err != nil {
		t.Fatal(err)
	}
	customer := ids.New("cus")
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_company_customers(id,organization_id,branch_id,customer_type,legal_name,display_name,normalized_name,duplicate_fingerprint,status,default_currency,created_by)VALUES($1,$2,$3,'BUSINESS','Test Customer','Test Customer',$4,$5,'ACTIVE','NGN',$6)`, customer, org.ID, branch, "test customer "+customer, customer, "seed"); err != nil {
		t.Fatal(err)
	}
	actor := ids.New("usr")
	return tenant.Context{OrganizationID: org.ID, OrganizationUserID: actor, ActorType: tenant.ActorTypeOrganizationUser, ActorID: actor, CorrelationID: ids.New("cor"), PermittedBranchIDs: []string{branch}}, branch, customer
}
func v64(v map[string]any, key string) int64 {
	if x, ok := v[key].(float64); ok {
		return int64(x)
	}
	if x, ok := v[key].(int64); ok {
		return x
	}
	return 0
}

func TestSection22FinancePayrollAndAuthorityBoundaries(t *testing.T) {
	pool := testPool(t)
	scope, branch, customer := section2223Scope(t, pool)
	ctx := context.Background()
	posting := &sectionPosting{}
	svc := financeops.NewService(financeops.Options{Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Media: section2223Media{}, Wallet: posting, Flags: financeops.Flags{Invoices: true, Receivables: true, Expenses: true, Budgets: true, Compensation: true, Payroll: true, Settlements: true, Claims: true, Posting: true}})
	invoice, err := svc.CreateInvoice(ctx, scope, financeops.InvoiceInput{BranchID: branch, CustomerID: customer, Currency: "ngn", DueDate: time.Now().UTC().Add(24 * time.Hour), Lines: []financeops.InvoiceLineInput{{SourceType: "SHIPMENT", SourceReference: "shipment-1", Description: "Delivery", Quantity: 3, UnitAmountMinor: 1200, TaxAmountMinor: 200}}})
	if err != nil {
		t.Fatal(err)
	}
	if v64(invoice, "subtotal_minor") != 3600 || v64(invoice, "total_minor") != 3800 {
		t.Fatalf("server totals incorrect: %+v", invoice)
	}
	invoice, err = svc.ApproveInvoice(ctx, scope, invoice["id"].(string), v64(invoice, "version"))
	if err != nil {
		t.Fatal(err)
	}
	invoice, err = svc.IssueInvoice(ctx, scope, invoice["id"].(string), v64(invoice, "version"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.UpdateInvoice(ctx, scope, invoice["id"].(string), financeops.InvoiceInput{ExpectedVersion: v64(invoice, "version"), CustomerID: customer, Currency: "NGN", DueDate: time.Now().Add(time.Hour), Lines: []financeops.InvoiceLineInput{{SourceType: "SHIPMENT", SourceReference: "x", Description: "x", Quantity: 1, UnitAmountMinor: 1}}}); err == nil {
		t.Fatal("issued invoice was mutable")
	}
	receivables, err := svc.ListReceivables(ctx, scope)
	if err != nil || len(receivables) != 1 {
		t.Fatalf("receivable=%+v err=%v", receivables, err)
	}
	if err = svc.ApplyPayment(ctx, financeops.PaymentEvent{EventID: "pay-1", OrganizationID: scope.OrganizationID, InvoiceID: invoice["id"].(string), PaymentReference: "payment-1", Currency: "NGN", AmountMinor: 1800, SourceVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if err = svc.ApplyPayment(ctx, financeops.PaymentEvent{EventID: "pay-1", OrganizationID: scope.OrganizationID, InvoiceID: invoice["id"].(string), PaymentReference: "payment-1", Currency: "NGN", AmountMinor: 1800, SourceVersion: 1}); err != nil {
		t.Fatal(err)
	}
	var paymentRows int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_receivable_payment_history WHERE organization_id=$1`, scope.OrganizationID).Scan(&paymentRows); err != nil || paymentRows != 1 {
		t.Fatalf("payment rows=%d err=%v", paymentRows, err)
	}
	invoice, err = svc.GetInvoice(ctx, scope, invoice["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.CreditInvoice(ctx, scope, invoice["id"].(string), financeops.CreditNoteInput{AmountMinor: 1000, Reason: "service credit", ExpectedVersion: v64(invoice, "version")}); err != nil {
		t.Fatal(err)
	}
	receivables, err = svc.ListReceivables(ctx, scope)
	if err != nil || v64(receivables[0], "original_amount_minor") != 2800 || v64(receivables[0], "paid_amount_minor") != 1800 || v64(receivables[0], "outstanding_amount_minor") != 1000 {
		t.Fatalf("credited receivable=%+v err=%v", receivables, err)
	}
	invoice, err = svc.GetInvoice(ctx, scope, invoice["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.CreditInvoice(ctx, scope, invoice["id"].(string), financeops.CreditNoteInput{AmountMinor: 1100, Reason: "too much", ExpectedVersion: v64(invoice, "version")}); err == nil {
		t.Fatal("over-credit accepted")
	}

	expense, err := svc.CreateExpense(ctx, scope, financeops.ExpenseInput{BranchID: branch, Category: "fuel", CostCentre: "OPS", AmountMinor: 2500, Currency: "NGN", ReceiptMediaID: "media-receipt", Description: "Fuel"})
	if err != nil {
		t.Fatal(err)
	}
	expense, err = svc.ExpenseCommand(ctx, scope, expense["id"].(string), "submit", "", v64(expense, "version"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ExpenseCommand(ctx, scope, expense["id"].(string), "approve", "", v64(expense, "version")); err == nil {
		t.Fatal("submitter approved own expense")
	}
	approver := scope
	approver.ActorID = ids.New("usr")
	approver.OrganizationUserID = approver.ActorID
	expense, err = svc.ExpenseCommand(ctx, approver, expense["id"].(string), "approve", "", v64(expense, "version"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_expenses SET status='POSTED' WHERE organization_id=$1 AND id=$2`, scope.OrganizationID, expense["id"]); err != nil {
		t.Fatal(err)
	}
	expense, err = svc.ExpenseCommand(ctx, approver, expense["id"].(string), "reverse", "duplicate receipt", v64(expense, "version"))
	if err != nil {
		t.Fatal(err)
	}
	var reversals int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_expense_reversals WHERE organization_id=$1 AND posting_operation_id IS NOT NULL`, scope.OrganizationID).Scan(&reversals)
	if reversals != 1 {
		t.Fatalf("expense reversals=%d", reversals)
	}
	budget, err := svc.CreateBudget(ctx, scope, financeops.BudgetInput{CostCentre: "OPS", Category: "FUEL", Currency: "NGN", PeriodStart: time.Now().UTC().AddDate(0, 0, -1), PeriodEnd: time.Now().UTC().AddDate(0, 1, 0), ApprovedAmountMinor: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.BudgetCommand(ctx, approver, budget["id"].(string), "approve", v64(budget, "version")); err != nil {
		t.Fatal(err)
	}
	amount := int64(5000)
	claim, err := svc.CreateClaim(ctx, scope, financeops.ClaimInput{ClaimType: "cargo", Currency: "NGN", AmountReferenceMinor: &amount, EvidenceMediaIDs: []string{"media-claim"}, ExternalClaimReference: "insurer-1", SettlementReference: "settlement-external"})
	if err != nil || claim["external_claim_reference"] != "insurer-1" {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}

	membership := ids.New("mem")
	provider := ids.New("pro")
	if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-dispatch-delivery-service','DISPATCH',$3,'EXISTING_PROVIDER_LINKED','ACTIVE')`, membership, scope.OrganizationID, provider); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_provider_subwallet_links(provider_membership_id,organization_id,wallet_account_id,status,provider_id,provider_service,currency,freshness_state)VALUES($1,$2,$3,'ACTIVE',$4,'driver-dispatch-delivery-service','NGN','CURRENT')`, membership, scope.OrganizationID, "subwallet-"+membership, provider); err != nil {
		t.Fatal(err)
	}
	assignment := ids.New("asn")
	if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,route_stop_id,provider_membership_id,provider_id,execution_service,status,response_deadline,created_by,updated_at)VALUES($1,$2,$3,'ROUTE_STOP',$4,$5,$6,'DISPATCH','COMPLETED',now()+interval '1 hour',$7,now())`, assignment, scope.OrganizationID, branch, "stop-"+assignment, membership, provider, scope.ActorID); err != nil {
		t.Fatal(err)
	}
	rule, err := svc.CreateCompensationRule(ctx, scope, financeops.CompensationRuleInput{Name: "Per job", RuleType: "PER_JOB", Currency: "NGN", CalculationConfiguration: json.RawMessage(`{"amountMinor":700}`), EffectiveFrom: time.Now().UTC().AddDate(0, 0, -7)})
	if err != nil {
		t.Fatal(err)
	}
	rule, err = svc.CompensationRuleCommand(ctx, approver, rule["id"].(string), "approve", v64(rule, "version"))
	if err != nil {
		t.Fatal(err)
	}
	period, err := svc.CreatePayrollPeriod(ctx, scope, financeops.PayrollPeriodInput{BranchID: branch, Currency: "NGN", PeriodStart: time.Now().UTC().AddDate(0, 0, -1), PeriodEnd: time.Now().UTC().AddDate(0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	period, err = svc.CalculatePayroll(ctx, scope, period["id"].(string), v64(period, "version"))
	if err != nil {
		t.Fatal(err)
	}
	var forbiddenDeductions int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_provider_compensation_items WHERE payroll_period_id=$1 AND (amount_minor<0 OR source_type IN('DECLINE','SAFETY_REPORT','SAFETY_DECLINE'))`, period["id"]).Scan(&forbiddenDeductions)
	if forbiddenDeductions != 0 {
		t.Fatal("safety/decline deduction generated")
	}
	period, err = svc.ApprovePayroll(ctx, approver, period["id"].(string), v64(period, "version"))
	if err != nil {
		t.Fatal(err)
	}
	period, err = svc.PostPayroll(ctx, approver, period["id"].(string), "payroll-post-1", v64(period, "version"))
	if err != nil {
		t.Fatal(err)
	}
	if period["status"] != "POSTED" || posting.submitted == 0 || posting.read != posting.submitted {
		t.Fatalf("period=%+v submit=%d read=%d", period, posting.submitted, posting.read)
	}
	statement, err := svc.ProviderStatements(ctx, scope, membership)
	if err != nil || len(statement.Periods) == 0 {
		t.Fatalf("statement=%+v err=%v", statement, err)
	}
	period, err = svc.ReversePayroll(ctx, approver, period["id"].(string), "corrected payroll", v64(period, "version"))
	if err != nil || period["status"] != "REVERSED" {
		t.Fatalf("payroll reversal=%+v err=%v", period, err)
	}
	settlement, err := svc.CreateSettlement(ctx, scope, financeops.SettlementInput{SupplierReference: "supplier", ProviderOrCarrierReference: provider, Currency: "NGN", PeriodStart: time.Now().AddDate(0, 0, -2), PeriodEnd: time.Now(), JobReferences: []string{assignment}, AmountMinor: 700})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.CreateSettlement(ctx, scope, financeops.SettlementInput{SupplierReference: "supplier2", ProviderOrCarrierReference: provider, Currency: "NGN", PeriodStart: time.Now().AddDate(0, 0, -2), PeriodEnd: time.Now(), JobReferences: []string{assignment}, AmountMinor: 700}); err == nil {
		t.Fatalf("duplicate settled job accepted after %+v", settlement)
	}
	var ledgers int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE 'fleet_partner%ledger%'`).Scan(&ledgers)
	if ledgers != 0 {
		t.Fatalf("Fleet ledger tables=%d", ledgers)
	}
}

func TestSection23CommunicationsSupportQualityAndSLA(t *testing.T) {
	pool := testPool(t)
	scope, branch, _ := section2223Scope(t, pool)
	ctx := context.Background()
	support := &sectionSupport{cases: map[string]engagement.SupportProjection{}}
	notifications := &sectionNotifications{submitted: map[string]engagement.DeliveryOutcome{}}
	svc := engagement.NewService(engagement.Options{Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Media: section2223Media{}, Support: support, Notification: notifications, Flags: engagement.Flags{Communications: true, NotificationRules: true, Support: true, Quality: true, SLA: true, Escalations: true}})
	thread, err := svc.CreateThread(ctx, scope, engagement.ThreadInput{BranchID: branch, SubjectType: "ORDER", SubjectReference: "order-1", Title: "Restricted work", Visibility: "RESTRICTED", Participants: []engagement.ParticipantInput{{ParticipantType: "CUSTOMER", ParticipantReference: "customer-1"}}})
	if err != nil {
		t.Fatal(err)
	}
	outsider := scope
	outsider.ActorID = ids.New("usr")
	outsider.OrganizationUserID = outsider.ActorID
	if _, err = svc.GetThread(ctx, outsider, thread["id"].(string)); err == nil {
		t.Fatal("restricted thread disclosed to non-participant")
	}
	note, err := svc.SendMessage(ctx, scope, thread["id"].(string), "message-note", engagement.MessageInput{Channel: "INTERNAL_NOTE", Text: "internal only", SourceTimestamp: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	var intents int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_notification_intents WHERE source_reference=$1`, note["id"]).Scan(&intents)
	if intents != 0 {
		t.Fatal("internal note generated external delivery")
	}
	message, err := svc.SendMessage(ctx, scope, thread["id"].(string), "message-external", engagement.MessageInput{Channel: "IN_APP", Text: "safe update", MediaFileIDs: []string{"media-message"}, SourceTimestamp: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.SendMessage(ctx, scope, thread["id"].(string), "message-external", engagement.MessageInput{Channel: "IN_APP", Text: "safe update", SourceTimestamp: time.Now().UTC()})
	if err != nil || replayed["id"] != message["id"] {
		t.Fatalf("message replay=%+v err=%v", replayed, err)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_notification_intents WHERE source_reference=$1`, message["id"]).Scan(&intents)
	if intents != 1 {
		t.Fatalf("notification intents=%d", intents)
	}
	if _, err = svc.SaveNotificationRule(ctx, scope, engagement.NotificationRuleInput{EventType: "invoice.overdue", RecipientScope: "OPERATIONS", Timezone: "Invalid/Timezone", Channels: json.RawMessage(`["IN_APP"]`), ExpectedVersion: 1}); err == nil {
		t.Fatal("invalid timezone accepted")
	}
	if _, err = svc.SaveNotificationRule(ctx, scope, engagement.NotificationRuleInput{EventType: "invoice.overdue", RecipientScope: "OPERATIONS", Timezone: "Europe/Rome", Channels: json.RawMessage(`["IN_APP"]`), QuietHours: json.RawMessage(`{"start":"22:00","end":"07:00"}`), ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}

	case1, err := svc.CreateSupportCase(ctx, scope, "support-create-1", engagement.SupportCreateInput{CaseType: "PLATFORM", Priority: "HIGH", Subject: "Need help", Description: "Canonical support case"})
	if err != nil {
		t.Fatal(err)
	}
	case2, err := svc.CreateSupportCase(ctx, scope, "support-create-1", engagement.SupportCreateInput{CaseType: "PLATFORM", Priority: "HIGH", Subject: "Need help", Description: "Canonical support case"})
	if err != nil || case1["id"] != case2["id"] || support.created != 1 {
		t.Fatalf("support idempotency c1=%+v c2=%+v created=%d err=%v", case1, case2, support.created, err)
	}
	caseID := case1["support_service_case_id"].(string)
	if err = svc.ApplySupportEvent(ctx, engagement.SupportEvent{EventID: "support-event-3", OrganizationID: scope.OrganizationID, CaseID: caseID, EventType: "status_changed", Status: "under_review", Priority: "high", SourceVersion: 3}); err != nil {
		t.Fatal(err)
	}
	projection, err := svc.GetSupportCase(ctx, scope, caseID)
	if err != nil || projection["gap_status"] != "GAP_DETECTED" {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
	if err = svc.ApplySupportEvent(ctx, engagement.SupportEvent{EventID: "support-event-3", OrganizationID: scope.OrganizationID, CaseID: caseID, EventType: "status_changed", Status: "under_review", Priority: "high", SourceVersion: 3}); err != nil {
		t.Fatal(err)
	}
	var supportEvents int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_support_case_events_inbox WHERE event_id='support-event-3'`).Scan(&supportEvents)
	if supportEvents != 1 {
		t.Fatalf("support inbox duplicates=%d", supportEvents)
	}

	quality, err := svc.CreateQualityCase(ctx, scope, engagement.QualityInput{BranchID: branch, Category: "DELIVERY", Severity: "HIGH", SourceType: "SHIPMENT", SourceReference: "shipment-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.UpdateQualityCase(ctx, scope, quality["id"].(string), engagement.QualityInput{ExpectedVersion: v64(quality, "version")}, true); err == nil {
		t.Fatal("quality resolved without root cause/action")
	}
	quality, err = svc.UpdateQualityCase(ctx, scope, quality["id"].(string), engagement.QualityInput{ExpectedVersion: v64(quality, "version"), RootCause: "routing", CorrectiveActionSummary: "new review gate"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if quality["status"] != "RESOLVED" {
		t.Fatalf("quality status=%v", quality["status"])
	}
	now := time.Now().UTC().Truncate(time.Second)
	unknown, err := svc.RecordMeasurement(ctx, engagement.MeasurementInput{OrganizationID: scope.OrganizationID, AgreementReference: "sla-1", MetricCode: "ON_TIME", FormulaVersion: 1, PeriodStart: now.Add(-time.Hour), PeriodEnd: now, TargetValue: "95.00", Unit: "PERCENT", Complete: false})
	if err != nil || unknown["breach_status"] != "UNKNOWN" {
		t.Fatalf("unknown=%+v err=%v", unknown, err)
	}
	breach, err := svc.RecordMeasurement(ctx, engagement.MeasurementInput{OrganizationID: scope.OrganizationID, AgreementReference: "sla-1", MetricCode: "ON_TIME", FormulaVersion: 1, PeriodStart: now, PeriodEnd: now.Add(time.Hour), TargetValue: "95.00", MeasuredValue: "90.00", Unit: "PERCENT", Complete: true, HigherIsBetter: true})
	if err != nil || breach["breach_status"] != "BREACHED" {
		t.Fatalf("breach=%+v err=%v", breach, err)
	}
	var escalations int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_escalations WHERE organization_id=$1 AND source_type='SLA_MEASUREMENT'`, scope.OrganizationID).Scan(&escalations)
	if escalations != 1 {
		t.Fatalf("escalations=%d", escalations)
	}
	var escalationID string
	var version int64
	_ = pool.QueryRow(ctx, `SELECT id,version FROM fleet_partner_escalations WHERE organization_id=$1 AND source_type='SLA_MEASUREMENT'`, scope.OrganizationID).Scan(&escalationID, &version)
	if _, err = svc.EscalationCommand(ctx, scope, escalationID, "acknowledge", version); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.EscalationCommand(ctx, scope, escalationID, "resolve", version); err == nil {
		t.Fatal("stale escalation version accepted")
	}
	var supportProjectionCount, qualityCount int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_support_case_projections WHERE organization_id=$1`, scope.OrganizationID).Scan(&supportProjectionCount)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_quality_cases WHERE organization_id=$1`, scope.OrganizationID).Scan(&qualityCount)
	if supportProjectionCount != 1 || qualityCount != 1 {
		t.Fatal("quality and platform support were conflated")
	}
}
