package fleetprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestClientAcceptAssignmentSignsFleetDecision(t *testing.T) {
	const caller = "driver-taxi-service"
	secret := []byte("taxi-fleet-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{caller: secret}, time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/fleet-partner/v1/assignments/asg-1/provider-accept" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get(HeaderIdempotencyKey) != "response-1" {
			t.Fatalf("idempotency=%q", r.Header.Get(HeaderIdempotencyKey))
		}
		if _, err := verifier.VerifyRequest(r); err != nil {
			t.Fatalf("verify hmac: %v", err)
		}
		var body decisionPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.ProviderID != "provider-1" || body.ProviderService != SourceServiceTaxi || body.ProviderSessionID != "session-1" {
			t.Fatalf("bad provider binding: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"assignmentId": "asg-1", "status": "ACCEPTED"}})
	}))
	defer server.Close()

	client, ok := NewClient(Config{BaseURL: server.URL, ServiceName: caller, Secret: secret, Timeout: time.Second})
	if !ok {
		t.Fatal("client was not configured")
	}
	raw, err := client.AcceptAssignment(context.Background(), Meta{IdempotencyKey: "response-1"}, "provider-1", "session-1", "asg-1", Decision{ClientResponseID: "response-1"})
	if err != nil {
		t.Fatalf("AcceptAssignment: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("empty response")
	}
}
