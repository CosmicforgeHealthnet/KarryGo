package bookingclients

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestCustomerContextClientSignsAndMinimizes(t *testing.T) {
	secret := []byte("taxi-customer-context-secret-0123456789")
	verifier := serviceauth.NewVerifier(map[string][]byte{"driver-taxi-service": secret}, 5*time.Minute)
	var gotPath, gotScopes string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.VerifyRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		gotPath, gotScopes = r.URL.Path, r.URL.Query().Get("scopes")
		_, _ = w.Write([]byte(`{"success":true,"data":{"mobility_preferences":{"wheelchair_required":true,"child_seat_preference":"NOT_NEEDED","luggage_default":"STANDARD","communication_preferences":{"quiet_ride":true,"preferred_channel":"TEXT"},"special_assistance_notes":"must not be read","version":4}}}`))
	}))
	defer server.Close()
	client := &CustomerContextClient{BaseURL: server.URL, ServiceName: "driver-taxi-service", Secret: secret}
	hints := client.MobilityHints(t.Context(), "cust-1")
	if hints.Status != capability.CustomerContextUsed || hints.WheelchairRequired == nil || !*hints.WheelchairRequired || hints.Version != 4 ||
		hints.QuietRide == nil || !*hints.QuietRide || hints.PreferredChannel != "TEXT" {
		t.Fatalf("hints = %+v", hints)
	}
	if gotPath != "/api/v1/customer/internal/customers/cust-1/context" || gotScopes != "mobility_preferences" {
		t.Fatalf("requested %s scopes=%s", gotPath, gotScopes)
	}
}

func TestCustomerContextClientDegradesToUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	client := &CustomerContextClient{BaseURL: server.URL, ServiceName: "driver-taxi-service", Secret: []byte("x")}
	if hints := client.MobilityHints(t.Context(), "cust-1"); hints.Status != capability.CustomerContextUnavailable || hints.WheelchairRequired != nil {
		t.Fatalf("forbidden context = %+v", hints)
	}
	var unconfigured *CustomerContextClient
	if hints := unconfigured.MobilityHints(t.Context(), "cust-1"); hints.Status != capability.CustomerContextNotRequested {
		t.Fatalf("unconfigured = %+v", hints)
	}
}
