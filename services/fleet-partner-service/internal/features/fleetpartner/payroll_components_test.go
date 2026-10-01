package fleetpartner

import (
	"testing"
	"time"
)

var (
	testPeriodStart = day(2026, time.April, 1)
	testPeriodEnd   = day(2026, time.April, 30)
)

func allowanceRule(basis string, amount int64, rateBps int) CompensationRule {
	return CompensationRule{
		ID: "rule_allowance", Version: 2, RuleType: RuleAllowance, Status: PlanActive,
		Config: RuleConfig{AmountMinor: amount, Currency: "NGN", Basis: basis, RateBps: rateBps},
	}
}

// CMP-011. Every basis computes from the quantity it names, and nothing else.
func TestEachAllowanceBasisComputesFromItsOwnQuantity(t *testing.T) {
	inputs := AllowanceInputs{EligibleSalaryMinor: 200000, Shifts: 12, Days: 20, Jobs: 47}

	for _, scenario := range []struct {
		basis    string
		amount   int64
		rateBps  int
		want     int64
		wantUnit string
	}{
		{AllowanceBasisFixed, 5000, 0, 5000, "PERIOD"},
		{AllowanceBasisPerShift, 750, 0, 750 * 12, "SHIFT"},
		{AllowanceBasisPerDay, 300, 0, 300 * 20, "DAY"},
		{AllowanceBasisPerJob, 100, 0, 100 * 47, "JOB"},
		{AllowanceBasisEligibleSalary, 0, 1000, 20000, "MINOR"},
	} {
		t.Run(scenario.basis, func(t *testing.T) {
			provenance, err := CalculateAllowance(
				allowanceRule(scenario.basis, scenario.amount, scenario.rateBps),
				testPeriodStart, testPeriodEnd, inputs)
			if err != nil {
				t.Fatal(err)
			}
			if provenance.AmountMinor != scenario.want {
				t.Fatalf("%s paid %d, want %d", scenario.basis, provenance.AmountMinor, scenario.want)
			}
			if provenance.BasisUnit != scenario.wantUnit {
				t.Fatalf("%s recorded unit %q, want %q", scenario.basis, provenance.BasisUnit, scenario.wantUnit)
			}
			if provenance.Basis != scenario.basis || provenance.RuleID == "" || provenance.RuleVersion == 0 {
				t.Fatalf("%s carries no provenance: %+v", scenario.basis, provenance)
			}
		})
	}
}

// An eligible-salary allowance is based on what was actually earned, so a
// provider who served half a period does not receive a full period's
// allowance.
func TestAnEligibleSalaryAllowanceFollowsTheProratedSalary(t *testing.T) {
	full, err := CalculateAllowance(allowanceRule(AllowanceBasisEligibleSalary, 0, 1000),
		testPeriodStart, testPeriodEnd, AllowanceInputs{EligibleSalaryMinor: 300000})
	if err != nil {
		t.Fatal(err)
	}
	half, err := CalculateAllowance(allowanceRule(AllowanceBasisEligibleSalary, 0, 1000),
		testPeriodStart, testPeriodEnd, AllowanceInputs{EligibleSalaryMinor: 150000})
	if err != nil {
		t.Fatal(err)
	}
	if half.AmountMinor*2 != full.AmountMinor {
		t.Fatalf("half a period's salary produced %d against a full period's %d",
			half.AmountMinor, full.AmountMinor)
	}
}

// Fails closed. A typo that quietly became a flat payment would be
// indistinguishable from an intended one.
func TestAnUnsupportedAllowanceBasisIsRefused(t *testing.T) {
	for _, basis := range []string{"", "WHATEVER", "PER_KILOMETRE", "eligible-salary"} {
		if _, err := CalculateAllowance(allowanceRule(basis, 5000, 0),
			testPeriodStart, testPeriodEnd, AllowanceInputs{}); err == nil {
			t.Fatalf("allowance basis %q was accepted", basis)
		}
	}
}

func TestANonAllowanceRuleIsNotAnAllowance(t *testing.T) {
	rule := allowanceRule(AllowanceBasisFixed, 5000, 0)
	rule.RuleType = RuleBonus
	if _, err := CalculateAllowance(rule, testPeriodStart, testPeriodEnd, AllowanceInputs{}); err == nil {
		t.Fatal("a bonus rule was computed as an allowance")
	}
}

func overtimeRule(rateMinor int64) CompensationRule {
	return CompensationRule{
		ID: "rule_overtime", Version: 4, RuleType: RuleOvertime, Status: PlanActive,
		Config: RuleConfig{AmountMinor: rateMinor, Currency: "NGN", Basis: "APPROVED_OVERTIME"},
	}
}

func approvedOvertime() ApprovedOvertimeRecord {
	return ApprovedOvertimeRecord{
		ID: "ot_1", MembershipID: "mem_1", OrganizationID: "org_1",
		ApprovedUnits: 6, Unit: "HOUR",
		ApprovedBy: "usr_supervisor", ApprovedAt: day(2026, time.April, 28),
		PeriodStart: testPeriodStart, PeriodEnd: testPeriodEnd,
	}
}

// CMP-012. Overtime is paid from the approval and carries it.
func TestOvertimeIsPaidFromItsApproval(t *testing.T) {
	provenance, err := CalculateOvertime(overtimeRule(1200), testPeriodStart, testPeriodEnd, approvedOvertime())
	if err != nil {
		t.Fatal(err)
	}
	if provenance.AmountMinor != 7200 {
		t.Fatalf("paid %d for six approved hours at 1200, want 7200", provenance.AmountMinor)
	}
	if provenance.AuthorizationID != "ot_1" || provenance.AuthorizedBy != "usr_supervisor" {
		t.Fatalf("overtime lost its approval: %+v", provenance)
	}
	if provenance.BasisQuantity != 6 || provenance.BasisUnit != "HOUR" {
		t.Fatalf("overtime recorded %d %s, want 6 HOUR", provenance.BasisQuantity, provenance.BasisUnit)
	}
}

// The whole point of CMP-012: no approval, no payment. An unapproved record is
// not paid at zero, it is refused -- a silent zero is how an overtime bug
// survives a whole quarter.
func TestOvertimeWithoutAValidApprovalIsRefused(t *testing.T) {
	for _, mutate := range []struct {
		name  string
		apply func(*ApprovedOvertimeRecord)
	}{
		{"no record id", func(r *ApprovedOvertimeRecord) { r.ID = "" }},
		{"no approver", func(r *ApprovedOvertimeRecord) { r.ApprovedBy = "" }},
		{"no approval time", func(r *ApprovedOvertimeRecord) { r.ApprovedAt = time.Time{} }},
		{"no approved quantity", func(r *ApprovedOvertimeRecord) { r.ApprovedUnits = 0 }},
		{"negative quantity", func(r *ApprovedOvertimeRecord) { r.ApprovedUnits = -6 }},
		{"no unit", func(r *ApprovedOvertimeRecord) { r.Unit = "" }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			record := approvedOvertime()
			mutate.apply(&record)
			if _, err := CalculateOvertime(overtimeRule(1200), testPeriodStart, testPeriodEnd, record); err == nil {
				t.Fatalf("overtime with %s was paid", mutate.name)
			}
		})
	}
}

// An approval belongs to the period it was approved for. Reusing it in a later
// period pays the same work twice, once in each.
func TestAnOvertimeApprovalCannotBeReusedInAnotherPeriod(t *testing.T) {
	record := approvedOvertime()
	nextStart, nextEnd := day(2026, time.May, 1), day(2026, time.May, 31)
	if _, err := CalculateOvertime(overtimeRule(1200), nextStart, nextEnd, record); err == nil {
		t.Fatal("April's overtime approval was paid again in May")
	}
}

func bonusAuthorization() BonusAuthorization {
	return BonusAuthorization{
		ID: "bon_1", Source: "PERFORMANCE_REVIEW", Reason: "Q1 service quality",
		AuthorizedBy: "usr_manager", AuthorizedAt: day(2026, time.April, 20),
		AmountMinor: 50000, Currency: "NGN",
		PeriodStart: testPeriodStart, PeriodEnd: testPeriodEnd,
	}
}

// CMP-013. A bonus carries where it came from, why, and who said so.
func TestABonusCarriesItsProvenance(t *testing.T) {
	provenance, err := CalculateBonus(testPeriodStart, testPeriodEnd, bonusAuthorization())
	if err != nil {
		t.Fatal(err)
	}
	if provenance.AmountMinor != 50000 {
		t.Fatalf("bonus paid %d, want 50000", provenance.AmountMinor)
	}
	for field, value := range map[string]string{
		"authorization": provenance.AuthorizationID,
		"authorizer":    provenance.AuthorizedBy,
		"reason":        provenance.Reason,
		"source":        provenance.Basis,
	} {
		if value == "" {
			t.Errorf("bonus carries no %s: %+v", field, provenance)
		}
	}
}

// A bonus with no valid provenance is refused, not quietly entered at zero.
func TestABonusWithoutProvenanceDoesNotEnterPayroll(t *testing.T) {
	for _, mutate := range []struct {
		name  string
		apply func(*BonusAuthorization)
	}{
		{"no authorization id", func(a *BonusAuthorization) { a.ID = "" }},
		{"no source", func(a *BonusAuthorization) { a.Source = "" }},
		{"no reason", func(a *BonusAuthorization) { a.Reason = "" }},
		{"no authorizer", func(a *BonusAuthorization) { a.AuthorizedBy = "" }},
		{"no authorization time", func(a *BonusAuthorization) { a.AuthorizedAt = time.Time{} }},
		{"no amount", func(a *BonusAuthorization) { a.AmountMinor = 0 }},
		{"negative amount", func(a *BonusAuthorization) { a.AmountMinor = -50000 }},
		{"no currency", func(a *BonusAuthorization) { a.Currency = "" }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			authorization := bonusAuthorization()
			mutate.apply(&authorization)
			if _, err := CalculateBonus(testPeriodStart, testPeriodEnd, authorization); err == nil {
				t.Fatalf("a bonus with %s entered payroll", mutate.name)
			}
		})
	}
}

func TestABonusCannotBePaidInAnotherPeriod(t *testing.T) {
	if _, err := CalculateBonus(day(2026, time.May, 1), day(2026, time.May, 31), bonusAuthorization()); err == nil {
		t.Fatal("April's bonus was paid again in May")
	}
}

func deductionAuthorization() DeductionAuthorization {
	return DeductionAuthorization{
		ID: "ded_1", DeductionType: "EQUIPMENT_DAMAGE", Source: "INCIDENT_REPORT",
		Reason: "Replacement helmet", AuthorizedBy: "usr_manager",
		AuthorizedAt: day(2026, time.April, 18),
		AmountMinor:  10000, Currency: "NGN",
		PeriodStart: testPeriodStart, PeriodEnd: testPeriodEnd,
		LiabilityReference: "inc_77", OutstandingBeforeMinor: 40000,
	}
}

// CMP-014. A deduction explains itself, including what is left to pay.
func TestADeductionIsFullyAuditable(t *testing.T) {
	line, err := CalculateDeduction(testPeriodStart, testPeriodEnd, deductionAuthorization())
	if err != nil {
		t.Fatal(err)
	}
	if line.AmountMinor != 10000 {
		t.Fatalf("deducted %d, want 10000", line.AmountMinor)
	}
	if line.DeductionType != "EQUIPMENT_DAMAGE" || line.Reason == "" || line.AuthorizedBy == "" {
		t.Fatalf("deduction is not explainable: %+v", line)
	}
	if line.OutstandingBeforeMinor != 40000 || line.OutstandingAfterMinor != 30000 {
		t.Fatalf("liability went from %d to %d, want 40000 to 30000",
			line.OutstandingBeforeMinor, line.OutstandingAfterMinor)
	}
}

// Never an opaque negative line.
func TestADeductionWithoutFullAuthorityIsRefused(t *testing.T) {
	for _, mutate := range []struct {
		name  string
		apply func(*DeductionAuthorization)
	}{
		{"no authorization id", func(a *DeductionAuthorization) { a.ID = "" }},
		{"no deduction type", func(a *DeductionAuthorization) { a.DeductionType = "" }},
		{"no source", func(a *DeductionAuthorization) { a.Source = "" }},
		{"no reason", func(a *DeductionAuthorization) { a.Reason = "" }},
		{"no authorizer", func(a *DeductionAuthorization) { a.AuthorizedBy = "" }},
		{"no authorization time", func(a *DeductionAuthorization) { a.AuthorizedAt = time.Time{} }},
		{"no amount", func(a *DeductionAuthorization) { a.AmountMinor = 0 }},
		{"negative amount", func(a *DeductionAuthorization) { a.AmountMinor = -10000 }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			authorization := deductionAuthorization()
			mutate.apply(&authorization)
			if _, err := CalculateDeduction(testPeriodStart, testPeriodEnd, authorization); err == nil {
				t.Fatalf("a deduction with %s was applied", mutate.name)
			}
		})
	}
}

// Recovering more than is owed is not a deduction, it is a charge.
func TestADeductionCannotExceedWhatIsOwed(t *testing.T) {
	authorization := deductionAuthorization()
	authorization.AmountMinor = 40001
	if _, err := CalculateDeduction(testPeriodStart, testPeriodEnd, authorization); err == nil {
		t.Fatal("a deduction took back more than was owed")
	}
}

// Repeated instalments recover the liability and stop.
func TestInstalmentsRecoverALiabilityAndNoMore(t *testing.T) {
	outstanding := int64(40000)
	for instalment := 0; instalment < 4; instalment++ {
		authorization := deductionAuthorization()
		authorization.ID = "ded_" + string(rune('a'+instalment))
		authorization.OutstandingBeforeMinor = outstanding
		line, err := CalculateDeduction(testPeriodStart, testPeriodEnd, authorization)
		if err != nil {
			t.Fatalf("instalment %d: %v", instalment, err)
		}
		outstanding = line.OutstandingAfterMinor
	}
	if outstanding != 0 {
		t.Fatalf("four 10000 instalments left %d of 40000 owed", outstanding)
	}
	exhausted := deductionAuthorization()
	exhausted.OutstandingBeforeMinor = 0
	if _, err := CalculateDeduction(testPeriodStart, testPeriodEnd, exhausted); err == nil {
		t.Fatal("a fifth instalment was taken against a settled liability")
	}
}

func TestADeductionCannotBeAppliedInAnotherPeriod(t *testing.T) {
	if _, err := CalculateDeduction(day(2026, time.May, 1), day(2026, time.May, 31),
		deductionAuthorization()); err == nil {
		t.Fatal("April's deduction was applied again in May")
	}
}
