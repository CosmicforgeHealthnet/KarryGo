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

func haulJob() map[string]any {
	return map[string]any{
		"id":                       "book-1",
		"status":                   "completed",
		"pickup_address":           "12 Apapa Wharf Road",
		"fare_final_kobo":          750000,
		"own_earning_minor":        600000,
		"commission_rate_bps":      2000,
		"tips_kobo":                5000,
		"organization_share_minor": 150000,
		"platform_margin_minor":    20000,
		"settlement_allocation":    17000,
		"nested": map[string]any{
			"customerFareMinor": 750000,
			"distance_km":       42.5,
		},
	}
}

// HLG-022: a salaried corporate driver must not receive the customer fare,
// organization revenue, platform share or internal settlement totals.
func TestSalariedManagedProviderSeesNoCommercialData(t *testing.T) {
	// The conservative policy is what a salaried membership resolves to:
	// payroll context only.
	salaried := providercompat.ConservativeVisibilityPolicy()
	out := asMap(t, ProjectResponse(haulJob(), true, &salaried))

	for _, field := range []string{
		"fare_final_kobo", "own_earning_minor", "commission_rate_bps", "tips_kobo",
		"organization_share_minor", "platform_margin_minor", "settlement_allocation",
	} {
		if out[field] != nil {
			t.Fatalf("%s must be concealed from a salaried corporate driver", field)
		}
	}
	if out["nested"].(map[string]any)["customerFareMinor"] != nil {
		t.Fatal("nested customer fare must be concealed too")
	}
	// Operational information is untouched.
	if out["pickup_address"] != "12 Apapa Wharf Road" {
		t.Fatal("a non-financial field must not be altered")
	}
	if out["nested"].(map[string]any)["distance_km"] == nil {
		t.Fatal("a nested non-financial field must survive")
	}
}

// HLG-023: a commission/per-job driver sees their own share and rate, but not
// the full organization/platform economics.
func TestCommissionProviderSeesOnlyTheirOwnShare(t *testing.T) {
	policy := providercompat.FinancialVisibilityPolicy{
		ShowOwnEarning: true, ShowCommissionRate: true, ShowProviderShare: true, ShowPayroll: true,
	}
	out := asMap(t, ProjectResponse(haulJob(), true, &policy))

	if out["own_earning_minor"] == nil || out["commission_rate_bps"] == nil {
		t.Fatal("own earning and commission rate must stay visible under a commission policy")
	}
	if out["fare_final_kobo"] != nil {
		t.Fatal("the customer fare must be concealed under a commission policy")
	}
	for _, field := range []string{"organization_share_minor", "platform_margin_minor", "settlement_allocation"} {
		if out[field] != nil {
			t.Fatalf("%s must never be provider-visible", field)
		}
	}
}

// No policy can authorize organization or platform commercial data.
func TestOrganizationAndPlatformNeverVisible(t *testing.T) {
	permissive := providercompat.IndependentVisibilityPolicy()
	out := asMap(t, ProjectResponse(haulJob(), true, &permissive))
	for _, field := range []string{"organization_share_minor", "platform_margin_minor", "settlement_allocation"} {
		if out[field] != nil {
			t.Fatalf("%s leaked under a fully permissive policy", field)
		}
	}
}

// A managed driver with no resolved policy fails closed.
func TestManagedWithoutPolicyFailsClosed(t *testing.T) {
	out := asMap(t, ProjectResponse(haulJob(), true, nil))
	for _, field := range []string{"fare_final_kobo", "own_earning_minor", "commission_rate_bps", "tips_kobo"} {
		if out[field] != nil {
			t.Fatalf("%s must be concealed when no policy is available", field)
		}
	}
}

// HLG-024: an independent hauling provider must not lose any legitimate
// financial information to a rule written for corporate privacy.
func TestIndependentProviderLosesNothing(t *testing.T) {
	out := asMap(t, ProjectResponse(haulJob(), false, nil))
	for _, field := range []string{"fare_final_kobo", "own_earning_minor", "commission_rate_bps", "tips_kobo"} {
		if out[field] == nil {
			t.Fatalf("independent provider lost %s", field)
		}
	}
	if out["nested"].(map[string]any)["customerFareMinor"] == nil {
		t.Fatal("independent provider lost the nested fare")
	}
	// Organization/platform values still do not exist for an independent
	// provider, so they stay concealed.
	if out["organization_share_minor"] != nil || out["platform_margin_minor"] != nil {
		t.Fatal("organization/platform values must never be provider-visible")
	}
}

// Defence-in-depth catches a money-looking field the typed registry does not
// classify -- but only for a managed provider.
func TestUnclassifiedMoneyFieldIsDefenceInDepthOnly(t *testing.T) {
	body := map[string]any{"weekly_incentive_margin_kobo": 4200, "distance_km": 42.5}

	managed := asMap(t, ProjectResponse(body, true, &providercompat.FinancialVisibilityPolicy{ShowOwnEarning: true}))
	if managed["weekly_incentive_margin_kobo"] != nil {
		t.Fatal("an unclassified money-looking field must fail closed for a managed provider")
	}
	if managed["distance_km"] == nil {
		t.Fatal("a non-financial field must survive")
	}

	independent := asMap(t, ProjectResponse(body, false, nil))
	if independent["weekly_incentive_margin_kobo"] == nil {
		t.Fatal("an independent provider must not lose a field to a spelling heuristic")
	}
}

// ProjectJSON operates on an encoded body and leaves untouched amounts
// byte-identical, so a response this projection never intended to change is
// not silently reformatted.
func TestProjectJSONPreservesUntouchedNumbers(t *testing.T) {
	raw := []byte(`{"distance_km":42.50,"weight_grams":1200000,"fare_final_kobo":750000}`)
	policy := providercompat.FinancialVisibilityPolicy{ShowOwnEarning: true}
	out := ProjectJSON(raw, true, &policy)

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("projected body is not JSON: %v (%s)", err, out)
	}
	if decoded["fare_final_kobo"] != nil {
		t.Fatal("the customer fare must be concealed")
	}
	if string(out) == "" {
		t.Fatal("the projected body must not be empty")
	}
	// Non-financial numbers keep their exact encoding.
	if !containsAll(string(out), []string{"42.50", "1200000"}) {
		t.Fatalf("untouched numbers were reformatted: %s", out)
	}
}

// A non-JSON body passes through unchanged rather than being mangled.
func TestProjectJSONLeavesNonJSONAlone(t *testing.T) {
	raw := []byte("not json at all")
	if got := ProjectJSON(raw, true, nil); string(got) != string(raw) {
		t.Fatalf("non-JSON body was altered: %s", got)
	}
	if got := ProjectJSON(nil, true, nil); len(got) != 0 {
		t.Fatal("an empty body must stay empty")
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		found := false
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
