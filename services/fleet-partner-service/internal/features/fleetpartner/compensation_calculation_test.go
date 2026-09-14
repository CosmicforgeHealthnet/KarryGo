package fleetpartner

// Section 13: compensation resolution for one completed job.
//
// These are unit tests because the arithmetic and the rule matching are pure:
// what is being pinned is that the same plan and the same job always produce
// the same money, and that the components which belong to a payroll period
// never leak into a job payout.

import (
	"errors"
	"testing"
	"time"
)

func at(day int) time.Time {
	return time.Date(2026, 6, day, 12, 0, 0, 0, time.UTC)
}

func activeRule(id, ruleType string, config RuleConfig) CompensationRule {
	return CompensationRule{
		ID: id, RuleType: ruleType, Status: PlanActive, Config: config,
		EffectiveFrom: at(1),
	}
}

func commissionRule(id string, rateBps int, basis, rounding string) CompensationRule {
	return activeRule(id, RuleCommission, RuleConfig{
		RateBps: rateBps, Basis: basis, Rounding: rounding, Currency: "NGN",
	})
}

// -----------------------------------------------------------------------------
// CMP-022 / FIN-02 / Invariant 2 — salary does not pay per job
// -----------------------------------------------------------------------------

func TestPureSalaryResolvesToZeroProviderJobShare(t *testing.T) {
	rules := []CompensationRule{
		activeRule("r_salary", RuleSalary, RuleConfig{AmountMinor: 30000000, Currency: "NGN", Cadence: CadenceMonthly}),
	}
	resolved, err := ResolveWorkCompensation(rules, 9000, "NGN", ServiceTaxi, "driver", at(10))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProviderShareMinor != 0 {
		t.Fatalf("salaried provider job share=%d, want 0: anything else is pay on top of salary", resolved.ProviderShareMinor)
	}
	if resolved.OrganizationShareMinor != 9000 {
		t.Fatalf("organization share=%d, want the whole eligible basis", resolved.OrganizationShareMinor)
	}
	if len(resolved.Matched) != 1 || resolved.Matched[0].Settlement != SettlementPayroll {
		t.Fatalf("salary must be recorded as a payroll-level component: %+v", resolved.Matched)
	}
}

// -----------------------------------------------------------------------------
// CMP-010 / CMP-016 — per-shift is payroll, not per job
// -----------------------------------------------------------------------------

func TestPerShiftIsNotPaidPerJob(t *testing.T) {
	rules := []CompensationRule{
		activeRule("r_shift", RulePerShift, RuleConfig{AmountMinor: 500000, Currency: "NGN"}),
	}
	resolved, err := ResolveWorkCompensation(rules, 9000, "NGN", ServiceDispatch, "rider", at(10))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProviderShareMinor != 0 {
		t.Fatalf("per-shift paid %d at job level: a shift with ten jobs would pay the shift rate ten times", resolved.ProviderShareMinor)
	}
	if SettlementLevelFor(RulePerShift) != SettlementPayroll {
		t.Fatal("per-shift must settle through payroll")
	}
}

func TestEveryRuleTypeHasAnExplicitSettlementLevel(t *testing.T) {
	work := map[string]bool{RulePerJob: true, RuleCommission: true}
	for _, ruleType := range []string{
		RuleSalary, RulePerJob, RulePerShift, RuleCommission,
		RuleAllowance, RuleOvertime, RuleBonus, RuleDeduction,
	} {
		level := SettlementLevelFor(ruleType)
		want := SettlementPayroll
		if work[ruleType] {
			want = SettlementWork
		}
		if level != want {
			t.Fatalf("%s settles at %s, want %s", ruleType, level, want)
		}
	}
}

// -----------------------------------------------------------------------------
// CMP-008 / CMP-023 / FIN-03 — commission
// -----------------------------------------------------------------------------

func TestCommissionUsesBasisPointsAgainstTheDeclaredBasis(t *testing.T) {
	// The specification's own worked example: basis 10000, rate 1500 bps,
	// provider share 1500 -- not the whole 10000.
	rules := []CompensationRule{commissionRule("r_comm", 1500, CommissionBasisEligibleJobGross, RoundingHalfUp)}
	resolved, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", at(10))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProviderShareMinor != 1500 {
		t.Fatalf("commission=%d, want 1500", resolved.ProviderShareMinor)
	}
	if resolved.OrganizationShareMinor != 8500 {
		t.Fatalf("organization share=%d, want 8500", resolved.OrganizationShareMinor)
	}
	matched := resolved.Matched[0]
	if matched.BasisMinor != 10000 || matched.RateBps != 1500 || matched.BasisType != CommissionBasisEligibleJobGross {
		t.Fatalf("resolution must record how it calculated: %+v", matched)
	}
	if matched.Settlement != SettlementWork {
		t.Fatalf("commission settles with the job: %+v", matched)
	}
}

func TestCommissionBasisChangesTheAnswer(t *testing.T) {
	// A per-job component first, then a commission on what is left for the
	// organization. Applying the same rate to the gross instead would quietly
	// overpay.
	rules := []CompensationRule{
		activeRule("r_perjob", RulePerJob, RuleConfig{AmountMinor: 2000, Currency: "NGN"}),
		commissionRule("r_comm", 1000, CommissionBasisOrganizationJobShare, RoundingHalfUp),
	}
	resolved, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", at(10))
	if err != nil {
		t.Fatal(err)
	}
	// 2000 per job, then 10% of the remaining 8000 = 800.
	if resolved.ProviderShareMinor != 2800 {
		t.Fatalf("provider share=%d, want 2000 + 10%% of the remaining 8000", resolved.ProviderShareMinor)
	}

	gross := []CompensationRule{
		activeRule("r_perjob", RulePerJob, RuleConfig{AmountMinor: 2000, Currency: "NGN"}),
		commissionRule("r_comm", 1000, CommissionBasisEligibleJobGross, RoundingHalfUp),
	}
	onGross, err := ResolveWorkCompensation(gross, 10000, "NGN", ServiceTaxi, "driver", at(10))
	if err != nil {
		t.Fatal(err)
	}
	if onGross.ProviderShareMinor != 3000 {
		t.Fatalf("gross-basis commission=%d, want 2000 + 10%% of 10000", onGross.ProviderShareMinor)
	}
	if onGross.ProviderShareMinor == resolved.ProviderShareMinor {
		t.Fatal("the declared basis made no difference, so it is not being honoured")
	}
}

func TestUnknownCommissionBasisFailsResolution(t *testing.T) {
	if _, err := CommissionBasisMinor("SOMETHING_ELSE", 10000, 0); err == nil {
		t.Fatal("an unrecognised basis must fail rather than default to one")
	}
	if _, err := CommissionBasisMinor("", 10000, 0); err == nil {
		t.Fatal("a missing basis must fail rather than be guessed")
	}
}

func TestRoundingPolicyIsDeterministicAndExplicit(t *testing.T) {
	// 3333 x 1500bps = 499.95 minor units exactly.
	const product = int64(3333 * 1500)
	cases := map[string]int64{
		RoundingHalfUp: 500,
		RoundingDown:   499,
		RoundingUp:     500,
	}
	for policy, want := range cases {
		if got := ApplyRounding(product, policy); got != want {
			t.Fatalf("%s rounding gave %d, want %d", policy, got, want)
		}
	}
	// 1 x 1bps rounds differently under each policy, which is the point of
	// making the policy explicit.
	if ApplyRounding(1, RoundingDown) != 0 || ApplyRounding(1, RoundingUp) != 1 {
		t.Fatal("down and up must differ on a sub-unit remainder")
	}
	if ApplyRounding(product, "") != 500 {
		t.Fatal("an unset policy must keep the historical half-up behaviour")
	}
	if !ValidRoundingPolicy("") || ValidRoundingPolicy("NEAREST_KOBO") {
		t.Fatal("rounding policy validation is wrong")
	}
}

func TestUnsupportedRoundingPolicyIsRejectedAtValidation(t *testing.T) {
	rule := commissionRule("r_comm", 1500, CommissionBasisEligibleJobGross, "BANKERS")
	if err := ValidateRule(rule); err == nil {
		t.Fatal("an unsupported rounding policy must be refused, not silently defaulted")
	}
}

// -----------------------------------------------------------------------------
// CMP-004 / CMP-005 / FIN-17 / FIN-18 — applicability
// -----------------------------------------------------------------------------

func TestServiceAndRoleApplicabilityAreEnforced(t *testing.T) {
	taxiOnly := commissionRule("r_taxi", 1500, CommissionBasisEligibleJobGross, RoundingHalfUp)
	taxiOnly.ApplicableServices = []string{ServiceTaxi}

	resolved, err := ResolveWorkCompensation([]CompensationRule{taxiOnly}, 10000, "NGN", ServiceDispatch, "rider", at(10))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProviderShareMinor != 0 || len(resolved.Matched) != 0 {
		t.Fatalf("a Taxi-only rule was applied to Dispatch work: %+v", resolved)
	}

	driverOnly := commissionRule("r_driver", 1500, CommissionBasisEligibleJobGross, RoundingHalfUp)
	driverOnly.ApplicableRoles = []string{"driver"}
	resolved, err = ResolveWorkCompensation([]CompensationRule{driverOnly}, 10000, "NGN", ServiceTaxi, "dispatcher", at(10))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProviderShareMinor != 0 {
		t.Fatal("a driver rule was applied to an unrelated role")
	}
}

// -----------------------------------------------------------------------------
// CMP-032 / FIN-15 / FIN-16 — effective dates
// -----------------------------------------------------------------------------

func TestWorkBeforeAndAfterAnEffectiveDateUsesDifferentRules(t *testing.T) {
	changeover := at(15)
	oldRule := commissionRule("r_old", 1000, CommissionBasisEligibleJobGross, RoundingHalfUp)
	oldRule.EffectiveFrom, oldRule.EffectiveTo = at(1), &changeover
	newRule := commissionRule("r_new", 2000, CommissionBasisEligibleJobGross, RoundingHalfUp)
	newRule.EffectiveFrom = changeover
	rules := []CompensationRule{oldRule, newRule}

	before, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", at(10))
	if err != nil {
		t.Fatal(err)
	}
	if before.ProviderShareMinor != 1000 {
		t.Fatalf("work before the change resolved to %d, want the old 10%%", before.ProviderShareMinor)
	}
	after, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", at(20))
	if err != nil {
		t.Fatal(err)
	}
	if after.ProviderShareMinor != 2000 {
		t.Fatalf("work after the change resolved to %d, want the new 20%%", after.ProviderShareMinor)
	}
	// The boundary itself belongs to the new rule, and never to both.
	onBoundary, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", changeover)
	if err != nil {
		t.Fatal(err)
	}
	if onBoundary.ProviderShareMinor != 2000 {
		t.Fatalf("the changeover instant resolved to %d, want a single unambiguous rule", onBoundary.ProviderShareMinor)
	}
}

// -----------------------------------------------------------------------------
// CMP-015 — deterministic multi-component order
// -----------------------------------------------------------------------------

func TestCalculationOrderIsDeterministic(t *testing.T) {
	rules := []CompensationRule{
		commissionRule("r_comm", 1000, CommissionBasisOrganizationJobShare, RoundingHalfUp),
		activeRule("r_perjob", RulePerJob, RuleConfig{AmountMinor: 2000, Currency: "NGN"}),
		activeRule("r_salary", RuleSalary, RuleConfig{AmountMinor: 100, Currency: "NGN", Cadence: CadenceMonthly}),
	}
	first, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", at(10))
	if err != nil {
		t.Fatal(err)
	}
	// Same rules, different input order: the answer must not move.
	shuffled := []CompensationRule{rules[2], rules[0], rules[1]}
	second, err := ResolveWorkCompensation(shuffled, 10000, "NGN", ServiceTaxi, "driver", at(10))
	if err != nil {
		t.Fatal(err)
	}
	if first.ProviderShareMinor != second.ProviderShareMinor {
		t.Fatalf("order changed the money: %d vs %d", first.ProviderShareMinor, second.ProviderShareMinor)
	}
	if first.ProviderShareMinor != 2800 {
		t.Fatalf("provider share=%d, want per-job 2000 then 10%% of the remaining 8000", first.ProviderShareMinor)
	}
}

// -----------------------------------------------------------------------------
// CMP-034 / FIN-26 — currency, and the allocation ceiling
// -----------------------------------------------------------------------------

func TestPerJobInAnotherCurrencyIsRefused(t *testing.T) {
	rules := []CompensationRule{activeRule("r_perjob", RulePerJob, RuleConfig{AmountMinor: 2000, Currency: "USD"})}
	_, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", at(10))
	if err == nil {
		t.Fatal("a USD per-job rate must not be paid against an NGN job")
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProviderShareCannotExceedTheEligibleBasis(t *testing.T) {
	rules := []CompensationRule{activeRule("r_perjob", RulePerJob, RuleConfig{AmountMinor: 20000, Currency: "NGN"})}
	if _, err := ResolveWorkCompensation(rules, 10000, "NGN", ServiceTaxi, "driver", at(10)); err == nil {
		t.Fatal("allocating more than the job carried must be refused, not paid from money that is not there")
	}
}

func TestResolutionAlwaysAccountsForTheWholeBasis(t *testing.T) {
	rules := []CompensationRule{
		commissionRule("r_comm", 1234, CommissionBasisEligibleJobGross, RoundingHalfUp),
		activeRule("r_bonus", RuleBonus, RuleConfig{AmountMinor: 500, Currency: "NGN", Basis: "PER_PERIOD"}),
	}
	for _, basis := range []int64{0, 1, 999, 10000, 123457} {
		resolved, err := ResolveWorkCompensation(rules, basis, "NGN", ServiceTaxi, "driver", at(10))
		if err != nil {
			t.Fatalf("basis %d: %v", basis, err)
		}
		if resolved.ProviderShareMinor+resolved.OrganizationShareMinor != basis {
			t.Fatalf("basis %d: shares sum to %d, so money was dropped or invented",
				basis, resolved.ProviderShareMinor+resolved.OrganizationShareMinor)
		}
	}
}
