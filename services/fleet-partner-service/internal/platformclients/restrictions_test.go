package platformclients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestRestrictionClientUsesSignedVersionedIdempotentContract(t *testing.T) {
	secret := []byte("restriction-contract-test-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"fleet-partner-service": secret}, time.Minute)
	var mu sync.Mutex
	seenNonces := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := verifier.VerifyRequest(r)
		if err != nil || caller != "fleet-partner-service" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		nonce := r.Header.Get(serviceauth.HeaderNonce)
		mu.Lock()
		if nonce == "" || seenNonces[nonce] {
			mu.Unlock()
			http.Error(w, "replayed", http.StatusConflict)
			return
		}
		seenNonces[nonce] = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/private/restrictions/apply":
			var command map[string]any
			if err = json.NewDecoder(r.Body).Decode(&command); err != nil {
				t.Error(err)
			}
			if command["operationReference"] != "operation-1" || command["sourceReference"] != "restriction-1" || command["sourceVersion"] != float64(7) {
				t.Errorf("apply command=%v", command)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"restrictionId":"authority-1","requestedState":"APPLIED","outcome":"CONFIRMED","sourceVersion":7}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/private/restrictions/operations/operation-1":
			_, _ = w.Write([]byte(`{"success":true,"data":{"restrictionId":"authority-1","requestedState":"APPLIED","outcome":"CONFIRMED","sourceVersion":7}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/private/restrictions/clear":
			_, _ = w.Write([]byte(`{"success":true,"data":{"restrictionId":"authority-1","requestedState":"CLEARED","outcome":"CONFIRMED","sourceVersion":8}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewRestrictionClient(New(Options{ServiceName: "dispatch", BaseURL: server.URL, Secret: secret, HTTPClient: server.Client()}), "/private/restrictions")
	meta := CallMeta{OrganizationID: "org-1", CorrelationID: "cor-1", IdempotencyKey: "operation-1"}
	command := RestrictionCommand{RestrictionID: "restriction-1", OrganizationID: "org-1", SubjectType: "PROVIDER", SubjectReference: "membership-1", ReasonCode: "NON_COMPLIANT", SourceReference: "status-1", ExpectedVersion: 7}
	applied, err := client.Apply(context.Background(), meta, command)
	if err != nil || applied.State != "APPLIED" || applied.SourceVersion != 7 {
		t.Fatalf("apply=%+v err=%v", applied, err)
	}
	read, err := client.Read(context.Background(), meta, "operation-1")
	if err != nil || read.State != "APPLIED" || read.SourceVersion != 7 {
		t.Fatalf("read=%+v err=%v", read, err)
	}
	command.ExpectedVersion = 8
	cleared, err := client.Clear(context.Background(), meta, command)
	if err != nil || cleared.State != "CLEARED" || cleared.SourceVersion != 8 {
		t.Fatalf("clear=%+v err=%v", cleared, err)
	}
	if len(seenNonces) != 3 {
		t.Fatalf("unique signed nonces=%d, want 3", len(seenNonces))
	}
}

func TestRestrictionClientTimeoutIsUnknownOutcome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	client := NewRestrictionClient(New(Options{ServiceName: "dispatch", BaseURL: server.URL, Secret: []byte("secret"), Timeout: 20 * time.Millisecond, HTTPClient: server.Client()}), "/restrictions")
	_, err := client.Apply(context.Background(), CallMeta{IdempotencyKey: "timeout-operation"}, RestrictionCommand{RestrictionID: "restriction-1"})
	if err == nil {
		t.Fatal("timed-out mutation succeeded")
	}
	contractErr, ok := err.(*Error)
	if !ok || contractErr.Outcome != OutcomeUnknown {
		t.Fatalf("timeout error=%T %v", err, err)
	}
}
