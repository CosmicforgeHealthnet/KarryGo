package fleetpartner

import (
	"testing"
	"time"
)

func TestValidationPrimitives(t *testing.T) {
	if err := ValidateMoney(1200, "NGN"); err != nil {
		t.Fatalf("valid money rejected: %v", err)
	}
	if err := ValidateMoney(1200, "ngn"); err == nil {
		t.Fatal("lowercase currency accepted")
	}
	if err := ValidateBPS(10000); err != nil {
		t.Fatalf("10000 bps rejected: %v", err)
	}
	if err := ValidateBPS(10001); err == nil {
		t.Fatal("bps above 10000 accepted")
	}
	if err := ValidateUTC("completedAt", time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("UTC timestamp rejected: %v", err)
	}
	if err := ValidateUTC("completedAt", time.Date(2026, 9, 14, 10, 0, 0, 0, time.FixedZone("WAT", 3600))); err == nil {
		t.Fatal("non-UTC timestamp accepted")
	}
}

func TestRuleSemantics(t *testing.T) {
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	salary := CompensationRule{
		ID: "r1", RuleType: RuleSalary, Status: PlanActive,
		ApplicableRoles: []string{"EMPLOYEE"}, ApplicableServices: []string{ServiceDispatch},
		EffectiveFrom: now.Add(-time.Hour),
		Config:        RuleConfig{AmountMinor: 500000, Currency: "NGN", Cadence: CadenceMonthly},
	}
	if !RuleApplies(salary, ServiceDispatch, "EMPLOYEE", now) {
		t.Fatal("salary rule did not apply to matching role/service")
	}
	if RuleApplies(salary, ServiceTaxi, "EMPLOYEE", now) {
		t.Fatal("dispatch-only rule applied to taxi")
	}
	matched, err := CalculateRuleShare(salary, 10000, "NGN")
	if err != nil {
		t.Fatalf("salary calculation failed: %v", err)
	}
	if matched.AmountMinor != 0 {
		t.Fatalf("salary produced per-job share %d", matched.AmountMinor)
	}

	commission := CompensationRule{
		ID: "r2", RuleType: RuleCommission, Status: PlanActive, EffectiveFrom: now.Add(-time.Hour),
		Config: RuleConfig{RateBps: 1500, Basis: CommissionBasisEligibleJobGross},
	}
	matched, err = CalculateRuleShare(commission, 10001, "NGN")
	if err != nil {
		t.Fatalf("commission calculation failed: %v", err)
	}
	if matched.AmountMinor != 1500 {
		t.Fatalf("commission rounding = %d, want 1500", matched.AmountMinor)
	}

	perJob := CompensationRule{
		ID: "r3", RuleType: RulePerJob, Status: PlanActive, EffectiveFrom: now.Add(-time.Hour),
		Config: RuleConfig{AmountMinor: 750, Currency: "NGN"},
	}
	matched, err = CalculateRuleShare(perJob, 9999, "NGN")
	if err != nil {
		t.Fatalf("per-job calculation failed: %v", err)
	}
	if matched.AmountMinor != 750 {
		t.Fatalf("per-job amount = %d, want 750", matched.AmountMinor)
	}
}

func TestExplicitBasisRequired(t *testing.T) {
	commission := CompensationRule{RuleType: RuleCommission, Status: PlanActive, Config: RuleConfig{RateBps: 1000}}
	if err := ValidateRule(commission); err == nil {
		t.Fatal("commission without basis accepted")
	}
	allowance := CompensationRule{RuleType: RuleAllowance, Status: PlanActive}
	if err := ValidateRule(allowance); err == nil {
		t.Fatal("allowance without explicit basis accepted")
	}
}

func TestVisibilityPolicies(t *testing.T) {
	salary := ConservativeVisibilityPolicy()
	if salary.ShowCustomerFare || salary.ShowOwnEarning || salary.ShowProviderShare || !salary.ShowPayroll {
		t.Fatalf("salary default visibility is not conservative: %+v", salary)
	}
	commission := CommissionVisibilityPolicy()
	if !commission.ShowOwnEarning || !commission.ShowCommissionRate || !commission.ShowProviderShare || !commission.ShowPayroll {
		t.Fatalf("commission visibility missing expected provider fields: %+v", commission)
	}
}

func TestCallerOwnsSource(t *testing.T) {
	if !callerOwnsSource("driver-dispatch-delivery-service", ServiceDispatch) {
		t.Fatal("dispatch caller rejected for dispatch source")
	}
	if callerOwnsSource("driver-dispatch-delivery-service", ServiceTaxi) {
		t.Fatal("dispatch caller accepted for taxi source")
	}
}

func TestOperatingModes(t *testing.T) {
	for _, mode := range []string{"CENTRALIZED", "DELEGATED", "HYBRID"} {
		if !OperatingModeValid(mode) {
			t.Fatalf("valid mode rejected: %s", mode)
		}
	}
	if OperatingModeValid("AUTO") {
		t.Fatal("invalid mode accepted")
	}
}
