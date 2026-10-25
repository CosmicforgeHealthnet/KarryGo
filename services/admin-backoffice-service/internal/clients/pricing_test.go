package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestPricingAdminClientUsesFixedSignedRoutesAndResolveQuery(t *testing.T) {
	secret := []byte(strings.Repeat("p", 32))
	var seen int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		if _, err := serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": secret}, time.Minute).VerifyRequest(r); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if seen == 1 {
			if r.Method != http.MethodPost || r.URL.Path != "/internal/pricing/v1/admin/policy-versions/version-1/approve" || r.Header.Get("X-Idempotency-Key") != "command-1" {
				t.Fatalf("unexpected command transport: %s %s", r.Method, r.URL.String())
			}
		} else if r.Method != http.MethodGet || r.URL.Path != "/internal/pricing/v1/admin/resolve" || r.URL.Query().Get("kind") != "taxi_fare" || r.URL.Query().Get("zone_id") != "zone-1" {
			t.Fatalf("resolve was not translated to the owner GET contract: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()
	client, err := NewPricing(server.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	client.client = server.Client()
	defer client.Close()
	if _, err = client.Execute(context.Background(), PricingOperation{Method: http.MethodPost, Path: "policy.approve", Target: "version-1", Staff: "staff-1", Reason: "CASE-1", IdempotencyKey: "command-1", Payload: json.RawMessage(`{"performed_by":"staff-1","reason":"CASE-1","expected_body_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Execute(context.Background(), PricingOperation{Method: http.MethodPost, Path: "resolve", Staff: "staff-1", Reason: "CASE-1", IdempotencyKey: "command-2", Payload: json.RawMessage(`{"kind":"taxi_fare","zone_id":"zone-1"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Execute(context.Background(), PricingOperation{Method: http.MethodPost, Path: "policy.approve", Target: "../secret", Staff: "staff-1", Reason: "CASE-1", IdempotencyKey: "command-3", Payload: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("unsafe target accepted")
	}
	if seen != 2 {
		t.Fatalf("unexpected request count %d", seen)
	}
}
