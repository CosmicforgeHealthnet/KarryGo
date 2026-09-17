package commercecapability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestClientReadsSignedCapabilityAndCaches(t *testing.T) {
	secret := []byte("commerce-capability-secret-0123456789")
	verifier := serviceauth.NewVerifier(map[string][]byte{"vendor-service": secret}, 5*time.Minute)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.VerifyRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		calls++
		_, _ = w.Write([]byte(`{"success":true,"data":{"authority":"TAXI","contract_version":2,"capability_version":"p1|multi=true","multi_pickup_supported":true,"maximum_pickups":3,"secure_pickup_handover":true}}`))
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, Path: "/api/v1/taxi/internal/commerce/capabilities", ServiceName: "vendor-service", Secret: secret, TTL: time.Minute}
	capability, err := client.Capability(context.Background())
	if err != nil || !capability.MultiPickupSupported || capability.MaximumPickups != 3 || !capability.SecurePickupHandover {
		t.Fatalf("capability = %+v (%v)", capability, err)
	}
	if _, err = client.Capability(context.Background()); err != nil || calls != 1 {
		t.Fatalf("calls = %d, want the cached answer to be reused", calls)
	}
}

func TestClientFailsClosed(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"refused": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) },
		"unknown contract": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"success":true,"data":{"contract_version":99,"multi_pickup_supported":true}}`))
		},
		"not a body": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`nonsense`)) },
	}
	for name, handler := range cases {
		server := httptest.NewServer(handler)
		client := &Client{BaseURL: server.URL, Path: "/capabilities", ServiceName: "vendor-service", Secret: []byte("s")}
		capability, err := client.Capability(context.Background())
		if err == nil || capability.MultiPickupSupported {
			t.Errorf("%s: capability = %+v (%v), want no capability", name, capability, err)
		}
		server.Close()
	}
	var unconfigured *Client
	if _, err := unconfigured.Capability(context.Background()); err == nil {
		t.Error("an unconfigured client must not report a capability")
	}
}
