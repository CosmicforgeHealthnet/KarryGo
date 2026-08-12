package providerauthusecases

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	providerauthmodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/models"
	"cosmicforge/logistics/shared/go/providercompat"
)

func TestSection30HaulingLogin_LegacyFixtureIsExactWhenExtensionDisabled(t *testing.T) {
	result := TokenResult{
		AccessToken: "sanitized-access-token", RefreshToken: "sanitized-refresh-token", ExpiresIn: 3600,
		Provider: providerauthmodels.PublicProvider{
			ID: "provider_fixture", Phone: "+2348000000000", FirstName: "Fixture", LastName: "Provider",
			OnboardingStatus: "complete", Status: "active", Rating: 4.8, TotalTrips: 12,
			LocationState: "Lagos", LocationCity: "Ikeja", Language: "en", ServiceType: "hauling",
			OperationMode: "independent", DriverLicenseNumber: "SANITIZED", LicenseExpiryYear: "2030",
			LicenseExpiryDate: "2030-12-31", GovIDURL: "media://fixture/gov-id",
			DriverLicenseURL: "media://fixture/license", VehicleRegURL: "media://fixture/vehicle",
			GuarantorName: "Sanitized Guarantor", GuarantorPhone: "+2348000000001",
			EmergencyContactName: "Sanitized Contact", EmergencyContactPhone: "+2348000000002",
			EmergencyContactRelationship: "friend", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	gotRaw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	wantRaw, err := os.ReadFile("testdata/section30-independent-login-v1.json")
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

func TestSection30HaulingLogin_ManagedFieldsAreAdditive(t *testing.T) {
	result := TokenResult{
		ProviderMode:          providercompat.ModeOrganizationManaged,
		Organization:          &providercompat.Organization{ID: "org_fixture", DisplayName: "Fixture Logistics", Status: "ACTIVE"},
		Membership:            &providercompat.Membership{Status: "ACTIVE", Role: "EMPLOYEE", ServiceCategory: "HAULING"},
		AssignmentPolicy:      &providercompat.AssignmentPolicy{AssignmentMode: "OFFER", ProviderDeclineRequired: true},
		FieldVisibilityPolicy: "ORGANIZATION",
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"providerMode", "organization", "membership", "assignmentPolicy", "fieldVisibilityPolicy"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("managed login missing additive field %q", key)
		}
	}
}
