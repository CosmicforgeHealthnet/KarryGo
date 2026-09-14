package fleetpartner

import (
	"testing"
	"time"
)

func salaryPlanRule(id string) CompensationRule {
	return activeRule(id, RuleSalary, RuleConfig{
		AmountMinor: 300000, Currency: "NGN", Cadence: CadenceMonthly,
	})
}

// CMP-021. Provider plus organization is the job, exactly, with nowhere for a
// unit to go or come from.
func TestProviderAndOrganizationAccountForTheWholeJob(t *testing.T) {
	for _, basis := range []int64{0, 1, 9999, 10000, 123457} {
		resolution, err := ResolveWorkCompensation(
			[]CompensationRule{commissionRule("r_comm", 1500, CommissionBasisEligibleJobGross, RoundingHalfUp)},
			basis, "NGN", "TAXI", "driver", day(2026, time.April, 10))
		if err != nil {
			t.Fatalf("basis %d: %v", basis, err)
		}
		conservation, err := CheckEconomicConservation(resolution, basis)
		if err != nil {
			t.Fatalf("basis %d: %v", basis, err)
		}
		if !conservation.Balanced() {
			t.Fatalf("basis %d did not balance: %+v", basis, conservation)
		}
	}
}

// A salaried plan pays nothing per job and the whole job is the
// organization's. That is the correct answer, not a missing one.
func TestASalariedPlanGivesTheWholeJobToTheOrganization(t *testing.T) {
	resolution, err := ResolveWorkCompensation(
		[]CompensationRule{salaryPlanRule("r_sal")},
		10000, "NGN", "TAXI", "driver", day(2026, time.April, 10))
	if err != nil {
		t.Fatal(err)
	}
	conservation, err := CheckEconomicConservation(resolution, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if conservation.ProviderShareMinor != 0 || conservation.OrganizationShareMinor != 10000 {
		t.Fatalf("conservation = %+v, want 0 to the provider and 10000 to the organization", conservation)
	}
}

// The payroll-accrued total is reported but deliberately excluded from the
// job's arithmetic: it is paid from the organization's own money later, not
// out of this job.
func TestPayrollAccrualIsReportedButNotChargedToTheJob(t *testing.T) {
	accrued := commissionRule("r_accrued", 1500, CommissionBasisEligibleJobGross, RoundingHalfUp)
	accrued.Config.SettlementStrategy = StrategyPayrollAccrual

	resolution, err := ResolveWorkCompensation(
		[]CompensationRule{accrued}, 10000, "NGN", "TAXI", "driver", day(2026, time.April, 10))
	if err != nil {
		t.Fatal(err)
	}
	conservation, err := CheckEconomicConservation(resolution, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if conservation.ProviderShareMinor != 0 {
		t.Fatalf("an accrued component was paid with the job: %+v", conservation)
	}
	if conservation.OrganizationShareMinor != 10000 {
		t.Fatalf("organization share = %d, want the whole 10000", conservation.OrganizationShareMinor)
	}
	if !conservation.Balanced() {
		t.Fatalf("conservation did not balance: %+v", conservation)
	}
}

// The overlap check: one rule settled in two places would be paid with the job
// AND accrued to payroll, and both figures would look correct in isolation.
func TestARuleSettledInTwoPlacesIsRefused(t *testing.T) {
	resolution := WorkResolution{
		ProviderShareMinor: 1500, OrganizationShareMinor: 8500,
		Matched: []MatchedRule{
			{RuleID: "r_comm", RuleType: RuleCommission, Settlement: SettlementWork, AmountMinor: 1500},
			{RuleID: "r_comm", RuleType: RuleCommission, Settlement: SettlementPayroll, AmountMinor: 1500},
		},
	}
	if _, err := CheckEconomicConservation(resolution, 10000); err == nil {
		t.Fatal("one rule settled at both levels was accepted")
	}
}

// A resolution whose parts do not add up to the job is refused rather than
// quietly paid.
func TestAResolutionThatDoesNotAddUpIsRefused(t *testing.T) {
	for _, resolution := range []WorkResolution{
		{ProviderShareMinor: 1500, OrganizationShareMinor: 8000,
			Matched: []MatchedRule{{RuleID: "r", Settlement: SettlementWork, AmountMinor: 1500}}},
		{ProviderShareMinor: 1500, OrganizationShareMinor: 9000,
			Matched: []MatchedRule{{RuleID: "r", Settlement: SettlementWork, AmountMinor: 1500}}},
		{ProviderShareMinor: 1500, OrganizationShareMinor: 8500,
			Matched: []MatchedRule{{RuleID: "r", Settlement: SettlementWork, AmountMinor: 900}}},
	} {
		if _, err := CheckEconomicConservation(resolution, 10000); err == nil {
			t.Fatalf("an unbalanced resolution was accepted: %+v", resolution)
		}
	}
}

// CMP-033. Identity must not split by spelling: a provider recorded as "taxi"
// in one place and "TAXI" in another is two people to payroll, and is paid
// twice.
func TestServiceContextIsCanonical(t *testing.T) {
	for _, spelling := range []string{"TAXI", "taxi", " Taxi ", "driver-taxi-service", "TAXI_GOODS"} {
		canonical, err := CanonicalServiceContext(spelling)
		if err != nil {
			t.Fatalf("%q: %v", spelling, err)
		}
		if canonical != ServiceContextTaxi {
			t.Fatalf("%q resolved to %q, want TAXI", spelling, canonical)
		}
	}
	if _, err := CanonicalServiceContext("SOMETHING_ELSE"); err == nil {
		t.Fatal("an unrecognised service context was accepted")
	}
}

// The most expensive possible bug: one salary paid once per service. It looks
// entirely normal on each service's own report.
func TestOneSalaryIsPaidOncePerProviderNotOncePerService(t *testing.T) {
	_, err := ResolveMultiServicePayroll("mem_1", "NGN", []PeriodComponent{
		{MembershipID: "mem_1", ServiceContext: "TAXI", Component: RuleSalary, RuleID: "r_sal", AmountMinor: 300000, Currency: "NGN"},
		{MembershipID: "mem_1", ServiceContext: "HAULING", Component: RuleSalary, RuleID: "r_sal", AmountMinor: 300000, Currency: "NGN"},
	})
	if err == nil {
		t.Fatal("one salary rule was paid under two services")
	}
}

func TestAFixedAllowanceIsAlsoPaidOncePerProvider(t *testing.T) {
	_, err := ResolveMultiServicePayroll("mem_1", "NGN", []PeriodComponent{
		{MembershipID: "mem_1", ServiceContext: "TAXI", Component: RuleAllowance, RuleID: "r_allow", AmountMinor: 5000, Currency: "NGN"},
		{MembershipID: "mem_1", ServiceContext: "DISPATCH", Component: RuleAllowance, RuleID: "r_allow", AmountMinor: 5000, Currency: "NGN"},
	})
	if err == nil {
		t.Fatal("one allowance rule was paid under two services")
	}
}

// Per-unit components are genuinely per service and must accumulate: a
// provider who drove a taxi and a truck earned both.
func TestPerServiceEarningsAccumulateAcrossServices(t *testing.T) {
	payroll, err := ResolveMultiServicePayroll("mem_1", "NGN", []PeriodComponent{
		{MembershipID: "mem_1", ServiceContext: "TAXI", Component: RulePerJob, RuleID: "r_taxi", AmountMinor: 4000, Currency: "NGN"},
		{MembershipID: "mem_1", ServiceContext: "HAULING", Component: RulePerJob, RuleID: "r_haul", AmountMinor: 9000, Currency: "NGN"},
		{MembershipID: "mem_1", ServiceContext: "DISPATCH", Component: RuleCommission, RuleID: "r_disp", AmountMinor: 2500, Currency: "NGN"},
		{MembershipID: "mem_1", ServiceContext: "TAXI", Component: RuleSalary, RuleID: "r_sal", AmountMinor: 300000, Currency: "NGN"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if payroll.TotalMinor != 315500 {
		t.Fatalf("total = %d, want 4000+9000+2500+300000", payroll.TotalMinor)
	}
	if payroll.ByService[ServiceContextTaxi] != 304000 {
		t.Fatalf("taxi total = %d, want 4000+300000", payroll.ByService[ServiceContextTaxi])
	}
	if payroll.ByService[ServiceContextHauling] != 9000 || payroll.ByService[ServiceContextDispatch] != 2500 {
		t.Fatalf("per-service totals wrong: %+v", payroll.ByService)
	}
}

// Nothing from another provider may land on this payslip.
func TestAnotherProvidersComponentCannotBePaidHere(t *testing.T) {
	_, err := ResolveMultiServicePayroll("mem_1", "NGN", []PeriodComponent{
		{MembershipID: "mem_2", ServiceContext: "TAXI", Component: RulePerJob, RuleID: "r", AmountMinor: 4000, Currency: "NGN"},
	})
	if err == nil {
		t.Fatal("another provider's component was paid on this payroll")
	}
}

// Combining currencies is meaningless, so it is refused rather than summed.
func TestComponentsInAnotherCurrencyAreRefused(t *testing.T) {
	_, err := ResolveMultiServicePayroll("mem_1", "NGN", []PeriodComponent{
		{MembershipID: "mem_1", ServiceContext: "TAXI", Component: RulePerJob, RuleID: "r", AmountMinor: 4000, Currency: "USD"},
	})
	if err == nil {
		t.Fatal("a USD component was added to an NGN payroll")
	}
}

// A provider who worked nothing is paid nothing, and that is a valid payroll
// rather than an error.
func TestAProviderWithNoComponentsHasAnEmptyPayroll(t *testing.T) {
	payroll, err := ResolveMultiServicePayroll("mem_1", "NGN", nil)
	if err != nil {
		t.Fatal(err)
	}
	if payroll.TotalMinor != 0 || len(payroll.Components) != 0 {
		t.Fatalf("payroll = %+v, want empty", payroll)
	}
}

// The combined payroll must be stable: the same components in a different
// order produce the same result.
func TestMultiServicePayrollIsOrderIndependent(t *testing.T) {
	forward := []PeriodComponent{
		{MembershipID: "mem_1", ServiceContext: "TAXI", Component: RulePerJob, RuleID: "r_a", AmountMinor: 4000, Currency: "NGN"},
		{MembershipID: "mem_1", ServiceContext: "HAULING", Component: RulePerJob, RuleID: "r_b", AmountMinor: 9000, Currency: "NGN"},
	}
	reversed := []PeriodComponent{forward[1], forward[0]}

	first, err := ResolveMultiServicePayroll("mem_1", "NGN", forward)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveMultiServicePayroll("mem_1", "NGN", reversed)
	if err != nil {
		t.Fatal(err)
	}
	if first.TotalMinor != second.TotalMinor {
		t.Fatalf("order changed the total: %d vs %d", first.TotalMinor, second.TotalMinor)
	}
	for index := range first.Components {
		if first.Components[index] != second.Components[index] {
			t.Fatalf("order changed component %d: %+v vs %+v",
				index, first.Components[index], second.Components[index])
		}
	}
}
