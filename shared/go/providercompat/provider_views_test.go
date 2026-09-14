package providercompat_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/providercompat"
)

func jobInputs() providercompat.JobFinancialInputs {
	return providercompat.JobFinancialInputs{
		JobID: "job_1", SourceService: "TAXI", Currency: "NGN",
		CustomerFareMinor: 10000, OwnEarningMinor: 1500, ProviderShareMinor: 1500,
		CommissionRateBps: 1500, TipsMinor: 200, AllowancesMinor: 300, BonusesMinor: 400,
		OrganizationShareMinor: 7500, PlatformShareMinor: 1000,
	}
}

func fieldFor(view providercompat.ProviderJobFinancialView, class providercompat.FinancialFieldClass) (providercompat.MoneyField, bool) {
	for _, field := range view.Fields {
		if field.Class == class {
			return field, true
		}
	}
	return providercompat.MoneyField{}, false
}

// VIS-007. The organization's and the platform's economics are not in the view
// at all -- not withheld behind a flag, not present as zero. They are inputs
// the builder is given precisely so its refusal to emit them is testable.
func TestAJobViewNeverCarriesOrganizationOrPlatformEconomics(t *testing.T) {
	for _, policy := range []providercompat.FinancialVisibilityPolicy{
		providercompat.IndependentVisibilityPolicy(),
		providercompat.ConservativeVisibilityPolicy(),
		{ShowCustomerFare: true, ShowOwnEarning: true, ShowCommissionRate: true,
			ShowProviderShare: true, ShowPayroll: true, ShowTips: true,
			ShowAllowances: true, ShowBonuses: true},
	} {
		view := providercompat.BuildProviderJobFinancialView(policy, jobInputs())
		for _, field := range view.Fields {
			if field.Class.AlwaysConcealed() {
				t.Fatalf("view carries a concealed class %s", field.Class)
			}
		}
		raw, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		// 7500 and 1000 are the organization's and the platform's shares. They
		// must not appear anywhere in the serialized response, under any name.
		for _, forbidden := range []string{"7500", "organization", "platform"} {
			if strings.Contains(strings.ToLower(string(raw)), forbidden) {
				t.Fatalf("serialized job view leaks %q: %s", forbidden, raw)
			}
		}
	}
}

// A withheld field is present and marked invisible rather than absent, so a
// client can say "your organization does not share this" -- and so a test can
// tell a withheld field from a forgotten one.
func TestAWithheldFieldIsMarkedRatherThanMissing(t *testing.T) {
	view := providercompat.BuildProviderJobFinancialView(
		providercompat.ConservativeVisibilityPolicy(), jobInputs())

	fare, present := fieldFor(view, providercompat.ClassCustomerFare)
	if !present {
		t.Fatal("the customer fare field is missing entirely rather than withheld")
	}
	if fare.Visible {
		t.Fatal("the conservative policy showed the customer fare")
	}
	if fare.AmountMinor != 0 {
		t.Fatalf("a withheld field carried the amount %d", fare.AmountMinor)
	}
}

// An independent provider keeps everything they legitimately saw.
func TestAnIndependentProviderStillSeesTheirOwnFigures(t *testing.T) {
	view := providercompat.BuildProviderJobFinancialView(
		providercompat.IndependentVisibilityPolicy(), jobInputs())

	for _, class := range []providercompat.FinancialFieldClass{
		providercompat.ClassCustomerFare, providercompat.ClassOwnEarning,
		providercompat.ClassProviderShare, providercompat.ClassTips,
	} {
		field, present := fieldFor(view, class)
		if !present || !field.Visible {
			t.Fatalf("an independent provider cannot see %s", class)
		}
	}
	earning, _ := fieldFor(view, providercompat.ClassOwnEarning)
	if earning.AmountMinor != 1500 {
		t.Fatalf("own earning = %d, want 1500", earning.AmountMinor)
	}
}

// The conservative default shows a managed provider nothing but their own
// payroll.
func TestTheConservativeDefaultShowsOnlyPayroll(t *testing.T) {
	view := providercompat.BuildProviderJobFinancialView(
		providercompat.ConservativeVisibilityPolicy(), jobInputs())
	for _, field := range view.Fields {
		if field.Visible {
			t.Fatalf("the conservative policy showed %s on a job view", field.Class)
		}
	}
}

// VIS-015. The explanation is typed, and a component the policy withholds is
// omitted rather than shown with a blank amount -- "you earned something, we
// won't say what" is worse than silence, and a zero would be a lie.
func TestTheExplanationOmitsWhatPolicyWithholds(t *testing.T) {
	inputs := []providercompat.ExplanationInput{
		{Component: "COMMISSION", Class: providercompat.ClassCommissionRate, RateBps: 1500, AmountMinor: 1500, Currency: "NGN"},
		{Component: "OWN_EARNING", Class: providercompat.ClassOwnEarning, AmountMinor: 1500, Currency: "NGN"},
		{Component: "ORGANIZATION_SHARE", Class: providercompat.ClassOrganizationOnly, AmountMinor: 7500, Currency: "NGN"},
		{Component: "PLATFORM_SHARE", Class: providercompat.ClassPlatformOnly, AmountMinor: 1000, Currency: "NGN"},
	}
	lines := providercompat.BuildCompensationExplanation(
		providercompat.FinancialVisibilityPolicy{ShowOwnEarning: true}, inputs)

	if len(lines) != 1 || lines[0].Component != "OWN_EARNING" {
		t.Fatalf("explanation = %+v, want only the provider's own earning", lines)
	}
	raw, err := json.Marshal(lines)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"7500", "1000", "ORGANIZATION", "PLATFORM"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("explanation leaks %q: %s", forbidden, raw)
		}
	}
}

// Concealed classes are unreachable even if a policy somehow claimed to permit
// them.
func TestNoPolicyCanUnlockOrganizationOrPlatformExplanationLines(t *testing.T) {
	everything := providercompat.FinancialVisibilityPolicy{
		ShowCustomerFare: true, ShowOwnEarning: true, ShowCommissionRate: true,
		ShowProviderShare: true, ShowPayroll: true, ShowTips: true,
		ShowAllowances: true, ShowBonuses: true,
	}
	lines := providercompat.BuildCompensationExplanation(everything, []providercompat.ExplanationInput{
		{Component: "ORGANIZATION_SHARE", Class: providercompat.ClassOrganizationOnly, AmountMinor: 7500},
		{Component: "PLATFORM_SHARE", Class: providercompat.ClassPlatformOnly, AmountMinor: 1000},
	})
	if len(lines) != 0 {
		t.Fatalf("a permissive policy unlocked %d concealed line(s): %+v", len(lines), lines)
	}
}

func payrollInput(membershipID string) providercompat.PayrollViewInput {
	start := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC)
	return providercompat.PayrollViewInput{
		MembershipID: membershipID, PeriodStart: start, PeriodEnd: end,
		Currency: "NGN", NetMinor: 285000,
		Components: []providercompat.ExplanationInput{
			{Component: "SALARY", Class: providercompat.ClassPayroll, AmountMinor: 300000, Currency: "NGN"},
			{Component: "DEDUCTION", Class: providercompat.ClassPayroll, AmountMinor: 15000, Currency: "NGN", Reason: "Equipment"},
			{Component: "ORGANIZATION_MARGIN", Class: providercompat.ClassOrganizationOnly, AmountMinor: 90000},
		},
	}
}

// VIS-014. A provider sees their own payroll.
func TestAProviderSeesTheirOwnPayroll(t *testing.T) {
	view := providercompat.BuildProviderPayrollView(
		providercompat.ConservativeVisibilityPolicy(), "mem_1", payrollInput("mem_1"))

	if !view.Visible || view.NetMinor != 285000 {
		t.Fatalf("view = %+v, want the provider's own 285000", view)
	}
	if len(view.Lines) != 2 {
		t.Fatalf("%d payroll lines, want the two payroll-class ones", len(view.Lines))
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "90000") || strings.Contains(strings.ToUpper(string(raw)), "ORGANIZATION_MARGIN") {
		t.Fatalf("payroll view leaks organization economics: %s", raw)
	}
}

// And nobody else's. A read model that only refuses when someone remembers to
// check it will eventually be read by something that forgot.
func TestAProviderCannotSeeAnotherProvidersPayroll(t *testing.T) {
	for _, requester := range []string{"mem_2", "", "   ", "MEM_1"} {
		view := providercompat.BuildProviderPayrollView(
			providercompat.ConservativeVisibilityPolicy(), requester, payrollInput("mem_1"))
		if view.Visible {
			t.Fatalf("requester %q saw mem_1's payroll", requester)
		}
		if view.NetMinor != 0 || len(view.Lines) != 0 {
			t.Fatalf("requester %q got figures anyway: %+v", requester, view)
		}
		if view.MembershipID != "" {
			t.Fatalf("requester %q learned whose payroll it was: %+v", requester, view)
		}
	}
}

// A policy that withholds payroll withholds the payroll view too.
func TestPayrollIsWithheldWhenPolicyForbidsIt(t *testing.T) {
	view := providercompat.BuildProviderPayrollView(
		providercompat.FinancialVisibilityPolicy{ShowOwnEarning: true}, "mem_1", payrollInput("mem_1"))
	if view.Visible || view.NetMinor != 0 {
		t.Fatalf("payroll was shown despite policy: %+v", view)
	}
}

// The view is a constructed shape with no passthrough, so a service holding
// extra figures cannot accidentally widen it.
func TestTheJobViewHasNoPassthroughForUnknownFields(t *testing.T) {
	view := providercompat.BuildProviderJobFinancialView(
		providercompat.IndependentVisibilityPolicy(), jobInputs())
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatal(err)
	}
	for key := range roundTrip {
		switch key {
		case "jobId", "sourceService", "currency", "fields", "explanation":
		default:
			t.Fatalf("the job view carries an unexpected key %q", key)
		}
	}
}
