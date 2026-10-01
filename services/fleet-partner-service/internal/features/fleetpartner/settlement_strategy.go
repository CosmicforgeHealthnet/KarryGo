package fleetpartner

// CMP-027 / CMP-027A: where each compensation component is actually paid.
//
// A compensation component is paid in one of two places and never both:
//
//   - IMMEDIATE_WORK_SETTLEMENT — settled with the completed job's own money,
//     through the managed settlement instruction.
//   - PAYROLL_ACCRUAL — accrued to a payroll period and paid there.
//
// The reason this is an explicit, persisted, plan-versioned decision rather
// than something inferred: payroll and work settlement are computed by
// different code at different times from different tables. If each guesses,
// they will eventually both decide yes for the same component, and the
// provider is paid twice for one job. Nothing downstream can detect that from
// the amounts alone, because a legitimate second component looks identical to
// a duplicated first one.

import (
	"fmt"
	"strings"
)

// Settlement strategies.
const (
	// StrategyImmediateWork settles the component with the job's money at
	// completion.
	StrategyImmediateWork = "IMMEDIATE_WORK_SETTLEMENT"

	// StrategyPayrollAccrual accrues the component to a payroll period.
	StrategyPayrollAccrual = "PAYROLL_ACCRUAL"
)

// DefaultSettlementStrategy is the strategy a rule gets when its plan does not
// state one.
//
// It reproduces the behaviour these rule types already had, so adopting the
// explicit field changes no existing plan's outcome: commission and per-job
// were settled with the job, everything else was not.
func DefaultSettlementStrategy(ruleType string) string {
	if SettlementLevelFor(ruleType) == SettlementWork {
		return StrategyImmediateWork
	}
	return StrategyPayrollAccrual
}

// SettlementStrategyFor returns the effective strategy for a rule.
func SettlementStrategyFor(rule CompensationRule) string {
	declared := strings.ToUpper(strings.TrimSpace(rule.Config.SettlementStrategy))
	if declared == "" {
		return DefaultSettlementStrategy(rule.RuleType)
	}
	return declared
}

// ValidateSettlementStrategy checks a rule's declared strategy is one this
// service can honour.
//
// Salary, per-shift and the periodic components may only accrue. An
// "immediate" salary would mean paying a month's pay on the first completed
// job of the month, and then again on the second: the rule type simply has no
// per-job meaning, so allowing the combination would only ever be a mistake.
func ValidateSettlementStrategy(rule CompensationRule) error {
	strategy := SettlementStrategyFor(rule)
	switch strategy {
	case StrategyImmediateWork, StrategyPayrollAccrual:
	default:
		return fmt.Errorf("%w: settlement strategy %q is not supported", ErrValidation, rule.Config.SettlementStrategy)
	}
	if strategy == StrategyImmediateWork && SettlementLevelFor(rule.RuleType) != SettlementWork {
		return fmt.Errorf(
			"%w: %s compensation cannot be settled per job; it has no per-job meaning",
			ErrValidation, rule.RuleType)
	}
	return nil
}

// SettlesWithTheJob reports whether a rule's component is paid at completion.
//
// A commission or per-job rate that an organization has chosen to accrue
// answers false here and is therefore absent from the job's settlement
// instruction entirely -- it is not posted as zero, it is not posted at all.
func SettlesWithTheJob(rule CompensationRule) bool {
	return SettlementLevelFor(rule.RuleType) == SettlementWork &&
		SettlementStrategyFor(rule) == StrategyImmediateWork
}

// AccruesToPayroll reports whether a rule's component is paid through payroll.
//
// Exactly one of SettlesWithTheJob and AccruesToPayroll is true for any rule,
// which is the whole point: there is no configuration in which a component is
// eligible for both.
func AccruesToPayroll(rule CompensationRule) bool { return !SettlesWithTheJob(rule) }

// WorkSettledComponent records one component that was paid with a job's money.
//
// Payroll reads these to know what it must NOT pay again. The identifiers are
// all here rather than only the amount, because "was this already paid?" has
// to be answerable about a specific component of a specific job, not about a
// figure that happens to match.
type WorkSettledComponent struct {
	OrganizationID string `json:"organizationId"`
	MembershipID   string `json:"membershipId"`
	SourceService  string `json:"sourceService"`
	SourceJobID    string `json:"sourceJobId"`

	// SourceAssignmentID is the Fleet-side assignment. Payroll walks
	// assignments rather than execution-service job ids, so this is the key it
	// matches on.
	SourceAssignmentID string `json:"sourceAssignmentId,omitempty"`

	CompletedWorkID          string `json:"completedWorkId"`
	CompensationResolutionID string `json:"compensationResolutionId"`
	SettlementInstructionID  string `json:"settlementInstructionId,omitempty"`
	WalletPostingOperationID string `json:"walletPostingOperationId,omitempty"`

	RuleID          string `json:"ruleId"`
	Component       string `json:"component"`
	SettlementLevel string `json:"settlementLevel"`

	// Strategy is the plan's declared settlement strategy, carried so an
	// auditor can see not just that a component was paid with the job but
	// that the plan said it should be.
	Strategy string `json:"strategy,omitempty"`

	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`
}

// ExclusionKey identifies one already-paid component.
//
// The job and the rule together, because one job may legitimately carry two
// components from different rules, and one rule legitimately applies to many
// jobs. Either alone would exclude too much or too little.
func (c WorkSettledComponent) ExclusionKey() string {
	return c.SourceService + ":" + c.SourceJobID + ":" + c.RuleID
}

// AssignmentExclusionKey identifies the same already-paid component from the
// payroll side, which knows the Fleet assignment rather than the execution
// service's job id.
func (c WorkSettledComponent) AssignmentExclusionKey() string {
	return c.SourceAssignmentID + ":" + c.RuleID
}

// WorkSettledComponentsFor extracts the components a resolution paid with the
// job's money.
//
// A payroll-level component that was merely recorded on the resolution is not
// returned: it was not paid, so payroll must still pay it.
func WorkSettledComponentsFor(
	record CompletedWorkRecord, resolution CompensationResolution, instructionID, walletOperationID string,
) []WorkSettledComponent {
	var settled []WorkSettledComponent
	for _, matched := range resolution.MatchedRules {
		if matched.Settlement != SettlementWork {
			continue
		}
		settled = append(settled, WorkSettledComponent{
			OrganizationID: record.OrganizationID, MembershipID: record.MembershipID,
			SourceService: record.SourceService, SourceJobID: record.SourceJobID,
			SourceAssignmentID: record.SourceAssignmentID,
			CompletedWorkID:    record.ID, CompensationResolutionID: resolution.ID,
			SettlementInstructionID: instructionID, WalletPostingOperationID: walletOperationID,
			RuleID: matched.RuleID, Component: matched.RuleType,
			SettlementLevel: SettlementWork, Strategy: matched.Strategy,
			AmountMinor: matched.AmountMinor, Currency: resolution.Currency,
		})
	}
	return settled
}

// PayrollExclusions turns already-settled components into a lookup payroll can
// consult.
//
// Both keys are indexed, because the two sides of this check know the job by
// different names: work settlement knows the execution service's job id,
// payroll knows the Fleet assignment.
func PayrollExclusions(settled []WorkSettledComponent) map[string]WorkSettledComponent {
	exclusions := make(map[string]WorkSettledComponent, len(settled)*2)
	for _, component := range settled {
		exclusions[component.ExclusionKey()] = component
		if component.SourceAssignmentID != "" {
			exclusions[component.AssignmentExclusionKey()] = component
		}
	}
	return exclusions
}

// AlreadyPaidAtWorkLevel reports whether payroll must skip this component.
func AlreadyPaidAtWorkLevel(exclusions map[string]WorkSettledComponent, sourceService, sourceJobID, ruleID string) bool {
	_, paid := exclusions[sourceService+":"+sourceJobID+":"+ruleID]
	return paid
}

// AssignmentAlreadyPaidAtWorkLevel reports whether payroll must skip an
// assignment's component because the job already paid it.
func AssignmentAlreadyPaidAtWorkLevel(exclusions map[string]WorkSettledComponent, sourceAssignmentID, ruleID string) bool {
	if sourceAssignmentID == "" {
		return false
	}
	_, paid := exclusions[sourceAssignmentID+":"+ruleID]
	return paid
}
