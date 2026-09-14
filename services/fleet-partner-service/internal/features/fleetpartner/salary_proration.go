package fleetpartner

// CMP-006 / 006A / 006B: paying a salary for part of a period.
//
// A provider who joins on the 12th is owed part of the month, not all of it and
// not none of it. Both wrong answers are silent: nobody notices an overpayment
// on one payslip, and the underpaid provider has no way to check the figure
// unless the service can show its working.
//
// So proration is deterministic, integer-only, and carries enough provenance to
// reconstruct the result: the original salary, the period, the days actually
// served, the divisor, and the remainder that was dropped or carried.

import (
	"fmt"
	"time"
)

// The canonical basis is the whole UTC calendar day.
//
// Days rather than hours or shifts because a salary is not paid for hours
// worked -- that is what overtime and per-shift rules are for -- and because a
// day is the only unit every service and every calendar agrees on.
const (
	// CadenceDailyDivisor is unused: a daily salary multiplies by the days
	// served rather than dividing, so it is exact and never rounds.
	weeklySalaryDivisorDays = 7

	// A monthly rule is prorated against the payroll period's own length,
	// which is what makes month-length differences come out right: a full
	// February pays exactly the monthly salary, as does a full March.
	minimumMonthlyPeriodDays = 28
	maximumMonthlyPeriodDays = 31
)

// SalaryProration is a prorated salary and the working behind it.
//
// Every field is here so the figure can be checked by hand. A provider
// disputing their pay is entitled to more than the answer.
type SalaryProration struct {
	SalaryMinor int64  `json:"salaryMinor"`
	Cadence     string `json:"cadence"`
	Currency    string `json:"currency"`

	PeriodStart time.Time `json:"periodStart"`
	PeriodEnd   time.Time `json:"periodEnd"`

	// PeriodDays is the payroll period's length in whole days, inclusive of
	// both ends.
	PeriodDays int64 `json:"periodDays"`

	// EligibleDays is how many of those days the provider was in service.
	EligibleDays int64 `json:"eligibleDays"`

	// DivisorDays is what EligibleDays was divided by. Zero for a daily
	// cadence, which multiplies instead.
	DivisorDays int64 `json:"divisorDays"`

	// RemainderMinor is what the division left over, before rounding. Carried
	// so an auditor can see whether the provider was rounded up or down and by
	// how much.
	RemainderMinor int64 `json:"remainderMinor"`

	// RoundedUp records which way the remainder went.
	RoundedUp bool `json:"roundedUp"`

	AmountMinor int64 `json:"amountMinor"`

	// RuleVersion ties the figure to the plan version it was computed under,
	// so a later plan change cannot silently restate a historical payslip.
	RuleID      string `json:"ruleId"`
	RuleVersion int64  `json:"ruleVersion"`
}

// ServicePeriod is when a provider was actually in service.
//
// A zero End means still in service, which is the common case and must not be
// confused with having left on the zero date.
type ServicePeriod struct {
	Start time.Time
	End   time.Time
}

// wholeDaysBetween counts calendar days inclusive of both ends.
//
// Both instants are truncated to their UTC date first, so a period that starts
// at 09:00 and ends at 17:00 is still counted in whole days rather than
// silently losing one to the clock.
func wholeDaysBetween(from, to time.Time) int64 {
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	if end.Before(start) {
		return 0
	}
	return int64(end.Sub(start).Hours()/24) + 1
}

// EligibleSalaryDays is how many days of a payroll period a provider served.
//
// It is the overlap of the two ranges and nothing cleverer. A provider who
// started before the period and is still in service serves all of it; one who
// started after it ended serves none.
func EligibleSalaryDays(periodStart, periodEnd time.Time, service ServicePeriod) int64 {
	start := periodStart
	if !service.Start.IsZero() && service.Start.After(start) {
		start = service.Start
	}
	end := periodEnd
	if !service.End.IsZero() && service.End.Before(end) {
		end = service.End
	}
	if end.Before(start) {
		return 0
	}
	return wholeDaysBetween(start, end)
}

// ProrateSalary computes what a salary rule owes for one payroll period.
//
// Daily multiplies, weekly and monthly divide. The divisor differs on purpose:
// a weekly salary covers seven days wherever the period boundaries fall, while
// a monthly salary covers the payroll period itself -- which is what makes a
// full February pay exactly the same monthly salary as a full March, rather
// than three days less.
func ProrateSalary(rule CompensationRule, periodStart, periodEnd time.Time, service ServicePeriod) (SalaryProration, error) {
	if rule.RuleType != RuleSalary {
		return SalaryProration{}, fmt.Errorf("%w: %s is not a salary rule", ErrValidation, rule.RuleType)
	}
	if err := ValidateRule(rule); err != nil {
		return SalaryProration{}, err
	}
	if periodStart.IsZero() || periodEnd.IsZero() || periodEnd.Before(periodStart) {
		return SalaryProration{}, fmt.Errorf("%w: payroll period is invalid", ErrValidation)
	}
	if periodStart.Location() != time.UTC || periodEnd.Location() != time.UTC {
		return SalaryProration{}, fmt.Errorf("%w: payroll period must be UTC", ErrValidation)
	}

	proration := SalaryProration{
		SalaryMinor: rule.Config.AmountMinor, Cadence: rule.Config.Cadence,
		Currency: rule.Config.Currency, PeriodStart: periodStart, PeriodEnd: periodEnd,
		PeriodDays:   wholeDaysBetween(periodStart, periodEnd),
		EligibleDays: EligibleSalaryDays(periodStart, periodEnd, service),
		RuleID:       rule.ID, RuleVersion: rule.Version,
	}
	if proration.PeriodDays <= 0 {
		return SalaryProration{}, fmt.Errorf("%w: payroll period covers no days", ErrValidation)
	}
	// No eligible service is a real answer, not an error: a provider who left
	// before the period began is owed nothing, and saying so explicitly is
	// what stops a downstream default paying them anyway.
	if proration.EligibleDays == 0 {
		return proration, nil
	}

	switch rule.Config.Cadence {
	case CadenceDaily:
		// Exact. A daily salary times the days served has no remainder, so
		// there is nothing to round and nobody to round against.
		amount, ok := checkedMultiply(rule.Config.AmountMinor, proration.EligibleDays)
		if !ok {
			return SalaryProration{}, fmt.Errorf("%w: prorated salary overflows minor units", ErrValidation)
		}
		proration.AmountMinor = amount
		return proration, nil

	case CadenceWeekly:
		proration.DivisorDays = weeklySalaryDivisorDays

	case CadenceMonthly:
		if proration.PeriodDays < minimumMonthlyPeriodDays || proration.PeriodDays > maximumMonthlyPeriodDays {
			// A monthly rule prorated against a period that is not a month
			// would pay a month's salary for a fortnight, or a fortnight's for
			// a quarter. Refusing is the only safe answer, because either
			// direction is a real payroll error nobody would notice.
			return SalaryProration{}, fmt.Errorf(
				"%w: a monthly salary needs a monthly payroll period, got %d days",
				ErrValidation, proration.PeriodDays)
		}
		proration.DivisorDays = proration.PeriodDays

	default:
		return SalaryProration{}, fmt.Errorf("%w: salary cadence %q is not supported", ErrValidation, rule.Config.Cadence)
	}

	numerator, ok := checkedMultiply(rule.Config.AmountMinor, proration.EligibleDays)
	if !ok {
		return SalaryProration{}, fmt.Errorf("%w: prorated salary overflows minor units", ErrValidation)
	}
	amount := numerator / proration.DivisorDays
	proration.RemainderMinor = numerator - amount*proration.DivisorDays

	// Half-up on the remainder, matching the platform's default rounding
	// policy. Persisting the remainder alongside means the choice is visible
	// rather than inferred from a figure that does not quite divide.
	if proration.RemainderMinor*2 >= proration.DivisorDays {
		amount++
		proration.RoundedUp = true
	}
	proration.AmountMinor = amount
	return proration, nil
}

func checkedMultiply(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	product := a * b
	if product/b != a {
		return 0, false
	}
	return product, true
}
