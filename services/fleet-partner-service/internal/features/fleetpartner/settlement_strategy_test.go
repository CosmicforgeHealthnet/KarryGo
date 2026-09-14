package fleetpartner

// CMP-027: a compensation component is paid in exactly one place.
//
// The invariant these tests defend is the one nothing downstream can check for
// itself: payroll and work settlement run at different times over different
// tables, so if both decide a commission is theirs to pay, the provider is
// paid twice and the amounts look entirely legitimate.

import (
	"testing"
)

func commissionRuleWithStrategy(id, strategy string) CompensationRule {
	rule := commissionRule(id, 1500, CommissionBasisEligibleJobGross, RoundingHalfUp)
	rule.Config.SettlementStrategy = strategy
	return rule
}

// -----------------------------------------------------------------------------
// CMP-027A — the strategy is explicit, versioned and exclusive
// -----------------------------------------------------------------------------

func TestEveryRuleSettlesInExactlyOnePlace(t *testing.T) {
	for _, ruleType := range []string{
		RuleSalary, RulePerJob, RulePerShift, RuleCommission,
		RuleAllowance, RuleOvertime, RuleBonus, RuleDeduction,
	} {
		rule := activeRule("r_"+ruleType, ruleType, RuleConfig{
			AmountMinor: 1000, Currency: "NGN", Cadence: CadenceMonthly,
			RateBps: 1500, Basis: CommissionBasisEligibleJobGross,
		})
		if SettlesWithTheJob(rule) == AccruesToPayroll(rule) {
			t.Fatalf("%s is either paid in both places or neither", ruleType)
		}
	}
}

func TestDefaultStrategyPreservesExistingPlanBehaviour(t *testing.T) {
	// Adopting the explicit field must not silently restate any plan that
	// never declared one.
	if DefaultSettlementStrategy(RuleCommission) != StrategyImmediateWork {
		t.Fatal("commission defaulted away from immediate settlement")
	}
	if DefaultSettlementStrategy(RulePerJob) != StrategyImmediateWork {
		t.Fatal("per-job defaulted away from immediate settlement")
	}
	for _, ruleType := range []string{RuleSalary, RulePerShift, RuleAllowance, RuleOvertime} {
		if DefaultSettlementStrategy(ruleType) != StrategyPayrollAccrual {
			t.Fatalf("%s defaulted to immediate settlement", ruleType)
		}
	}
}

func TestSalaryCannotBeSettledPerJob(t *testing.T) {
	salary := activeRule("r_salary", RuleSalary, RuleConfig{
		AmountMinor: 30000000, Currency: "NGN", Cadence: CadenceMonthly,
		SettlementStrategy: StrategyImmediateWork,
	})
	if err := ValidateSettlementStrategy(salary); err == nil {
		t.Fatal("an immediate salary would pay a month's wage on every completed job")
	}
	if err := ValidateRule(salary); err == nil {
		t.Fatal("rule validation must refuse it too, not only the strategy check")
	}
}

func TestUnknownStrategyIsRefused(t *testing.T) {
	rule := commissionRuleWithStrategy("r_comm", "WHENEVER")
	if err := ValidateSettlementStrategy(rule); err == nil {
		t.Fatal("an unrecognised strategy must be refused, not defaulted")
	}
}

// -----------------------------------------------------------------------------
// CMP-027B — an immediately-settled commission is excluded from payroll
// -----------------------------------------------------------------------------

func TestImmediateCommissionIsPaidWithTheJobAndExcludedFromPayroll(t *testing.T) {
	rules := []CompensationRule{commissionRuleWithStrategy("r_comm", StrategyImmediateWork)}

	// Three jobs, each settling its own commission.
	var exclusions []WorkSettledComponent
	for i, jobID := range []string{"job_1", "job_2", "job_3"} {
		resolved, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", at(10))
		if err != nil {
			t.Fatal(err)
		}
		if resolved.ProviderShareMinor != 1500 {
			t.Fatalf("job %d provider share=%d, want the commission paid with the job", i, resolved.ProviderShareMinor)
		}
		if resolved.Matched[0].Settlement != SettlementWork {
			t.Fatalf("job %d commission was not recorded as work-settled", i)
		}
		record := CompletedWorkRecord{
			ID: "cw_" + jobID, OrganizationID: "org_1", MembershipID: "mem_1",
			SourceService: ServiceTaxi, SourceJobID: jobID,
		}
		resolution := CompensationResolution{
			ID: "res_" + jobID, MatchedRules: resolved.Matched, Currency: "NGN",
		}
		exclusions = append(exclusions,
			WorkSettledComponentsFor(record, resolution, "inst_"+jobID, "op_"+jobID)...)
	}

	if len(exclusions) != 3 {
		t.Fatalf("%d work-settled components recorded for three jobs", len(exclusions))
	}
	lookup := PayrollExclusions(exclusions)

	// Payroll now runs over the same three jobs. Every one of them must be
	// skipped: the commission is already in the provider's wallet.
	for _, jobID := range []string{"job_1", "job_2", "job_3"} {
		if !AlreadyPaidAtWorkLevel(lookup, ServiceTaxi, jobID, "r_comm") {
			t.Fatalf("payroll would pay %s commission a second time", jobID)
		}
	}
	// A different rule on the same job is NOT excluded: only what was paid.
	if AlreadyPaidAtWorkLevel(lookup, ServiceTaxi, "job_1", "r_allowance") {
		t.Fatal("an unrelated rule was excluded, so payroll would underpay")
	}
	// And a job that never settled is not excluded either.
	if AlreadyPaidAtWorkLevel(lookup, ServiceTaxi, "job_4", "r_comm") {
		t.Fatal("an unsettled job was excluded from payroll")
	}
	// Each component carries the linkage CMP-027 requires, so "already paid"
	// is auditable rather than asserted.
	for _, component := range exclusions {
		if component.CompletedWorkID == "" || component.CompensationResolutionID == "" ||
			component.SettlementInstructionID == "" || component.WalletPostingOperationID == "" {
			t.Fatalf("component is missing its source linkage: %+v", component)
		}
		if component.AmountMinor != 1500 || component.Currency != "NGN" {
			t.Fatalf("component amount is wrong: %+v", component)
		}
	}
}

// -----------------------------------------------------------------------------
// CMP-027C — an accrued commission is NOT paid with the job
// -----------------------------------------------------------------------------

func TestAccruedCommissionIsNotPaidWithTheJob(t *testing.T) {
	rules := []CompensationRule{commissionRuleWithStrategy("r_comm", StrategyPayrollAccrual)}

	resolved, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", at(10))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProviderShareMinor != 0 {
		t.Fatalf("an accrued commission paid %d with the job", resolved.ProviderShareMinor)
	}
	if resolved.OrganizationShareMinor != 10000 {
		t.Fatalf("organization share=%d", resolved.OrganizationShareMinor)
	}
	if len(resolved.Matched) != 1 || resolved.Matched[0].Settlement != SettlementPayroll {
		t.Fatalf("an accrued commission must be recorded at payroll level: %+v", resolved.Matched)
	}
	if resolved.Matched[0].Strategy != StrategyPayrollAccrual {
		t.Fatalf("the resolution must record the strategy it applied: %+v", resolved.Matched[0])
	}

	// Nothing was settled with the job, so payroll excludes nothing and will
	// pay the accrual exactly once.
	record := CompletedWorkRecord{ID: "cw_1", SourceService: ServiceTaxi, SourceJobID: "job_1"}
	resolution := CompensationResolution{ID: "res_1", MatchedRules: resolved.Matched, Currency: "NGN"}
	settled := WorkSettledComponentsFor(record, resolution, "inst_1", "op_1")
	if len(settled) != 0 {
		t.Fatalf("an accrued component was recorded as work-settled: %+v", settled)
	}
	if AlreadyPaidAtWorkLevel(PayrollExclusions(settled), ServiceTaxi, "job_1", "r_comm") {
		t.Fatal("payroll would skip an accrual that was never paid, underpaying the provider")
	}
}

// TestOneComponentCannotBeBothImmediateAndAccrued is the invariant stated
// directly: for any rule, exactly one of the two paths claims it.
func TestOneComponentCannotBeBothImmediateAndAccrued(t *testing.T) {
	for _, strategy := range []string{StrategyImmediateWork, StrategyPayrollAccrual} {
		rule := commissionRuleWithStrategy("r_comm", strategy)
		if err := ValidateSettlementStrategy(rule); err != nil {
			t.Fatalf("%s: %v", strategy, err)
		}
		settledWithJob := SettlesWithTheJob(rule)
		if settledWithJob == AccruesToPayroll(rule) {
			t.Fatalf("%s claimed by both paths or neither", strategy)
		}
		resolved, err := ResolveWorkCompensation([]CompensationRule{rule}, 10000, "NGN", ServiceTaxi, "driver", at(10))
		if err != nil {
			t.Fatal(err)
		}
		paidWithJob := resolved.ProviderShareMinor > 0
		if paidWithJob != settledWithJob {
			t.Fatalf("%s: strategy says settledWithJob=%v but resolution paid=%v",
				strategy, settledWithJob, paidWithJob)
		}
	}
}

// TestMixedPlanSplitsComponentsBetweenBothPlaces is the realistic case: a
// salaried provider who also earns an immediate per-job bonus rate.
func TestMixedPlanSplitsComponentsBetweenBothPlaces(t *testing.T) {
	rules := []CompensationRule{
		activeRule("r_salary", RuleSalary, RuleConfig{
			AmountMinor: 30000000, Currency: "NGN", Cadence: CadenceMonthly,
		}),
		activeRule("r_perjob", RulePerJob, RuleConfig{AmountMinor: 500, Currency: "NGN"}),
	}
	resolved, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", at(10))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProviderShareMinor != 500 {
		t.Fatalf("provider share=%d, want only the per-job component", resolved.ProviderShareMinor)
	}
	record := CompletedWorkRecord{ID: "cw_1", SourceService: ServiceTaxi, SourceJobID: "job_1"}
	resolution := CompensationResolution{ID: "res_1", MatchedRules: resolved.Matched, Currency: "NGN"}
	settled := WorkSettledComponentsFor(record, resolution, "inst_1", "op_1")
	if len(settled) != 1 || settled[0].RuleID != "r_perjob" {
		t.Fatalf("only the per-job component was paid with the job: %+v", settled)
	}
	// Payroll skips the per-job component and still pays the salary.
	lookup := PayrollExclusions(settled)
	if !AlreadyPaidAtWorkLevel(lookup, ServiceTaxi, "job_1", "r_perjob") {
		t.Fatal("payroll would pay the per-job component twice")
	}
	if AlreadyPaidAtWorkLevel(lookup, ServiceTaxi, "job_1", "r_salary") {
		t.Fatal("payroll would skip the salary, which was never paid with the job")
	}
}
