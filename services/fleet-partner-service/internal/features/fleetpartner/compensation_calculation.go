package fleetpartner

// Section 13: how a compensation plan turns into money for one completed job.
//
// The distinction this file exists to make is between two kinds of
// compensation that were previously calculated as though they were the same:
//
//   - WORK-level components are earned by doing the job. A commission or a
//     per-job rate is settled with that job's money.
//   - PAYROLL-level components are earned by being employed. A salary, an
//     approved shift, a monthly allowance, overtime, a bonus, a deduction --
//     none of these are caused by the job that happens to have just finished,
//     and paying them per job would pay them once per job instead of once per
//     period.
//
// Collapsing the two is the double-pay bug: a salaried provider who is also
// credited every completed job is paid twice for the same work.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Settlement levels.
const (
	// SettlementWork is settled against the completed job's own money.
	SettlementWork = "WORK"

	// SettlementPayroll accrues to a payroll period and is settled there.
	SettlementPayroll = "PAYROLL"
)

// Rounding policies for commission arithmetic (CMP-008).
//
// The policy is explicit because "round it sensibly" is not reproducible, and
// a compensation resolution has to be reproducible years later from its stored
// plan version.
const (
	RoundingHalfUp = "HALF_UP"
	RoundingDown   = "DOWN"
	RoundingUp     = "UP"
)

// SettlementLevelFor reports when a rule type is settled.
//
// PER_SHIFT sits on the payroll side deliberately. A shift is not a job: it is
// approved work-session data, and paying a per-shift rate on every completed
// job inside that shift would multiply it by the number of jobs.
func SettlementLevelFor(ruleType string) string {
	switch ruleType {
	case RulePerJob, RuleCommission:
		return SettlementWork
	case RuleSalary, RulePerShift, RuleAllowance, RuleOvertime, RuleBonus, RuleDeduction:
		return SettlementPayroll
	default:
		return SettlementPayroll
	}
}

// ruleOrder fixes the order work-level rules are applied in (CMP-015).
//
// It matters because a commission taken on ORGANIZATION_JOB_SHARE is taken on
// what is left after the other work-level components, so "what is left" has to
// mean the same thing every time the same job is resolved.
func ruleOrder(ruleType string) int {
	switch ruleType {
	case RulePerJob:
		return 1
	case RuleCommission:
		return 2
	default:
		return 3
	}
}

// SortRulesForCalculation returns the rules in deterministic application
// order. Ties break on rule id so two rules of the same type never depend on
// the order the database happened to return them in.
func SortRulesForCalculation(rules []CompensationRule) []CompensationRule {
	ordered := make([]CompensationRule, len(rules))
	copy(ordered, rules)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ruleOrder(ordered[i].RuleType), ruleOrder(ordered[j].RuleType)
		if left != right {
			return left < right
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

// CommissionBasisMinor resolves the amount a commission rate applies to
// (CMP-009).
//
// eligibleBasisMinor is the money the execution service reported as available
// for corporate allocation. providerWorkSoFarMinor is what earlier work-level
// components have already allocated to the provider.
//
// A basis this function does not recognise is an error, never a default: a
// commission silently taken on the wrong number is a compensation bug nobody
// notices until a provider complains.
func CommissionBasisMinor(basis string, eligibleBasisMinor, providerWorkSoFarMinor int64) (int64, error) {
	switch strings.ToUpper(strings.TrimSpace(basis)) {
	case CommissionBasisEligibleJobGross:
		return eligibleBasisMinor, nil

	case CommissionBasisProviderEligibleBase:
		// The provider-eligible base is the same eligible figure: it is what
		// the execution service declared the provider's work was worth before
		// any corporate split.
		return eligibleBasisMinor, nil

	case CommissionBasisOrganizationJobShare:
		// What would otherwise fall to the organization, once earlier
		// work-level provider components are taken out.
		remaining := eligibleBasisMinor - providerWorkSoFarMinor
		if remaining < 0 {
			remaining = 0
		}
		return remaining, nil

	default:
		return 0, fmt.Errorf("%w: commission basis %q is not supported", ErrValidation, basis)
	}
}

// ApplyRounding converts an exact basis-point product into minor units under
// an explicit policy.
//
// The product is carried exactly (basis x rateBps) and divided by 10000 here,
// so no intermediate floating point is involved and the same inputs always
// give the same integer.
func ApplyRounding(product int64, policy string) int64 {
	const scale = int64(10000)
	if product <= 0 {
		return 0
	}
	switch strings.ToUpper(strings.TrimSpace(policy)) {
	case RoundingDown:
		return product / scale
	case RoundingUp:
		return (product + scale - 1) / scale
	case RoundingHalfUp, "":
		// Half-up is the default because it is what the existing
		// implementation did; changing the default would silently restate
		// every historical commission by a unit.
		return (product + scale/2) / scale
	default:
		return (product + scale/2) / scale
	}
}

// ValidRoundingPolicy reports whether a policy is one this service applies.
func ValidRoundingPolicy(policy string) bool {
	switch strings.ToUpper(strings.TrimSpace(policy)) {
	case "", RoundingHalfUp, RoundingDown, RoundingUp:
		return true
	default:
		return false
	}
}

// WorkResolution is the outcome of applying a plan to one completed job.
type WorkResolution struct {
	// ProviderShareMinor is what the provider is owed for this job, settled
	// with this job's money. For a pure salary plan it is zero, and that is
	// the correct answer rather than a missing one.
	ProviderShareMinor int64

	// OrganizationShareMinor is the remainder of the eligible basis.
	OrganizationShareMinor int64

	// Matched carries every rule that applied, work-level and payroll-level
	// alike, so the resolution explains itself. Payroll-level rules contribute
	// nothing to the job shares; they are recorded so payroll can see that
	// this job's period had them in force.
	Matched []MatchedRule
}

// ResolveWorkCompensation applies a plan's rules to one completed job.
//
// Rules that do not apply to this service, role or moment are simply absent
// from the result. A rule that applies but cannot be calculated is an error,
// because resolving a job against a plan this service does not understand
// would produce a number nobody can defend.
func ResolveWorkCompensation(
	rules []CompensationRule, eligibleBasisMinor int64, currency, service, role string, at time.Time,
) (WorkResolution, error) {
	var out WorkResolution
	for _, rule := range SortRulesForCalculation(rules) {
		if !RuleApplies(rule, service, role, at) {
			continue
		}
		if err := ValidateRule(rule); err != nil {
			return WorkResolution{}, err
		}
		if err := ValidateSettlementStrategy(rule); err != nil {
			return WorkResolution{}, err
		}
		matched := MatchedRule{
			RuleID: rule.ID, RuleType: rule.RuleType,
			Settlement: SettlementLevelFor(rule.RuleType),
			Strategy:   SettlementStrategyFor(rule),
		}
		// A component the plan chose to accrue is settled through payroll even
		// though its type could have been settled with the job. It is recorded
		// at payroll level and contributes nothing here -- not posted as zero,
		// not posted at all (CMP-027A, CMP-027C).
		if !SettlesWithTheJob(rule) {
			matched.Settlement = SettlementPayroll
		}
		if matched.Settlement == SettlementPayroll {
			// Recorded, deliberately worth nothing here. The provider is paid
			// for it through the payroll period, not through this job.
			out.Matched = append(out.Matched, matched)
			continue
		}

		switch rule.RuleType {
		case RulePerJob:
			if !strings.EqualFold(rule.Config.Currency, currency) {
				return WorkResolution{}, fmt.Errorf("%w: per-job currency does not match the job currency", ErrValidation)
			}
			matched.AmountMinor = rule.Config.AmountMinor

		case RuleCommission:
			basis, err := CommissionBasisMinor(rule.Config.Basis, eligibleBasisMinor, out.ProviderShareMinor)
			if err != nil {
				return WorkResolution{}, err
			}
			matched.BasisMinor = basis
			matched.BasisType = strings.ToUpper(strings.TrimSpace(rule.Config.Basis))
			matched.RateBps = rule.Config.RateBps
			matched.Rounding = roundingPolicyOrDefault(rule.Config.Rounding)
			matched.AmountMinor = ApplyRounding(basis*int64(rule.Config.RateBps), matched.Rounding)
		}

		out.ProviderShareMinor += matched.AmountMinor
		out.Matched = append(out.Matched, matched)
	}

	// The provider can never be allocated more than the job actually carried.
	// If a plan asks for more, the plan is wrong, and quietly paying the
	// larger figure out of money that does not exist is worse than refusing.
	if out.ProviderShareMinor > eligibleBasisMinor {
		return WorkResolution{}, fmt.Errorf(
			"%w: resolved provider share %d exceeds the eligible basis %d",
			ErrValidation, out.ProviderShareMinor, eligibleBasisMinor)
	}
	out.OrganizationShareMinor = eligibleBasisMinor - out.ProviderShareMinor
	return out, nil
}

func roundingPolicyOrDefault(policy string) string {
	normalized := strings.ToUpper(strings.TrimSpace(policy))
	if normalized == "" {
		return RoundingHalfUp
	}
	return normalized
}
