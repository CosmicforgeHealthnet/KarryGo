package apiresponse

import (
	"encoding/json"
	"testing"

	"cosmicforge/logistics/shared/go/providercompat"
)

func projected(t *testing.T, data any, conceal bool, policy *providercompat.FinancialVisibilityPolicy) map[string]any {
	t.Helper()
	raw, err := json.Marshal(providerViewWithPolicy(data, conceal, policy))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func missionBody() map[string]any {
	return map[string]any{
		"id":                       "trip-1",
		"status":                   "assigned",
		"dropoff_address":          "5 Herbert Macaulay Way",
		"fare_amount":              250000,
		"own_earning_minor":        200000,
		"commission_rate_bps":      2000,
		"tips_kobo":                3000,
		"organization_share_minor": 50000,
		"platform_margin_minor":    9000,
		"settlement_allocation":    17000,
		"nested": map[string]any{
			"customerFareMinor": 250000,
			"distance_km":       7.5,
		},
	}
}

// DSP-011..013: typed classification is the PRIMARY protection. Under a
// commission policy the rider sees their own earning and commission but not
// the customer fare, at every nesting depth.
func TestTypedPolicyIsPrimaryProtection(t *testing.T) {
	policy := providercompat.FinancialVisibilityPolicy{
		ShowOwnEarning: true, ShowCommissionRate: true, ShowProviderShare: true, ShowPayroll: true,
	}
	out := projected(t, missionBody(), true, &policy)

	if out["own_earning_minor"] == nil || out["commission_rate_bps"] == nil {
		t.Fatal("policy-authorized fields must stay visible")
	}
	if out["fare_amount"] != nil {
		t.Fatal("customer fare must be concealed under a commission policy")
	}
	if out["tips_kobo"] != nil {
		t.Fatal("tips must be concealed when the policy does not allow them")
	}
	nested := out["nested"].(map[string]any)
	if nested["customerFareMinor"] != nil {
		t.Fatal("nested customer fare must be concealed too")
	}
	if nested["distance_km"] == nil {
		t.Fatal("a nested non-financial field must survive")
	}
	if out["dropoff_address"] != "5 Herbert Macaulay Way" {
		t.Fatal("a non-financial field must not be altered")
	}
}

// No policy may authorize organization or platform commercial data.
func TestOrganizationAndPlatformNeverVisible(t *testing.T) {
	permissive := providercompat.IndependentVisibilityPolicy()
	out := projected(t, missionBody(), true, &permissive)
	for _, field := range []string{"organization_share_minor", "platform_margin_minor", "settlement_allocation"} {
		if out[field] != nil {
			t.Fatalf("%s leaked under a fully permissive policy", field)
		}
	}
}

// A managed rider with no resolved policy fails closed.
func TestManagedWithoutPolicyFailsClosed(t *testing.T) {
	out := projected(t, missionBody(), true, nil)
	for _, field := range []string{"fare_amount", "own_earning_minor", "commission_rate_bps", "tips_kobo"} {
		if out[field] != nil {
			t.Fatalf("%s must be concealed when no policy is available", field)
		}
	}
}

// An independent rider loses nothing: the name heuristic is defence-in-depth
// only and never runs for them.
func TestIndependentRiderLosesNothing(t *testing.T) {
	out := projected(t, missionBody(), false, nil)
	for _, field := range []string{"fare_amount", "own_earning_minor", "commission_rate_bps", "tips_kobo"} {
		if out[field] == nil {
			t.Fatalf("independent rider lost %s", field)
		}
	}
	if out["nested"].(map[string]any)["customerFareMinor"] == nil {
		t.Fatal("independent rider lost a nested fare")
	}
}

// Defence-in-depth still catches a money-looking field the typed registry
// does not classify, but only for a managed rider.
func TestUnclassifiedMoneyFieldIsDefenceInDepth(t *testing.T) {
	body := map[string]any{"weekly_incentive_margin_kobo": 4200, "distance_km": 7.5}

	managed := projected(t, body, true, &providercompat.FinancialVisibilityPolicy{ShowOwnEarning: true})
	if managed["weekly_incentive_margin_kobo"] != nil {
		t.Fatal("an unclassified money-looking field must fail closed for a managed rider")
	}
	if managed["distance_km"] == nil {
		t.Fatal("a non-financial field must survive")
	}

	independent := projected(t, body, false, nil)
	if independent["weekly_incentive_margin_kobo"] == nil {
		t.Fatal("an independent rider must not lose a field to a spelling heuristic")
	}
}
