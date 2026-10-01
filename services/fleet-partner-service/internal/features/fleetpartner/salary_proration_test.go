package fleetpartner

import (
	"testing"
	"time"
)

func salaryRule(cadence string, amount int64) CompensationRule {
	return CompensationRule{
		ID: "rule_salary", Version: 3, RuleType: RuleSalary, Status: PlanActive,
		Config: RuleConfig{AmountMinor: amount, Currency: "NGN", Cadence: cadence},
	}
}

func day(year int, month time.Month, d int) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}

// A provider who served the whole period is owed the whole salary. Anything
// else means the arithmetic is wrong at its simplest case.
func TestAFullPeriodPaysExactlyTheSalary(t *testing.T) {
	for _, period := range []struct {
		name       string
		start, end time.Time
	}{
		{"31-day January", day(2026, time.January, 1), day(2026, time.January, 31)},
		{"28-day February", day(2026, time.February, 1), day(2026, time.February, 28)},
		{"29-day leap February", day(2024, time.February, 1), day(2024, time.February, 29)},
		{"30-day April", day(2026, time.April, 1), day(2026, time.April, 30)},
	} {
		t.Run(period.name, func(t *testing.T) {
			proration, err := ProrateSalary(salaryRule(CadenceMonthly, 300000), period.start, period.end,
				ServicePeriod{Start: day(2020, time.January, 1)})
			if err != nil {
				t.Fatal(err)
			}
			if proration.AmountMinor != 300000 {
				t.Fatalf("a full %s paid %d, want the whole 300000", period.name, proration.AmountMinor)
			}
		})
	}
}

// CMP-006A. Starting mid-period pays for the days served and no more.
func TestStartingMidPeriodPaysOnlyTheDaysServed(t *testing.T) {
	// April has 30 days. Starting on the 16th serves 15 of them.
	proration, err := ProrateSalary(salaryRule(CadenceMonthly, 300000),
		day(2026, time.April, 1), day(2026, time.April, 30),
		ServicePeriod{Start: day(2026, time.April, 16)})
	if err != nil {
		t.Fatal(err)
	}
	if proration.EligibleDays != 15 {
		t.Fatalf("eligible days = %d, want 15", proration.EligibleDays)
	}
	if proration.AmountMinor != 150000 {
		t.Fatalf("paid %d for half a month, want 150000", proration.AmountMinor)
	}
}

// CMP-006B. Leaving mid-period likewise.
func TestLeavingMidPeriodPaysOnlyTheDaysServed(t *testing.T) {
	proration, err := ProrateSalary(salaryRule(CadenceMonthly, 300000),
		day(2026, time.April, 1), day(2026, time.April, 30),
		ServicePeriod{Start: day(2020, time.January, 1), End: day(2026, time.April, 15)})
	if err != nil {
		t.Fatal(err)
	}
	if proration.EligibleDays != 15 {
		t.Fatalf("eligible days = %d, want 15", proration.EligibleDays)
	}
	if proration.AmountMinor != 150000 {
		t.Fatalf("paid %d, want 150000", proration.AmountMinor)
	}
}

// Both boundaries inside the period.
func TestStartingAndLeavingWithinOnePeriod(t *testing.T) {
	proration, err := ProrateSalary(salaryRule(CadenceMonthly, 300000),
		day(2026, time.April, 1), day(2026, time.April, 30),
		ServicePeriod{Start: day(2026, time.April, 10), End: day(2026, time.April, 19)})
	if err != nil {
		t.Fatal(err)
	}
	if proration.EligibleDays != 10 {
		t.Fatalf("eligible days = %d, want 10", proration.EligibleDays)
	}
	if proration.AmountMinor != 100000 {
		t.Fatalf("paid %d for ten days of thirty, want 100000", proration.AmountMinor)
	}
}

// The first and last days of a period are served days, not boundary cases to
// be dropped. An off-by-one here costs a provider a day's pay every time they
// join or leave.
func TestTheFirstAndLastDayCount(t *testing.T) {
	period := [2]time.Time{day(2026, time.April, 1), day(2026, time.April, 30)}

	firstDayOnly, err := ProrateSalary(salaryRule(CadenceMonthly, 300000), period[0], period[1],
		ServicePeriod{Start: day(2026, time.April, 1), End: day(2026, time.April, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if firstDayOnly.EligibleDays != 1 {
		t.Fatalf("serving only the first day counted %d days", firstDayOnly.EligibleDays)
	}

	lastDayOnly, err := ProrateSalary(salaryRule(CadenceMonthly, 300000), period[0], period[1],
		ServicePeriod{Start: day(2026, time.April, 30)})
	if err != nil {
		t.Fatal(err)
	}
	if lastDayOnly.EligibleDays != 1 {
		t.Fatalf("starting on the last day counted %d days", lastDayOnly.EligibleDays)
	}

	wholePeriod, err := ProrateSalary(salaryRule(CadenceMonthly, 300000), period[0], period[1],
		ServicePeriod{Start: day(2026, time.April, 1), End: day(2026, time.April, 30)})
	if err != nil {
		t.Fatal(err)
	}
	if wholePeriod.EligibleDays != 30 {
		t.Fatalf("serving the whole of April counted %d days, want 30", wholePeriod.EligibleDays)
	}
}

// No eligible service is an answer, not an error. A provider who left before
// the period began is owed nothing, and the figure must say so rather than
// leaving a downstream default to pay them.
func TestNoEligibleServicePaysNothing(t *testing.T) {
	for _, service := range []ServicePeriod{
		{Start: day(2020, time.January, 1), End: day(2026, time.March, 31)},
		{Start: day(2026, time.May, 1)},
	} {
		proration, err := ProrateSalary(salaryRule(CadenceMonthly, 300000),
			day(2026, time.April, 1), day(2026, time.April, 30), service)
		if err != nil {
			t.Fatalf("service %+v: %v", service, err)
		}
		if proration.EligibleDays != 0 || proration.AmountMinor != 0 {
			t.Fatalf("service %+v paid %d over %d days, want nothing",
				service, proration.AmountMinor, proration.EligibleDays)
		}
	}
}

// A daily salary multiplies and never rounds, so it cannot drift.
func TestADailySalaryIsExact(t *testing.T) {
	proration, err := ProrateSalary(salaryRule(CadenceDaily, 3333),
		day(2026, time.April, 1), day(2026, time.April, 30),
		ServicePeriod{Start: day(2026, time.April, 8)})
	if err != nil {
		t.Fatal(err)
	}
	if proration.EligibleDays != 23 {
		t.Fatalf("eligible days = %d, want 23", proration.EligibleDays)
	}
	if proration.AmountMinor != 3333*23 {
		t.Fatalf("paid %d, want %d", proration.AmountMinor, 3333*23)
	}
	if proration.RemainderMinor != 0 || proration.RoundedUp {
		t.Fatalf("a daily salary rounded: %+v", proration)
	}
}

func TestAWeeklySalaryDividesBySeven(t *testing.T) {
	proration, err := ProrateSalary(salaryRule(CadenceWeekly, 70000),
		day(2026, time.April, 1), day(2026, time.April, 7),
		ServicePeriod{Start: day(2020, time.January, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if proration.AmountMinor != 70000 {
		t.Fatalf("a full week paid %d, want 70000", proration.AmountMinor)
	}

	partial, err := ProrateSalary(salaryRule(CadenceWeekly, 70000),
		day(2026, time.April, 1), day(2026, time.April, 7),
		ServicePeriod{Start: day(2026, time.April, 5)})
	if err != nil {
		t.Fatal(err)
	}
	if partial.EligibleDays != 3 || partial.AmountMinor != 30000 {
		t.Fatalf("three days of a week paid %d over %d days, want 30000/3",
			partial.AmountMinor, partial.EligibleDays)
	}
}

// Requirement: remainders must be deterministic and auditable. The provenance
// has to be enough to reproduce the figure by hand.
func TestTheRemainderIsRecordedAndTheRoundingIsVisible(t *testing.T) {
	// 100000 over 30 days, 7 days served: 700000/30 = 23333 remainder 10.
	proration, err := ProrateSalary(salaryRule(CadenceMonthly, 100000),
		day(2026, time.April, 1), day(2026, time.April, 30),
		ServicePeriod{Start: day(2026, time.April, 24)})
	if err != nil {
		t.Fatal(err)
	}
	if proration.EligibleDays != 7 || proration.DivisorDays != 30 {
		t.Fatalf("proration = %+v, want 7 of 30 days", proration)
	}
	// The recorded working must reconstruct the paid figure exactly.
	base := proration.SalaryMinor * proration.EligibleDays / proration.DivisorDays
	if proration.RoundedUp {
		base++
	}
	if base != proration.AmountMinor {
		t.Fatalf("the recorded working gives %d but %d was paid", base, proration.AmountMinor)
	}
	if proration.RemainderMinor == 0 {
		t.Fatal("a division that does not come out even recorded no remainder")
	}
	if proration.RuleID == "" || proration.RuleVersion == 0 {
		t.Fatalf("proration carries no rule provenance: %+v", proration)
	}
}

// Deterministic: the same inputs pay the same figure, every time.
func TestProrationIsDeterministic(t *testing.T) {
	first, err := ProrateSalary(salaryRule(CadenceMonthly, 123457),
		day(2026, time.April, 1), day(2026, time.April, 30),
		ServicePeriod{Start: day(2026, time.April, 11)})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 50; attempt++ {
		again, err := ProrateSalary(salaryRule(CadenceMonthly, 123457),
			day(2026, time.April, 1), day(2026, time.April, 30),
			ServicePeriod{Start: day(2026, time.April, 11)})
		if err != nil {
			t.Fatal(err)
		}
		if again.AmountMinor != first.AmountMinor {
			t.Fatalf("attempt %d paid %d, first paid %d", attempt, again.AmountMinor, first.AmountMinor)
		}
	}
}

// A monthly salary prorated against a fortnight would pay a month's salary for
// two weeks; against a quarter it would pay a third of what is owed. Both are
// payroll errors nobody would notice, so the combination is refused.
func TestAMonthlySalaryNeedsAMonthlyPeriod(t *testing.T) {
	for _, period := range [][2]time.Time{
		{day(2026, time.April, 1), day(2026, time.April, 14)},
		{day(2026, time.April, 1), day(2026, time.June, 30)},
	} {
		if _, err := ProrateSalary(salaryRule(CadenceMonthly, 300000), period[0], period[1],
			ServicePeriod{Start: day(2020, time.January, 1)}); err == nil {
			t.Fatalf("a monthly salary was prorated against %v..%v", period[0], period[1])
		}
	}
}

// Clock time within a day must not cost anybody a day's pay.
func TestTimeOfDayDoesNotChangeTheDayCount(t *testing.T) {
	midMorning := time.Date(2026, time.April, 16, 9, 30, 0, 0, time.UTC)
	proration, err := ProrateSalary(salaryRule(CadenceMonthly, 300000),
		day(2026, time.April, 1), day(2026, time.April, 30),
		ServicePeriod{Start: midMorning})
	if err != nil {
		t.Fatal(err)
	}
	if proration.EligibleDays != 15 {
		t.Fatalf("starting at 09:30 counted %d days, want 15", proration.EligibleDays)
	}
}

func TestANonSalaryRuleIsNotProrated(t *testing.T) {
	rule := CompensationRule{
		ID: "r", Version: 1, RuleType: RulePerJob, Status: PlanActive,
		Config: RuleConfig{AmountMinor: 100, Currency: "NGN"},
	}
	if _, err := ProrateSalary(rule, day(2026, time.April, 1), day(2026, time.April, 30), ServicePeriod{}); err == nil {
		t.Fatal("a per-job rule was prorated as a salary")
	}
}

func TestAnInvalidPeriodIsRefused(t *testing.T) {
	for _, period := range [][2]time.Time{
		{day(2026, time.April, 30), day(2026, time.April, 1)},
		{{}, day(2026, time.April, 30)},
	} {
		if _, err := ProrateSalary(salaryRule(CadenceMonthly, 300000), period[0], period[1],
			ServicePeriod{Start: day(2020, time.January, 1)}); err == nil {
			t.Fatalf("an invalid period %v..%v was accepted", period[0], period[1])
		}
	}
	local := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.FixedZone("WAT", 3600))
	if _, err := ProrateSalary(salaryRule(CadenceMonthly, 300000), local, day(2026, time.April, 30),
		ServicePeriod{Start: day(2020, time.January, 1)}); err == nil {
		t.Fatal("a non-UTC payroll period was accepted")
	}
}
