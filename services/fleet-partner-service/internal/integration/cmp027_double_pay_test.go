// CMP-027: a compensation component paid with a completed job's own money must
// never be paid a second time by payroll.
//
// This is the release blocker Section 13 names, and it is checked here against
// a real database rather than in the domain package alone: work settlement and
// payroll are computed by different code, at different times, from different
// tables, and the only thing that actually stops the double payment is the
// durable record one writes and the other reads. A unit test over the domain
// helpers would pass even if payroll never consulted that record at all.
package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// seedCompletedAssignment creates one completed assignment for a fresh
// membership, which is exactly what payroll's per-job pass walks.
func seedCompletedAssignment(t *testing.T, pool *pgxpool.Pool, scope tenant.Context, branch string) (membership, assignment string) {
	t.Helper()
	ctx := context.Background()
	membership = ids.New("mem")
	provider := ids.New("pro")
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-dispatch-delivery-service','DISPATCH',$3,'EXISTING_PROVIDER_LINKED','ACTIVE')`, membership, scope.OrganizationID, provider); err != nil {
		t.Fatal(err)
	}
	assignment = ids.New("asn")
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,route_stop_id,provider_membership_id,provider_id,execution_service,status,response_deadline,created_by,updated_at)VALUES($1,$2,$3,'ROUTE_STOP',$4,$5,$6,'DISPATCH','COMPLETED',now()+interval '1 hour',$7,now())`,
		assignment, scope.OrganizationID, branch, "stop-"+assignment, membership, provider, scope.ActorID); err != nil {
		t.Fatal(err)
	}
	return membership, assignment
}

// recordWorkSettled writes the durable "this was already paid with the job's
// money" record that Fleet writes in the same transaction as the settlement
// instruction.
func recordWorkSettled(t *testing.T, pool *pgxpool.Pool, orgID, membership, assignment, ruleID string, amount int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_work_settled_components
		(id,organization_id,membership_id,source_service,source_job_id,source_assignment_id,
		 completed_work_id,compensation_resolution_id,settlement_instruction_id,wallet_posting_operation_id,
		 rule_id,component,settlement_level,settlement_strategy,amount_minor,currency)
		VALUES($1,$2,$3,'DISPATCH',$4,$5,$6,$7,$8,$9,$10,'PER_JOB','WORK','IMMEDIATE_WORK_SETTLEMENT',$11,'NGN')`,
		ids.New("wsc"), orgID, membership, "job-"+assignment, assignment,
		"cwr-"+assignment, "cre-"+assignment, "msi-"+assignment, "wop-"+assignment,
		ruleID, amount); err != nil {
		t.Fatal(err)
	}
}

// TestCMP027_PayrollDoesNotRepayWorkSettledComponent is FIN-31.
//
// Two identical completed assignments, one per-job rule, one payroll period.
// One assignment was already settled with its own job's money; the other was
// not. Payroll must produce exactly one compensation item, for the assignment
// that has not been paid.
//
// The control assignment is what makes this a real test: without it, a payroll
// pass that had simply stopped producing per-job items at all -- a regression,
// not a fix -- would still pass.
func TestCMP027_PayrollDoesNotRepayWorkSettledComponent(t *testing.T) {
	pool := testPool(t)
	scope, branch, _ := section2223Scope(t, pool)
	ctx := context.Background()

	svc := financeops.NewService(financeops.Options{
		Pool:   pool,
		Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Wallet: &sectionPosting{},
		Flags:  financeops.Flags{Compensation: true, Payroll: true},
	})
	approver := scope
	approver.ActorID = ids.New("usr")
	approver.OrganizationUserID = approver.ActorID

	paidMembership, paidAssignment := seedCompletedAssignment(t, pool, scope, branch)
	_, unpaidAssignment := seedCompletedAssignment(t, pool, scope, branch)

	rule, err := svc.CreateCompensationRule(ctx, scope, financeops.CompensationRuleInput{
		Name: "Per job", RuleType: "PER_JOB", Currency: "NGN",
		CalculationConfiguration: json.RawMessage(`{"amountMinor":700}`),
		EffectiveFrom:            time.Now().UTC().AddDate(0, 0, -7),
	})
	if err != nil {
		t.Fatal(err)
	}
	ruleID := rule["id"].(string)
	if _, err = svc.CompensationRuleCommand(ctx, approver, ruleID, "approve", v64(rule, "version")); err != nil {
		t.Fatal(err)
	}

	// The first assignment's per-job component was already paid with the job's
	// money, by this exact rule.
	recordWorkSettled(t, pool, scope.OrganizationID, paidMembership, paidAssignment, ruleID, 700)

	period, err := svc.CreatePayrollPeriod(ctx, scope, financeops.PayrollPeriodInput{
		BranchID: branch, Currency: "NGN",
		PeriodStart: time.Now().UTC().AddDate(0, 0, -1),
		PeriodEnd:   time.Now().UTC().AddDate(0, 0, 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.CalculatePayroll(ctx, scope, period["id"].(string), v64(period, "version")); err != nil {
		t.Fatal(err)
	}

	var paidAgain int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_provider_compensation_items WHERE payroll_period_id=$1 AND rule_id=$2 AND source_reference=$3`,
		period["id"], ruleID, paidAssignment).Scan(&paidAgain); err != nil {
		t.Fatal(err)
	}
	if paidAgain != 0 {
		t.Fatalf("payroll re-paid a component already settled with the job's money: %d item(s) for assignment %s", paidAgain, paidAssignment)
	}

	var stillOwed int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_provider_compensation_items WHERE payroll_period_id=$1 AND rule_id=$2 AND source_reference=$3`,
		period["id"], ruleID, unpaidAssignment).Scan(&stillOwed); err != nil {
		t.Fatal(err)
	}
	if stillOwed != 1 {
		t.Fatalf("payroll failed to pay an unsettled component: %d item(s) for assignment %s, want 1", stillOwed, unpaidAssignment)
	}
}

// TestCMP027_RecalculationDoesNotAccumulateDuplicates: recalculating a payroll
// period must not turn one owed component into two, and must not resurrect a
// component that work settlement already paid.
func TestCMP027_RecalculationDoesNotAccumulateDuplicates(t *testing.T) {
	pool := testPool(t)
	scope, branch, _ := section2223Scope(t, pool)
	ctx := context.Background()

	svc := financeops.NewService(financeops.Options{
		Pool:   pool,
		Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Wallet: &sectionPosting{},
		Flags:  financeops.Flags{Compensation: true, Payroll: true},
	})
	approver := scope
	approver.ActorID = ids.New("usr")
	approver.OrganizationUserID = approver.ActorID

	paidMembership, paidAssignment := seedCompletedAssignment(t, pool, scope, branch)

	rule, err := svc.CreateCompensationRule(ctx, scope, financeops.CompensationRuleInput{
		Name: "Per job", RuleType: "PER_JOB", Currency: "NGN",
		CalculationConfiguration: json.RawMessage(`{"amountMinor":900}`),
		EffectiveFrom:            time.Now().UTC().AddDate(0, 0, -7),
	})
	if err != nil {
		t.Fatal(err)
	}
	ruleID := rule["id"].(string)
	if _, err = svc.CompensationRuleCommand(ctx, approver, ruleID, "approve", v64(rule, "version")); err != nil {
		t.Fatal(err)
	}
	recordWorkSettled(t, pool, scope.OrganizationID, paidMembership, paidAssignment, ruleID, 900)

	period, err := svc.CreatePayrollPeriod(ctx, scope, financeops.PayrollPeriodInput{
		BranchID: branch, Currency: "NGN",
		PeriodStart: time.Now().UTC().AddDate(0, 0, -1),
		PeriodEnd:   time.Now().UTC().AddDate(0, 0, 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	periodID := period["id"].(string)
	for attempt := 0; attempt < 3; attempt++ {
		period, err = svc.CalculatePayroll(ctx, scope, periodID, v64(period, "version"))
		if err != nil {
			t.Fatalf("recalculation %d: %v", attempt, err)
		}
	}

	var total int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_provider_compensation_items WHERE payroll_period_id=$1 AND rule_id=$2`, periodID, ruleID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("repeated recalculation produced %d per-job item(s) for a component already paid with the job's money, want 0", total)
	}
}
