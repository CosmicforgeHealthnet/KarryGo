package providercompat

// Section 14: the single provider financial-visibility policy.
//
// One policy governs Taxi, Dispatch and Hauling. These tests pin the
// properties that make that claim meaningful: the policy has exactly the
// fields the specification names, the two never-visible classes are never
// visible under ANY policy, and an unclassified money field is withheld rather
// than guessed at.

import (
	"reflect"
	"testing"
)

// TestPolicyCarriesExactlyTheRequiredFields is VIS-002.
//
// An extra field would be an unjustified visibility lever; a missing one would
// mean a service had to invent its own rule, which is what VIS-001 forbids.
func TestPolicyCarriesExactlyTheRequiredFields(t *testing.T) {
	required := map[string]bool{
		"ShowCustomerFare": true, "ShowOwnEarning": true, "ShowCommissionRate": true,
		"ShowProviderShare": true, "ShowPayroll": true, "ShowTips": true,
		"ShowAllowances": true, "ShowBonuses": true,
	}
	policyType := reflect.TypeOf(FinancialVisibilityPolicy{})
	seen := map[string]bool{}
	for i := 0; i < policyType.NumField(); i++ {
		name := policyType.Field(i).Name
		if !required[name] {
			t.Errorf("policy carries an unrequired field %q: every visibility lever needs explicit justification", name)
		}
		seen[name] = true
	}
	for name := range required {
		if !seen[name] {
			t.Errorf("policy is missing required field %q", name)
		}
	}
}

// TestOrganizationAndPlatformDataAreNeverVisible is VIS-003 and VIS-004.
//
// Not "false by default" -- unreachable. There is no combination of policy
// flags that reveals the platform's margin or the organization's internal
// commercial position to a provider.
func TestOrganizationAndPlatformDataAreNeverVisible(t *testing.T) {
	permissive := FinancialVisibilityPolicy{
		ShowCustomerFare: true, ShowOwnEarning: true, ShowCommissionRate: true,
		ShowProviderShare: true, ShowPayroll: true, ShowTips: true,
		ShowAllowances: true, ShowBonuses: true,
	}
	for _, class := range []FinancialFieldClass{ClassOrganizationOnly, ClassPlatformOnly} {
		if !class.AlwaysConcealed() {
			t.Fatalf("%s is not always concealed", class)
		}
		if permissive.Allows(class) {
			t.Fatalf("%s became visible under a fully permissive policy", class)
		}
		if (FinancialVisibilityPolicy{}).Allows(class) {
			t.Fatalf("%s visible under an empty policy", class)
		}
		if IndependentVisibilityPolicy().Allows(class) {
			t.Fatalf("%s visible to an independent provider", class)
		}
	}
}

// TestSalariedDefaultPolicyIsConservative is VIS-005.
//
// The zero value is what a service falls back to when it has no policy. For a
// managed provider that fallback must show nothing except what an explicit
// policy turns on.
func TestSalariedDefaultPolicyIsConservative(t *testing.T) {
	var conservative FinancialVisibilityPolicy
	for _, class := range []FinancialFieldClass{
		ClassCustomerFare, ClassOwnEarning, ClassCommissionRate, ClassProviderShare,
		ClassTips, ClassAllowances, ClassBonuses,
	} {
		if conservative.Allows(class) {
			t.Fatalf("%s visible under the default policy: a salaried provider would see per-job money", class)
		}
	}
	// Payroll is the one thing a salaried provider legitimately sees, and it
	// is still opt-in rather than assumed.
	if !ConservativeVisibilityPolicy().Allows(ClassPayroll) {
		t.Fatal("the fail-closed policy must still show the provider their own payroll")
	}
	if conservative.Allows(ClassPayroll) {
		t.Fatal("payroll must not be visible without being authorized")
	}
}

// TestCommissionPolicyExposesOnlyOwnFinance is VIS-006.
func TestCommissionPolicyExposesOnlyOwnFinance(t *testing.T) {
	commission := FinancialVisibilityPolicy{
		ShowOwnEarning: true, ShowCommissionRate: true, ShowProviderShare: true,
	}
	for _, class := range []FinancialFieldClass{ClassOwnEarning, ClassCommissionRate, ClassProviderShare} {
		if !commission.Allows(class) {
			t.Fatalf("%s hidden from a commission provider who is authorized to see it", class)
		}
	}
	for _, class := range []FinancialFieldClass{ClassOrganizationOnly, ClassPlatformOnly} {
		if commission.Allows(class) {
			t.Fatalf("%s leaked to a commission provider", class)
		}
	}
}

// TestUnclassifiedMoneyFieldIsWithheld is VIS-009's fail-closed corollary.
//
// A money field nobody classified is a field nobody decided a provider may
// see. Withholding it is recoverable; leaking it is not.
func TestUnclassifiedMoneyFieldIsWithheld(t *testing.T) {
	if _, classified := ClassifyFinancialField("some_new_settlement_margin_minor"); classified {
		t.Fatal("an unknown field must not classify")
	}
	// And an unclassified field is not visible to anybody, managed or not.
	visible, classified := FieldVisible("some_new_settlement_margin_minor", true, nil)
	if visible || classified {
		t.Fatal("an unclassified money field must be withheld from a managed provider")
	}
	if visible, _ = FieldVisible("some_new_settlement_margin_minor", false, nil); visible {
		t.Fatal("an unclassified money field must be withheld from an independent provider too")
	}

	// FIN-29 / VIS-008: an independent provider keeps the visibility they
	// legitimately had.
	if visible, classified = FieldVisible("customer_fare_minor", false, nil); !visible || !classified {
		t.Fatal("an independent provider lost sight of the customer fare")
	}
	// VIS-005: the same field is withheld from a managed provider with no
	// established policy.
	if visible, _ = FieldVisible("customer_fare_minor", true, nil); visible {
		t.Fatal("a managed provider with no established policy saw the customer fare")
	}
}
