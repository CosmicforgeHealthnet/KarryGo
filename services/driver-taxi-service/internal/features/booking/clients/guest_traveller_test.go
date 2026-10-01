package bookingclients

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// The body mirrors customer-service's guest_traveller context response
// (contexthttp TestGuestTravellerScopeIsOwnershipScopedAndMinimized).
const guestBody = `{"success":true,"data":{"customer_id":"cust-1","scopes":["guest_traveller"],"guest_traveller":{"id":"guest-1","name":"Ada","phone":"+2348011111111","mobility_requirements":{"wheelchair_required":true,"pet_travel_preference":"SMALL_PET"},"version":4},"context_version":"v"}}`

func TestGuestTravellerClientSignsScopesAndParses(t *testing.T) {
	secret := []byte("taxi-customer-context-secret-0123456789")
	verifier := serviceauth.NewVerifier(map[string][]byte{"driver-taxi-service": secret}, 5*time.Minute)
	var gotPath, gotScopes, gotGuest string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.VerifyRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		gotPath, gotScopes, gotGuest = r.URL.Path, r.URL.Query().Get("scopes"), r.URL.Query().Get("guest_traveller_id")
		_, _ = w.Write([]byte(guestBody))
	}))
	defer server.Close()
	client := &CustomerContextClient{BaseURL: server.URL, ServiceName: "driver-taxi-service", Secret: secret}
	guest, err := client.GuestTraveller(t.Context(), "cust-1", "guest-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/customer/internal/customers/cust-1/context" || gotScopes != "guest_traveller" || gotGuest != "guest-1" {
		t.Fatalf("requested %s scopes=%s guest=%s", gotPath, gotScopes, gotGuest)
	}
	if guest.Snapshot.GuestTravellerRef != "guest-1" || guest.Snapshot.Name != "Ada" || guest.Snapshot.Phone == nil || guest.Snapshot.SourceVersion != 4 {
		t.Fatalf("snapshot = %+v", guest.Snapshot)
	}
	if guest.Hints.WheelchairRequired == nil || !*guest.Hints.WheelchairRequired || guest.Hints.PetTravelPreference == nil || *guest.Hints.PetTravelPreference != "SMALL_PET" {
		t.Fatalf("hints = %+v", guest.Hints)
	}
}

func TestGuestTravellerClientFailsClosed(t *testing.T) {
	respond := func(status int, body string) *CustomerContextClient {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(server.Close)
		return &CustomerContextClient{BaseURL: server.URL, ServiceName: "driver-taxi-service", Secret: []byte("x")}
	}
	cases := []struct {
		name   string
		client *CustomerContextClient
		want   error
	}{
		{"not found", respond(http.StatusNotFound, `{}`), capability.ErrGuestTravellerNotFound},
		{"forbidden", respond(http.StatusForbidden, `{}`), capability.ErrGuestTravellerUnavailable},
		{"outage", respond(http.StatusServiceUnavailable, `{}`), capability.ErrGuestTravellerUnavailable},
		{"answer for a different traveller", respond(http.StatusOK, `{"success":true,"data":{"guest_traveller":{"id":"guest-2","name":"Bo","version":1}}}`), capability.ErrGuestTravellerUnavailable},
		{"no traveller in answer", respond(http.StatusOK, `{"success":true,"data":{}}`), capability.ErrGuestTravellerUnavailable},
		{"unconfigured", nil, capability.ErrGuestTravellerUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.client.GuestTraveller(t.Context(), "cust-1", "guest-1"); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
