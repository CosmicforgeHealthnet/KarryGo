package apiresponse

import (
	"encoding/json"
	"testing"

	"cosmicforge/logistics/shared/go/providercompat"
)

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func bookingBody() map[string]any {
	return map[string]any{
		"id":                       "book-1",
		"status":                   "completed",
		"pickup_address":           "12 Awolowo Road",
		"fare_final_kobo":          450000,
		"own_earning_minor":        360000,
		"commission_rate_bps":      2000,
		"tips_kobo":                5000,
		"organization_share_minor": 90000,
		"platform_margin_minor":    12000,
		"nested": map[string]any{
			"customerFareMinor": 450000,
			"receiver_name":     "Ada",
		},
	}
}

// TAX-019B: an independent provider must not lose any legitimate financial
// information to a rule written for corporate privacy.
func TestProjectResponse_IndependentLosesNothing(t *testing.T) {
	out := asMap(t, ProjectResponse(bookingBody(), false, nil))
	for _, field := range []string{"fare_final_kobo", "own_earning_minor", "commission_rate_bps", "tips_kobo"} {
		if out[field] == nil {
			t.Fatalf("independent provider lost %s", field)
		}
	}
	nested := out["nested"].(map[string]any)
	if nested["customerFareMinor"] == nil {
		t.Fatal("independent provider lost nested customer fare")
	}
	// Organization and platform values still do not exist for an independent
	// provider, so they stay concealed.
	if out["organization_share_minor"] != nil || out["platform_margin_minor"] != nil {
		t.Fatal("organization/platform values must never be provider-visible")
	}
}

// A managed provider on a commission policy sees their own earning and
// commission, but not the customer fare.
func TestProjectResponse_ManagedAppliesTypedPolicy(t *testing.T) {
	policy := providercompat.FinancialVisibilityPolicy{
		ShowOwnEarning: true, ShowCommissionRate: true, ShowProviderShare: true, ShowPayroll: true,
	}
	out := asMap(t, ProjectResponse(bookingBody(), true, &policy))

	if out["own_earning_minor"] == nil {
		t.Fatal("own earning must stay visible under a commission policy")
	}
	if out["commission_rate_bps"] == nil {
		t.Fatal("commission rate must stay visible under a commission policy")
	}
	if out["fare_final_kobo"] != nil {
		t.Fatal("customer fare must be concealed under a commission policy")
	}
	if out["tips_kobo"] != nil {
		t.Fatal("tips must be concealed when the policy does not allow them")
	}
	nested := out["nested"].(map[string]any)
	if nested["customerFareMinor"] != nil {
		t.Fatal("nested customer fare must be concealed too")
	}
	// Non-financial fields are untouched.
	if out["pickup_address"] != "12 Awolowo Road" {
		t.Fatal("a non-financial field must not be altered")
	}
	if nested["receiver_name"] != "Ada" {
		t.Fatal("a nested non-financial field must not be altered")
	}
}

// No policy can authorize organization or platform commercial data.
func TestProjectResponse_OrganizationAndPlatformNeverVisible(t *testing.T) {
	permissive := providercompat.IndependentVisibilityPolicy()
	out := asMap(t, ProjectResponse(bookingBody(), true, &permissive))
	if out["organization_share_minor"] != nil {
		t.Fatal("organization share leaked under a fully permissive policy")
	}
	if out["platform_margin_minor"] != nil {
		t.Fatal("platform margin leaked under a fully permissive policy")
	}
}

// A managed provider with no resolved policy fails closed.
func TestProjectResponse_ManagedWithoutPolicyFailsClosed(t *testing.T) {
	out := asMap(t, ProjectResponse(bookingBody(), true, nil))
	for _, field := range []string{"fare_final_kobo", "own_earning_minor", "commission_rate_bps", "tips_kobo"} {
		if out[field] != nil {
			t.Fatalf("%s must be concealed when no policy is available", field)
		}
	}
}

// Defence-in-depth: a money-looking field the typed registry does not yet
// classify is still concealed from a managed provider, and still shown to an
// independent one.
func TestProjectResponse_UnclassifiedMoneyFieldFailsClosedForManaged(t *testing.T) {
	body := map[string]any{"weekly_incentive_margin_kobo": 4200, "distance_km": 12}

	managed := asMap(t, ProjectResponse(body, true, &providercompat.FinancialVisibilityPolicy{ShowOwnEarning: true}))
	if managed["weekly_incentive_margin_kobo"] != nil {
		t.Fatal("an unclassified money-looking field must fail closed for a managed provider")
	}
	if managed["distance_km"] == nil {
		t.Fatal("a non-financial field must survive")
	}

	independent := asMap(t, ProjectResponse(body, false, nil))
	if independent["weekly_incentive_margin_kobo"] == nil {
		t.Fatal("an independent provider must not lose an unclassified field to a spelling heuristic")
	}
}

// TAX-019A: the typed projection omits unauthorized fields entirely rather
// than emitting them as zero.
func TestProjectFinancials_OmitsRatherThanZeroes(t *testing.T) {
	inputs := FinancialInputs{
		CustomerFareMinor: 450000, OwnEarningMinor: 360000,
		CommissionRateBps: 2000, ProviderShareMinor: 360000, TipsMinor: 5000,
		Currency: "NGN",
	}

	independent := ProjectFinancials(inputs, false, nil)
	if independent.CustomerFareMinor == nil || *independent.CustomerFareMinor != 450000 {
		t.Fatal("independent provider must see the customer fare")
	}
	if independent.Mode != providercompat.ModeIndependent {
		t.Fatalf("mode = %q", independent.Mode)
	}

	policy := providercompat.FinancialVisibilityPolicy{ShowOwnEarning: true, ShowPayroll: true}
	managed := ProjectFinancials(inputs, true, &policy)
	if managed.CustomerFareMinor != nil {
		t.Fatal("customer fare must be absent, not zero")
	}
	if managed.OwnEarningMinor == nil || *managed.OwnEarningMinor != 360000 {
		t.Fatal("own earning must be present")
	}
	if managed.Mode != providercompat.ModeOrganizationManaged {
		t.Fatalf("mode = %q", managed.Mode)
	}

	raw, _ := json.Marshal(managed)
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	if _, present := decoded["customerFareMinor"]; present {
		t.Fatal("an unauthorized field must not appear in the JSON at all")
	}
}
