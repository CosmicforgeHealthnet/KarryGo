// Package fleetpartner implements the Section 7 corporate-provider control
// plane foundations. It deliberately does not own trip execution or wallet
// ledger posting.
package fleetpartner

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/money"
)

const (
	ServiceFleet         = "FLEET"
	ServiceTaxi          = "TAXI"
	ServiceDispatch      = "DISPATCH"
	ServiceHauling       = "HAULING"
	ServiceVendor        = "VENDOR"
	ServicePaymentWallet = "PAYMENT_WALLET"

	PlanDraft    = "DRAFT"
	PlanActive   = "ACTIVE"
	PlanInactive = "INACTIVE"
	PlanArchived = "ARCHIVED"

	RuleSalary     = "SALARY"
	RulePerJob     = "PER_JOB"
	RulePerShift   = "PER_SHIFT"
	RuleCommission = "COMMISSION"
	RuleAllowance  = "ALLOWANCE"
	RuleOvertime   = "OVERTIME"
	RuleBonus      = "BONUS"
	RuleDeduction  = "DEDUCTION"

	CadenceDaily   = "DAILY"
	CadenceWeekly  = "WEEKLY"
	CadenceMonthly = "MONTHLY"

	CommissionBasisEligibleJobGross     = "ELIGIBLE_JOB_GROSS"
	CommissionBasisOrganizationJobShare = "ORGANIZATION_JOB_SHARE"
	CommissionBasisProviderEligibleBase = "PROVIDER_ELIGIBLE_BASE"

	VisibilityShowCustomerFare   = "showCustomerFare"
	VisibilityShowOwnEarning     = "showOwnEarning"
	VisibilityShowCommissionRate = "showCommissionRate"
	VisibilityShowProviderShare  = "showProviderShare"
	VisibilityShowPayroll        = "showPayroll"
	VisibilityShowTips           = "showTips"
	VisibilityShowAllowances     = "showAllowances"
	VisibilityShowBonuses        = "showBonuses"
)

var ErrValidation = errors.New("fleetpartner: validation failed")

type ProviderFinancialVisibilityPolicy struct {
	ShowCustomerFare   bool `json:"showCustomerFare"`
	ShowOwnEarning     bool `json:"showOwnEarning"`
	ShowCommissionRate bool `json:"showCommissionRate"`
	ShowProviderShare  bool `json:"showProviderShare"`
	ShowPayroll        bool `json:"showPayroll"`
	ShowTips           bool `json:"showTips"`
	ShowAllowances     bool `json:"showAllowances"`
	ShowBonuses        bool `json:"showBonuses"`
}

func ConservativeVisibilityPolicy() ProviderFinancialVisibilityPolicy {
	return ProviderFinancialVisibilityPolicy{ShowPayroll: true}
}

func CommissionVisibilityPolicy() ProviderFinancialVisibilityPolicy {
	return ProviderFinancialVisibilityPolicy{
		ShowOwnEarning: true, ShowCommissionRate: true,
		ShowProviderShare: true, ShowPayroll: true,
	}
}

func (p ProviderFinancialVisibilityPolicy) AsMap() map[string]bool {
	return map[string]bool{
		VisibilityShowCustomerFare:   p.ShowCustomerFare,
		VisibilityShowOwnEarning:     p.ShowOwnEarning,
		VisibilityShowCommissionRate: p.ShowCommissionRate,
		VisibilityShowProviderShare:  p.ShowProviderShare,
		VisibilityShowPayroll:        p.ShowPayroll,
		VisibilityShowTips:           p.ShowTips,
		VisibilityShowAllowances:     p.ShowAllowances,
		VisibilityShowBonuses:        p.ShowBonuses,
	}
}

func VisibilityPolicyFromMap(values map[string]bool) ProviderFinancialVisibilityPolicy {
	p := ConservativeVisibilityPolicy()
	p.ShowCustomerFare = values[VisibilityShowCustomerFare]
	p.ShowOwnEarning = values[VisibilityShowOwnEarning]
	p.ShowCommissionRate = values[VisibilityShowCommissionRate]
	p.ShowProviderShare = values[VisibilityShowProviderShare]
	if v, ok := values[VisibilityShowPayroll]; ok {
		p.ShowPayroll = v
	}
	p.ShowTips = values[VisibilityShowTips]
	p.ShowAllowances = values[VisibilityShowAllowances]
	p.ShowBonuses = values[VisibilityShowBonuses]
	return p
}

type CompensationPlan struct {
	ID, OrganizationID, Name, Description, Currency, Status string
	Version                                                 int64
	EffectiveFrom                                           time.Time
	EffectiveTo                                             *time.Time
	CreatedBy, ApprovedBy                                   string
	CreatedAt, UpdatedAt                                    time.Time
}

type RuleConfig struct {
	AmountMinor int64  `json:"amountMinor,omitempty"`
	Currency    string `json:"currency,omitempty"`
	Cadence     string `json:"cadence,omitempty"`
	RateBps     int    `json:"rateBps,omitempty"`
	Basis       string `json:"basis,omitempty"`
	Rounding    string `json:"rounding,omitempty"`
	BasisMode   string `json:"basisMode,omitempty"`

	// SettlementStrategy says where this component is paid: with the job, or
	// through payroll (CMP-027A). It lives in the rule's versioned
	// configuration so the answer is reproducible from the plan version a
	// historical job was resolved against, rather than from today's code.
	SettlementStrategy string `json:"settlementStrategy,omitempty"`
}

type CompensationRule struct {
	ID, OrganizationID, PlanID, RuleType, Status string
	PlanVersion, Version                         int64
	ApplicableRoles, ApplicableServices          []string
	Config                                       RuleConfig
	EffectiveFrom                                time.Time
	EffectiveTo                                  *time.Time
	CreatedAt, UpdatedAt                         time.Time
}

type MembershipCompensationAssignment struct {
	ID, MembershipID, OrganizationID, CompensationPlanID string
	CompensationPlanVersion                              int64
	ServiceContext, Currency, Status                     string
	EffectiveFrom                                        time.Time
	EffectiveTo                                          *time.Time
	AssignedBy, ApprovedBy                               string
	CreatedAt, UpdatedAt                                 time.Time
}

type CompletedWorkRecord struct {
	ID, OrganizationID, MembershipID, ProviderID      string
	SourceService, SourceJobID, SourceAssignmentID    string
	WorkType, Currency, IngestionStatus               string
	CompletedAt                                       time.Time
	EligibleBasisMinor, SourceVersion, Version        int64
	RequestFingerprint, IdempotencyKey, CorrelationID string
	CreatedAt                                         time.Time
}

type CompensationResolution struct {
	ID, OrganizationID, CompletedWorkID, MembershipID string
	CompensationPlanID                                string
	CompensationPlanVersion                           int64
	MatchedRules                                      []MatchedRule
	ProviderShareMinor, OrganizationShareMinor        int64
	Currency                                          string
	CalculationBasis                                  map[string]any
	VisibilityPolicySnapshot                          ProviderFinancialVisibilityPolicy
	ResolvedAt                                        time.Time
	ResolutionVersion                                 int64
	Status                                            string
}

type MatchedRule struct {
	RuleID      string `json:"ruleId"`
	RuleType    string `json:"ruleType"`
	AmountMinor int64  `json:"amountMinor"`
	BasisMinor  int64  `json:"basisMinor,omitempty"`
	RateBps     int    `json:"rateBps,omitempty"`

	// Settlement says whether this component was settled with the job's money
	// or accrued to a payroll period (CMP-016). Without it a resolution cannot
	// be read back to explain why a salaried provider's job share was zero.
	Settlement string `json:"settlement,omitempty"`

	// BasisType and Rounding record HOW a commission was calculated, not just
	// what it produced, so the figure stays reproducible when the plan is
	// re-read years later.
	BasisType string `json:"basisType,omitempty"`
	Rounding  string `json:"rounding,omitempty"`

	// Strategy is the plan's declared settlement strategy for this component
	// (CMP-027A). Stored on the resolution so payroll can tell an accrued
	// commission from one that was already paid with the job, without having
	// to re-read and re-interpret the plan.
	Strategy string `json:"strategy,omitempty"`
}

type ManagedWorkSettlementInstruction struct {
	InstructionID, OrganizationID, MembershipID, ProviderID     string
	SourceService, SourceJobID, SourceAssignmentID              string
	CompensationPlanID                                          string
	CompensationPlanVersion, CompensationResolutionVersion      int64
	GrossBasisMinor, ProviderShareMinor, OrganizationShareMinor int64
	PlatformShareMinor                                          *int64
	Currency, Status, IdempotencyKey, RequestFingerprint        string
	VisibilityPolicySnapshot                                    ProviderFinancialVisibilityPolicy
	Version                                                     int64
	CreatedAt                                                   time.Time
}

type CorporateContext struct {
	ProviderID                string                            `json:"providerId"`
	IsManagedProvider         bool                              `json:"isManagedProvider"`
	MembershipID              string                            `json:"membershipId,omitempty"`
	OrganizationID            string                            `json:"organizationId,omitempty"`
	MembershipStatus          string                            `json:"membershipStatus,omitempty"`
	OrganizationRole          string                            `json:"organizationRole,omitempty"`
	OperatingMode             string                            `json:"operatingMode,omitempty"`
	ServiceEligibility        map[string]bool                   `json:"serviceEligibility"`
	CompensationContext       map[string]any                    `json:"compensationContext,omitempty"`
	FinancialVisibilityPolicy ProviderFinancialVisibilityPolicy `json:"financialVisibilityPolicy"`
	Version                   int64                             `json:"version"`
}

func ValidateSourceService(value string) bool {
	switch value {
	case ServiceTaxi, ServiceDispatch, ServiceHauling, ServiceFleet, ServiceVendor:
		return true
	default:
		return false
	}
}

func ValidateBPS(rate int) error {
	if rate < 0 || rate > 10000 {
		return fmt.Errorf("%w: rateBps must be between 0 and 10000", ErrValidation)
	}
	return nil
}

func ValidateMoney(amountMinor int64, currency string) error {
	if amountMinor < 0 {
		return fmt.Errorf("%w: amountMinor must be non-negative", ErrValidation)
	}
	_, err := money.New(amountMinor, currency)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return nil
}

func ValidateUTC(name string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%w: %s is required", ErrValidation, name)
	}
	if value.Location() != time.UTC {
		return fmt.Errorf("%w: %s must be UTC", ErrValidation, name)
	}
	return nil
}

func ValidateRule(rule CompensationRule) error {
	if rule.RuleType == "" || rule.Status == "" {
		return fmt.Errorf("%w: rule type and status are required", ErrValidation)
	}
	switch rule.RuleType {
	case RuleSalary:
		if err := ValidateMoney(rule.Config.AmountMinor, rule.Config.Currency); err != nil {
			return err
		}
		if rule.Config.Cadence != CadenceDaily && rule.Config.Cadence != CadenceWeekly && rule.Config.Cadence != CadenceMonthly {
			return fmt.Errorf("%w: salary cadence is invalid", ErrValidation)
		}
	case RuleCommission:
		if err := ValidateBPS(rule.Config.RateBps); err != nil {
			return err
		}
		switch rule.Config.Basis {
		case CommissionBasisEligibleJobGross, CommissionBasisOrganizationJobShare, CommissionBasisProviderEligibleBase:
		default:
			return fmt.Errorf("%w: commission basis is required", ErrValidation)
		}
		// An unrecognised rounding policy is refused rather than defaulted:
		// the policy is part of how the figure is reproduced, so a typo must
		// not quietly become half-up.
		if !ValidRoundingPolicy(rule.Config.Rounding) {
			return fmt.Errorf("%w: commission rounding policy %q is not supported", ErrValidation, rule.Config.Rounding)
		}
	case RulePerJob, RulePerShift:
		if err := ValidateMoney(rule.Config.AmountMinor, rule.Config.Currency); err != nil {
			return err
		}
	case RuleAllowance, RuleOvertime, RuleBonus, RuleDeduction:
		if rule.Config.Basis == "" && rule.Config.BasisMode == "" {
			return fmt.Errorf("%w: explicit calculation basis is required", ErrValidation)
		}
	default:
		return fmt.Errorf("%w: rule type is invalid", ErrValidation)
	}
	// Where the component is paid is part of what makes a rule valid: a plan
	// that asks for a salary to be settled per job is not a plan this service
	// can honour (CMP-027A).
	return ValidateSettlementStrategy(rule)
}

func RuleApplies(rule CompensationRule, service, role string, at time.Time) bool {
	if rule.Status != PlanActive {
		return false
	}
	if at.Before(rule.EffectiveFrom) || (rule.EffectiveTo != nil && !at.Before(*rule.EffectiveTo)) {
		return false
	}
	if !containsOrEmpty(rule.ApplicableServices, service) {
		return false
	}
	if !containsOrEmpty(rule.ApplicableRoles, role) {
		return false
	}
	return true
}

func CalculateRuleShare(rule CompensationRule, basisMinor int64, currency string) (MatchedRule, error) {
	if err := ValidateRule(rule); err != nil {
		return MatchedRule{}, err
	}
	out := MatchedRule{RuleID: rule.ID, RuleType: rule.RuleType}
	switch rule.RuleType {
	case RuleSalary:
		// Salary itself is paid through payroll, not per customer job.
		return out, nil
	case RulePerJob, RulePerShift:
		if rule.Config.Currency != currency {
			return MatchedRule{}, fmt.Errorf("%w: currency mismatch", ErrValidation)
		}
		out.AmountMinor = rule.Config.AmountMinor
		return out, nil
	case RuleCommission:
		share := (basisMinor*int64(rule.Config.RateBps) + 5000) / 10000
		out.AmountMinor = int64(math.Max(0, float64(share)))
		out.BasisMinor = basisMinor
		out.RateBps = rule.Config.RateBps
		return out, nil
	default:
		return out, nil
	}
}

func SnapshotJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

func OperatingModeValid(mode string) bool {
	return models.ValidOperatingMode(mode)
}

func containsOrEmpty(list []string, value string) bool {
	if len(list) == 0 {
		return true
	}
	value = strings.TrimSpace(value)
	for _, item := range list {
		if item == value || item == "*" {
			return true
		}
	}
	return false
}
