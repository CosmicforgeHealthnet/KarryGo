package authusecases

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"cosmicforge/logistics/shared/go/providercompat"
)

func TestSection30DispatchLogin_LegacyFixtureIsExactWhenExtensionDisabled(t *testing.T) {
	result := TokenResult{
		ProviderID: "provider_fixture", Role: "dispatch_provider", TokenType: "Bearer",
		AccessToken: "sanitized-access-token", RefreshToken: "sanitized-refresh-token", ExpiresInSeconds: 900,
	}
	assertJSONMatchesFixture(t, result, "testdata/section30-independent-login-v1.json")
}

func TestSection30DispatchLogin_ManagedFieldsAreAdditiveAndFinanceSafe(t *testing.T) {
	result := TokenResult{
		ProviderID: "provider_fixture", Role: "dispatch_provider", TokenType: "Bearer",
		AccessToken: "sanitized-access-token", RefreshToken: "sanitized-refresh-token", ExpiresInSeconds: 900,
	}
	applyTokenCompatibility(&result, providercompat.Summary{
		ProviderMode:          providercompat.ModeOrganizationManaged,
		Organization:          &providercompat.Organization{ID: "org_fixture", DisplayName: "Fixture Dispatch Co", Status: "ACTIVE"},
		Membership:            &providercompat.Membership{Status: "ACTIVE", Role: "EMPLOYEE", ServiceCategory: "DISPATCH"},
		AssignmentPolicy:      &providercompat.AssignmentPolicy{AssignmentMode: "OFFER", ProviderDeclineRequired: true},
		FieldVisibilityPolicy: "ORGANIZATION",
	}, true)
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{"providerMode", "organization", "membership", "assignmentPolicy", "fieldVisibilityPolicy"} {
		if !containsJSONKey(text, required) {
			t.Errorf("managed login is missing additive field %q: %s", required, text)
		}
	}
	for _, forbidden := range []string{"companyMargin", "companyWallet", "contractPrice", "organizationSettlement"} {
		if containsJSONKey(text, forbidden) {
			t.Errorf("managed login leaked forbidden field %q: %s", forbidden, text)
		}
	}
}

func assertJSONMatchesFixture(t *testing.T, value any, fixturePath string) {
	t.Helper()
	gotRaw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	wantRaw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var got, want map[string]any
	if err := json.Unmarshal(gotRaw, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("serialized contract differs from legacy fixture:\n got: %#v\nwant: %#v", got, want)
	}
}

func containsJSONKey(raw, key string) bool {
	return len(raw) >= len(key)+3 && json.Valid([]byte(raw)) &&
		stringContains(raw, `"`+key+`"`)
}

func stringContains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
