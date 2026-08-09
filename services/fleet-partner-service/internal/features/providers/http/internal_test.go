package http

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
)

func loadFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return decoded
}

func viewAsMap(t *testing.T, view membershipSummaryView) map[string]any {
	t.Helper()
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal view: %v", err)
	}
	return decoded
}

// TestMembershipSummaryView_IndependentMatchesGoldenFixture is Section 30
// WP17's golden-contract test for the INDEPENDENT-mode shape: an
// independent provider's summary is ONLY {"providerMode":"INDEPENDENT"} --
// no organization/membership/assignmentPolicy/fieldVisibilityPolicy key at
// all (proven via reflect.DeepEqual against the exact decoded fixture, not
// a subset check), which is what WP18/WP26 require: nothing about an
// independent provider's own contract may be perturbed by this feature
// existing.
func TestMembershipSummaryView_IndependentMatchesGoldenFixture(t *testing.T) {
	summary := usecases.MembershipSummary{ProviderMode: models.ProviderModeIndependent}
	got := viewAsMap(t, newMembershipSummaryView(summary))
	want := loadFixture(t, "membership_summary_independent.json")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("INDEPENDENT view does not match golden fixture:\n got:  %#v\n want: %#v", got, want)
	}
}

// TestMembershipSummaryView_OrganizationManagedMatchesGoldenFixture proves
// the full organization-managed shape -- every additive field WP20
// requires (organization id/displayName/status/branch, membership
// status/role/serviceCategory, assignmentPolicy assignmentMode/
// providerDeclineRequired, fieldVisibilityPolicy) is present with the
// expected real domain values (OFFER/ORGANIZATION are genuine
// AssignmentMode/VisibilityPolicy enum members, see migration 000007 --
// not invented placeholder strings).
func TestMembershipSummaryView_OrganizationManagedMatchesGoldenFixture(t *testing.T) {
	summary := usecases.MembershipSummary{
		ProviderMode: models.ProviderModeOrganizationManaged,
		Organization: &usecases.OrganizationSummary{
			ID:          "org_fixture0000000000000001",
			DisplayName: "Fixture Logistics Co",
			Status:      "ACTIVE",
			BranchID:    "brn_fixture0000000000000001",
			BranchName:  "Lagos Mainland Hub",
			BranchType:  "HUB",
		},
		MembershipStatus:        "ACTIVE",
		MembershipRole:          "EMPLOYEE",
		ServiceCategory:         "HAULING",
		AssignmentMode:          "OFFER",
		ProviderDeclineRequired: true,
		FieldVisibilityPolicy:   "ORGANIZATION",
	}
	got := viewAsMap(t, newMembershipSummaryView(summary))
	want := loadFixture(t, "membership_summary_organization_managed.json")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ORGANIZATION_MANAGED view does not match golden fixture:\n got:  %#v\n want: %#v", got, want)
	}
}

// TestMembershipSummaryView_OrganizationManagedFieldsNeverLeakIntoIndependent
// is the WP18/WP26 boundary-preservation regression guard as an explicit
// assertion (not just fixture equality): construct a summary with EVERY
// organization-managed field populated but ProviderMode left as
// INDEPENDENT (a defensive shape that should never occur from
// GetMembershipSummaryForProvider, but the view builder must not trust
// ProviderMode blindly propagate populated substructs either) and prove
// the serialized view still omits every one of them.
func TestMembershipSummaryView_OrganizationManagedFieldsNeverLeakIntoIndependent(t *testing.T) {
	summary := usecases.MembershipSummary{
		ProviderMode: models.ProviderModeIndependent, // deliberately mismatched with the populated fields below
		Organization: &usecases.OrganizationSummary{
			ID: "org_should_never_appear", DisplayName: "Should Never Appear Co", Status: "ACTIVE",
		},
		MembershipStatus:        "ACTIVE",
		MembershipRole:          "EMPLOYEE",
		ServiceCategory:         "HAULING",
		AssignmentMode:          "OFFER",
		ProviderDeclineRequired: true,
		FieldVisibilityPolicy:   "ORGANIZATION",
	}
	got := viewAsMap(t, newMembershipSummaryView(summary))
	for _, forbidden := range []string{"organization", "membership", "assignmentPolicy", "fieldVisibilityPolicy"} {
		if _, present := got[forbidden]; present {
			t.Errorf("independent-mode view leaked organization-managed field %q: %#v", forbidden, got)
		}
	}
	if len(got) != 1 || got["providerMode"] != "INDEPENDENT" {
		t.Errorf("independent-mode view should be exactly {providerMode: INDEPENDENT}, got %#v", got)
	}
}

// TestMembershipSummaryView_NoForbiddenFinancialOrCrossProviderFields is
// Section 30 WP25's field-visibility regression guard: enumerate every
// JSON field this response type can ever emit (via its struct tags,
// including nested types) and assert none of them is a customer contract
// price, company charge/margin/commission, org wallet balance/settlement
// value, or another provider's data -- the categories WP25 explicitly
// forbids from an organization-managed provider response.
func TestMembershipSummaryView_NoForbiddenFinancialOrCrossProviderFields(t *testing.T) {
	forbiddenSubstrings := []string{
		"price", "margin", "commission", "wallet", "balance", "settlement",
		"charge", "fee", "otherProvider", "customerContract",
	}
	for _, jsonTag := range collectJSONFieldNames(reflect.TypeOf(membershipSummaryView{})) {
		lower := strings.ToLower(jsonTag)
		for _, forbidden := range forbiddenSubstrings {
			if strings.Contains(lower, forbidden) {
				t.Errorf("membershipSummaryView (or a nested type) exposes field %q, which matches the forbidden category %q", jsonTag, forbidden)
			}
		}
	}
}

func collectJSONFieldNames(t reflect.Type) []string {
	var names []string
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return names
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			names = append(names, name)
		}
		fieldType := field.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct {
			names = append(names, collectJSONFieldNames(fieldType)...)
		}
	}
	return names
}
