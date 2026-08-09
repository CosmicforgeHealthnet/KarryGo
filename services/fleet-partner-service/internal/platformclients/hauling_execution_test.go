package platformclients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHaulingExecutionUsesPrivateSignedContract(t *testing.T) {
	secret := []byte("hauling-test-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != haulingInternalBase+"/reservations" {
			t.Errorf("path=%s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("X-Service-Signature") == "" || r.Header.Get("X-Service-Nonce") == "" {
			t.Error("missing signature or nonce")
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("Idempotency-Key") != "reserve-1" {
			t.Errorf("idempotency header=%q", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"reservation_id": "res-1", "status": "RESERVED", "winner_organization_id": "org-1", "source_version": 4}})
	}))
	defer server.Close()
	set := BuildExecutionSet(Options{}, Options{ServiceName: "driver-hauling-service", BaseURL: server.URL, Secret: secret, Timeout: time.Second})
	hauling, _ := set.For(CategoryHauling)
	if !hauling.Configured() {
		t.Fatal("hauling adapter not configured")
	}
	out, err := hauling.ReserveForOrganization(context.Background(), CallMeta{IdempotencyKey: "reserve-1"}, ReserveRequest{SourceRequestID: "req-1", OrganizationID: "org-1", IdempotencyKey: "reserve-1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "RESERVED" || out.SourceVersion != 4 {
		t.Fatalf("out=%+v", out)
	}
	taxi, _ := set.For(CategoryTaxi)
	if taxi.Configured() {
		t.Fatal("taxi became configured with hauling")
	}
}

func TestHaulingAuthorityClientUsesHaulingIdentity(t *testing.T) {
	c := NewHaulingVehicleAuthorityClient(Options{})
	if c.AuthorityService() != "driver-hauling-service" {
		t.Fatalf("service=%q", c.AuthorityService())
	}
	if c.Configured() {
		t.Fatal("empty client configured")
	}
}
