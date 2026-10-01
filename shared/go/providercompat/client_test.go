package providercompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestClientReadsSignedOrganizationManagedSummary(t *testing.T) {
	const caller = "driver-hauling-service"
	secret := []byte("section30-test-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{caller: secret}, time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/fleet-partner/v1/providers/provider_fixture/services/driver-hauling-service/membership-summary" {
			t.Errorf("incorrect membership route: %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if _, err := verifier.VerifyRequest(r); err != nil {
			t.Fatalf("verify signed request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(Summary{
			ProviderMode:          ModeOrganizationManaged,
			Organization:          &Organization{ID: "org_fixture", DisplayName: "Fixture Logistics", Status: "ACTIVE"},
			Membership:            &Membership{Status: "ACTIVE", Role: "EMPLOYEE", ServiceCategory: "HAULING"},
			AssignmentPolicy:      &AssignmentPolicy{AssignmentMode: "OFFER", ProviderDeclineRequired: true},
			FieldVisibilityPolicy: "ORGANIZATION",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, caller, secret, time.Second)
	got, err := client.ReadMembershipSummary(context.Background(), caller, "provider_fixture")
	if err != nil {
		t.Fatalf("ReadMembershipSummary: %v", err)
	}
	if got.ProviderMode != ModeOrganizationManaged || got.Organization == nil || got.Organization.ID != "org_fixture" {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestClientRejectsIncompleteManagedSummaryAndStripsIndependentLeaks(t *testing.T) {
	for _, tc := range []struct {
		name      string
		response  Summary
		wantError bool
	}{
		{name: "incomplete managed", response: Summary{ProviderMode: ModeOrganizationManaged}, wantError: true},
		{name: "independent leak stripped", response: Summary{
			ProviderMode: ModeIndependent,
			Organization: &Organization{ID: "must_not_leak"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(tc.response)
			}))
			defer server.Close()
			client := NewClient(server.URL, "caller", []byte("secret"), time.Second)
			got, err := client.ReadMembershipSummary(context.Background(), "service", "provider")
			if tc.wantError != (err != nil) {
				t.Fatalf("error = %v, wantError=%t", err, tc.wantError)
			}
			if !tc.wantError && got.Organization != nil {
				t.Fatalf("independent response retained organization data: %+v", got)
			}
		})
	}
}

func TestClientReadsSignedCorporateContext(t *testing.T) {
	const caller = "driver-taxi-service"
	secret := []byte("section8-test-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{caller: secret}, time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/fleet-partner/v1/providers/provider_fixture/corporate-context" {
			t.Errorf("incorrect corporate-context route: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("service") != "TAXI" {
			t.Errorf("service query = %q, want TAXI", r.URL.Query().Get("service"))
		}
		if _, err := verifier.VerifyRequest(r); err != nil {
			t.Fatalf("verify signed request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": CorporateContext{
			ProviderID:         "provider_fixture",
			IsManagedProvider:  true,
			MembershipID:       "membership_fixture",
			OrganizationID:     "org_fixture",
			MembershipStatus:   "ACTIVE",
			OrganizationRole:   "DRIVER",
			OperatingMode:      "HYBRID",
			ServiceEligibility: map[string]bool{"TAXI": true, "DISPATCH": false},
			FinancialVisibilityPolicy: FinancialVisibilityPolicy{
				ShowOwnEarning: true, ShowProviderShare: true,
			},
			Version: 7,
		}})
	}))
	defer server.Close()

	client := NewClient(server.URL, caller, secret, time.Second)
	got, err := client.ReadCorporateContext(context.Background(), "TAXI", "provider_fixture")
	if err != nil {
		t.Fatalf("ReadCorporateContext: %v", err)
	}
	if !got.IsManagedProvider || got.MembershipID != "membership_fixture" || !got.ServiceEligibility["TAXI"] {
		t.Fatalf("unexpected corporate context: %+v", got)
	}
	if !got.FinancialVisibilityPolicy.ShowOwnEarning || !got.FinancialVisibilityPolicy.ShowProviderShare {
		t.Fatalf("visibility policy not decoded: %+v", got.FinancialVisibilityPolicy)
	}
}
