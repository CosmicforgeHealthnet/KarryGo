package fleetpartner

// CMP-011 / 012 / 013 / 014: the payroll components that are not a salary.
//
// What these four have in common is that each one is a way for money to enter
// or leave a payslip without a completed job behind it. That makes them the
// easiest place for an unexplained figure to appear -- and the hardest place to
// notice one, because a payslip line with a plausible amount looks exactly like
// a correct one.
//
// So none of them is computed from an implied basis, an inferred quantity or an
// unrecorded authorisation. Each carries the evidence that produced it, and an
// input that cannot produce that evidence is refused rather than defaulted.

import (
	"fmt"
	"strings"
	"time"
)

// Allowance bases (CMP-011).
//
// An allowance must say what it is based on. Inferring it from the rule type
// would mean the same configuration paying different amounts depending on which
// code path reached it.
const (
	// AllowanceBasisFixed pays a flat amount for the period.
	AllowanceBasisFixed = "FIXED"

	// AllowanceBasisEligibleSalary pays a proportion of the salary actually
	// earned in the period -- the prorated figure, not the headline salary, so
	// a provider who served half a month does not receive a full month's
	// housing allowance.
	AllowanceBasisEligibleSalary = "ELIGIBLE_SALARY"

	// AllowanceBasisPerShift, PerDay and PerJob pay per counted unit.
	AllowanceBasisPerShift = "PER_SHIFT"
	AllowanceBasisPerDay   = "PER_DAY"
	AllowanceBasisPerJob   = "PER_JOB"
)

// ValidAllowanceBasis reports whether a basis is one this service can compute.
//
// Fails closed: an unrecognised basis is refused, never treated as fixed. A
// typo that silently became a flat payment would be indistinguishable from an
// intended one.
func ValidAllowanceBasis(basis string) bool {
	switch strings.ToUpper(strings.TrimSpace(basis)) {
	case AllowanceBasisFixed, AllowanceBasisEligibleSalary,
		AllowanceBasisPerShift, AllowanceBasisPerDay, AllowanceBasisPerJob:
		return true
	default:
		return false
	}
}

// AllowanceInputs are the counted quantities an allowance may be based on.
//
// They are supplied by the caller rather than derived here, because this
// package does not own attendance, shifts or job counts -- and a component that
// invented its own quantities would be unauditable by the service that does.
type AllowanceInputs struct {
	EligibleSalaryMinor int64
	Shifts              int64
	Days                int64
	Jobs                int64
}

// ComponentProvenance is the evidence behind one payroll line.
//
// Every non-salary component carries one. A payslip line that cannot say where
// it came from is exactly the line nobody can challenge.
type ComponentProvenance struct {
	Component   string `json:"component"`
	RuleID      string `json:"ruleId,omitempty"`
	RuleVersion int64  `json:"ruleVersion,omitempty"`

	Basis         string `json:"basis"`
	BasisQuantity int64  `json:"basisQuantity"`
	BasisUnit     string `json:"basisUnit,omitempty"`
	RateMinor     int64  `json:"rateMinor,omitempty"`
	RateBps       int    `json:"rateBps,omitempty"`

	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`

	PeriodStart time.Time `json:"periodStart"`
	PeriodEnd   time.Time `json:"periodEnd"`

	// AuthorizationID and AuthorizedBy name what permitted this line to exist,
	// for the components that require an authorisation.
	AuthorizationID string `json:"authorizationId,omitempty"`
	AuthorizedBy    string `json:"authorizedBy,omitempty"`
	Reason          string `json:"reason,omitempty"`
	SourceReference string `json:"sourceReference,omitempty"`
}

// CalculateAllowance computes an allowance and the working behind it.
func CalculateAllowance(rule CompensationRule, periodStart, periodEnd time.Time, inputs AllowanceInputs) (ComponentProvenance, error) {
	if rule.RuleType != RuleAllowance {
		return ComponentProvenance{}, fmt.Errorf("%w: %s is not an allowance rule", ErrValidation, rule.RuleType)
	}
	basis := strings.ToUpper(strings.TrimSpace(rule.Config.Basis))
	if !ValidAllowanceBasis(basis) {
		return ComponentProvenance{}, fmt.Errorf(
			"%w: allowance basis %q is not supported", ErrValidation, rule.Config.Basis)
	}
	provenance := ComponentProvenance{
		Component: RuleAllowance, RuleID: rule.ID, RuleVersion: rule.Version,
		Basis: basis, Currency: rule.Config.Currency,
		PeriodStart: periodStart, PeriodEnd: periodEnd,
	}

	switch basis {
	case AllowanceBasisFixed:
		provenance.BasisQuantity, provenance.BasisUnit = 1, "PERIOD"
		provenance.RateMinor = rule.Config.AmountMinor
		provenance.AmountMinor = rule.Config.AmountMinor

	case AllowanceBasisEligibleSalary:
		if err := ValidateBPS(rule.Config.RateBps); err != nil {
			return ComponentProvenance{}, err
		}
		if inputs.EligibleSalaryMinor < 0 {
			return ComponentProvenance{}, fmt.Errorf("%w: eligible salary cannot be negative", ErrValidation)
		}
		if !ValidRoundingPolicy(rule.Config.Rounding) {
			return ComponentProvenance{}, fmt.Errorf(
				"%w: allowance rounding policy %q is not supported", ErrValidation, rule.Config.Rounding)
		}
		provenance.BasisQuantity, provenance.BasisUnit = inputs.EligibleSalaryMinor, "MINOR"
		provenance.RateBps = rule.Config.RateBps
		product, ok := checkedMultiply(inputs.EligibleSalaryMinor, int64(rule.Config.RateBps))
		if !ok {
			return ComponentProvenance{}, fmt.Errorf("%w: allowance overflows minor units", ErrValidation)
		}
		provenance.AmountMinor = ApplyRounding(product, rule.Config.Rounding)

	case AllowanceBasisPerShift, AllowanceBasisPerDay, AllowanceBasisPerJob:
		quantity, unit := inputs.Shifts, "SHIFT"
		if basis == AllowanceBasisPerDay {
			quantity, unit = inputs.Days, "DAY"
		}
		if basis == AllowanceBasisPerJob {
			quantity, unit = inputs.Jobs, "JOB"
		}
		if quantity < 0 {
			return ComponentProvenance{}, fmt.Errorf("%w: allowance quantity cannot be negative", ErrValidation)
		}
		amount, ok := checkedMultiply(rule.Config.AmountMinor, quantity)
		if !ok {
			return ComponentProvenance{}, fmt.Errorf("%w: allowance overflows minor units", ErrValidation)
		}
		provenance.BasisQuantity, provenance.BasisUnit = quantity, unit
		provenance.RateMinor = rule.Config.AmountMinor
		provenance.AmountMinor = amount
	}

	if provenance.AmountMinor < 0 {
		return ComponentProvenance{}, fmt.Errorf("%w: an allowance cannot be negative", ErrValidation)
	}
	return provenance, nil
}

// ApprovedOvertimeRecord is an authoritative overtime approval (CMP-012).
//
// Overtime is paid from these and from nothing else. Deriving it from job
// duration or attendance would mean paying for time nobody approved, which is
// the difference between an overtime policy and an open till.
type ApprovedOvertimeRecord struct {
	ID             string
	MembershipID   string
	OrganizationID string

	// ApprovedUnits is the approved quantity -- hours, shifts, whatever the
	// approval was denominated in. The unit travels with it so a payslip can
	// say "6 hours" rather than "6".
	ApprovedUnits int64
	Unit          string

	ApprovedBy string
	ApprovedAt time.Time

	// PeriodStart and PeriodEnd bound which payroll period may pay it. An
	// approval reused in a later period is a second payment for one piece of
	// work.
	PeriodStart time.Time
	PeriodEnd   time.Time
}

// CalculateOvertime pays approved overtime, and only approved overtime.
func CalculateOvertime(rule CompensationRule, periodStart, periodEnd time.Time, record ApprovedOvertimeRecord) (ComponentProvenance, error) {
	if rule.RuleType != RuleOvertime {
		return ComponentProvenance{}, fmt.Errorf("%w: %s is not an overtime rule", ErrValidation, rule.RuleType)
	}
	if strings.TrimSpace(record.ID) == "" {
		return ComponentProvenance{}, fmt.Errorf("%w: overtime requires an approval record", ErrValidation)
	}
	if strings.TrimSpace(record.ApprovedBy) == "" || record.ApprovedAt.IsZero() {
		return ComponentProvenance{}, fmt.Errorf("%w: overtime approval must name who approved it and when", ErrValidation)
	}
	if record.ApprovedUnits <= 0 {
		return ComponentProvenance{}, fmt.Errorf("%w: overtime approval must carry an approved quantity", ErrValidation)
	}
	if strings.TrimSpace(record.Unit) == "" {
		return ComponentProvenance{}, fmt.Errorf("%w: overtime approval must state its unit", ErrValidation)
	}
	// An approval belongs to the period it was approved for. Paying it in a
	// later period pays the same work twice -- once in each.
	if record.PeriodStart.IsZero() || record.PeriodEnd.IsZero() ||
		!record.PeriodStart.Equal(periodStart) || !record.PeriodEnd.Equal(periodEnd) {
		return ComponentProvenance{}, fmt.Errorf(
			"%w: this overtime approval belongs to a different payroll period", ErrValidation)
	}
	amount, ok := checkedMultiply(rule.Config.AmountMinor, record.ApprovedUnits)
	if !ok {
		return ComponentProvenance{}, fmt.Errorf("%w: overtime overflows minor units", ErrValidation)
	}
	return ComponentProvenance{
		Component: RuleOvertime, RuleID: rule.ID, RuleVersion: rule.Version,
		Basis: "APPROVED_OVERTIME", BasisQuantity: record.ApprovedUnits, BasisUnit: strings.ToUpper(record.Unit),
		RateMinor: rule.Config.AmountMinor, AmountMinor: amount, Currency: rule.Config.Currency,
		PeriodStart: periodStart, PeriodEnd: periodEnd,
		AuthorizationID: record.ID, AuthorizedBy: record.ApprovedBy,
	}, nil
}

// BonusAuthorization is what permits a bonus to exist (CMP-013).
type BonusAuthorization struct {
	ID           string
	Source       string
	Reason       string
	AuthorizedBy string
	AuthorizedAt time.Time
	AmountMinor  int64
	Currency     string
	PeriodStart  time.Time
	PeriodEnd    time.Time
}

// CalculateBonus records a bonus with its provenance.
//
// A bonus with no valid authorisation does not enter payroll quietly with a
// zero amount -- it is refused, because a silent zero and a silent full payment
// are the same class of bug and only one of them is visible.
func CalculateBonus(periodStart, periodEnd time.Time, authorization BonusAuthorization) (ComponentProvenance, error) {
	if strings.TrimSpace(authorization.ID) == "" ||
		strings.TrimSpace(authorization.Source) == "" ||
		strings.TrimSpace(authorization.Reason) == "" ||
		strings.TrimSpace(authorization.AuthorizedBy) == "" ||
		authorization.AuthorizedAt.IsZero() {
		return ComponentProvenance{}, fmt.Errorf(
			"%w: a bonus needs a source, a reason, an authorising actor and a timestamp", ErrValidation)
	}
	if authorization.AmountMinor <= 0 {
		return ComponentProvenance{}, fmt.Errorf("%w: a bonus must carry a positive amount", ErrValidation)
	}
	if err := ValidateMoney(authorization.AmountMinor, authorization.Currency); err != nil {
		return ComponentProvenance{}, err
	}
	if !authorization.PeriodStart.Equal(periodStart) || !authorization.PeriodEnd.Equal(periodEnd) {
		return ComponentProvenance{}, fmt.Errorf(
			"%w: this bonus was authorised for a different payroll period", ErrValidation)
	}
	return ComponentProvenance{
		Component: RuleBonus, Basis: strings.ToUpper(strings.TrimSpace(authorization.Source)),
		BasisQuantity: 1, BasisUnit: "AWARD",
		AmountMinor: authorization.AmountMinor, Currency: authorization.Currency,
		PeriodStart: periodStart, PeriodEnd: periodEnd,
		AuthorizationID: authorization.ID, AuthorizedBy: authorization.AuthorizedBy,
		Reason: authorization.Reason,
	}, nil
}

// DeductionAuthorization is what permits money to be taken off a payslip
// (CMP-014).
//
// More is required here than for a bonus, deliberately: a deduction reduces
// what someone is paid, so "why is this less than I expected?" has to have an
// answer that does not require reading the code.
type DeductionAuthorization struct {
	ID            string
	DeductionType string
	Source        string
	Reason        string
	AuthorizedBy  string
	AuthorizedAt  time.Time
	AmountMinor   int64
	Currency      string
	PeriodStart   time.Time
	PeriodEnd     time.Time

	// LiabilityReference points at what is being recovered -- a damaged item,
	// an advance, a fine. OutstandingBeforeMinor and the remaining balance let
	// a provider see an instalment in context rather than as an unexplained
	// subtraction.
	LiabilityReference     string
	OutstandingBeforeMinor int64
}

// DeductionLine is a deduction and everything needed to explain it.
type DeductionLine struct {
	ComponentProvenance
	DeductionType          string `json:"deductionType"`
	LiabilityReference     string `json:"liabilityReference,omitempty"`
	OutstandingBeforeMinor int64  `json:"outstandingBeforeMinor"`
	OutstandingAfterMinor  int64  `json:"outstandingAfterMinor"`
}

// CalculateDeduction records a deduction with full provenance.
func CalculateDeduction(periodStart, periodEnd time.Time, authorization DeductionAuthorization) (DeductionLine, error) {
	if strings.TrimSpace(authorization.ID) == "" ||
		strings.TrimSpace(authorization.DeductionType) == "" ||
		strings.TrimSpace(authorization.Source) == "" ||
		strings.TrimSpace(authorization.Reason) == "" ||
		strings.TrimSpace(authorization.AuthorizedBy) == "" ||
		authorization.AuthorizedAt.IsZero() {
		return DeductionLine{}, fmt.Errorf(
			"%w: a deduction needs a type, a source, a reason, an authorising actor and a timestamp", ErrValidation)
	}
	if authorization.AmountMinor <= 0 {
		return DeductionLine{}, fmt.Errorf("%w: a deduction must carry a positive amount", ErrValidation)
	}
	if err := ValidateMoney(authorization.AmountMinor, authorization.Currency); err != nil {
		return DeductionLine{}, err
	}
	if !authorization.PeriodStart.Equal(periodStart) || !authorization.PeriodEnd.Equal(periodEnd) {
		return DeductionLine{}, fmt.Errorf(
			"%w: this deduction was authorised for a different payroll period", ErrValidation)
	}
	if authorization.OutstandingBeforeMinor < 0 {
		return DeductionLine{}, fmt.Errorf("%w: an outstanding liability cannot be negative", ErrValidation)
	}
	// Recovering more than is owed is not a deduction, it is a charge. If a
	// liability is stated, the instalment cannot exceed it.
	if authorization.LiabilityReference != "" && authorization.AmountMinor > authorization.OutstandingBeforeMinor {
		return DeductionLine{}, fmt.Errorf(
			"%w: a deduction of %d exceeds the %d still owed",
			ErrValidation, authorization.AmountMinor, authorization.OutstandingBeforeMinor)
	}
	remaining := authorization.OutstandingBeforeMinor - authorization.AmountMinor
	if authorization.LiabilityReference == "" {
		remaining = 0
	}
	return DeductionLine{
		ComponentProvenance: ComponentProvenance{
			Component: RuleDeduction, Basis: strings.ToUpper(strings.TrimSpace(authorization.Source)),
			BasisQuantity: 1, BasisUnit: "DEDUCTION",
			AmountMinor: authorization.AmountMinor, Currency: authorization.Currency,
			PeriodStart: periodStart, PeriodEnd: periodEnd,
			AuthorizationID: authorization.ID, AuthorizedBy: authorization.AuthorizedBy,
			Reason: authorization.Reason, SourceReference: authorization.LiabilityReference,
		},
		DeductionType:          strings.ToUpper(strings.TrimSpace(authorization.DeductionType)),
		LiabilityReference:     authorization.LiabilityReference,
		OutstandingBeforeMinor: authorization.OutstandingBeforeMinor,
		OutstandingAfterMinor:  remaining,
	}, nil
}
