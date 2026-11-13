package platformclients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWriteMetaFillsTheHeadersOwningServicesRequire(t *testing.T) {
	got := writeMeta(CallMeta{IdempotencyKey: "assoc:pmb_1"})
	if got.CorrelationID != "assoc:pmb_1" || got.CausationID != "assoc:pmb_1" {
		t.Fatalf("reconciler write meta = %+v", got)
	}
	got = writeMeta(CallMeta{RequestID: "req_1", CorrelationID: "cor_1", IdempotencyKey: "k"})
	if got.CorrelationID != "cor_1" || got.CausationID != "k" {
		t.Fatalf("request write meta = %+v", got)
	}
}

func TestAssociationStatusMapsTheOwningServiceStates(t *testing.T) {
	for state, want := range map[string]string{
		"REQUESTED": AssociationAwaitingProvider,
		"ACTIVE":    AssociationAccepted,
		"SUSPENDED": AssociationAccepted, // consent was given; the organization suspended later
		"REJECTED":  AssociationDeclined,
		"REVOKED":   AssociationCancelled,
		"":          AssociationAwaitingProvider,
	} {
		if got := associationStatus(state); got != want {
			t.Fatalf("associationStatus(%q)=%s want %s", state, got, want)
		}
	}
}

func TestDispatchAndTaxiClientsUseTheirOwnBasePathsAndWriteHeaders(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]http.Header{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Method+" "+r.URL.Path] = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/by-provider/p1"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND"}}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":{"linkId":"l1","status":"REQUESTED","sourceVersion":1}}`))
		}
	}))
	defer server.Close()
	opts := Options{BaseURL: server.URL, Secret: []byte("secret"), Timeout: 2 * time.Second}
	for _, client := range []*ManagedProviderClient{NewDispatchProviderClient(opts), NewTaxiProviderClient(opts)} {
		association, err := client.CreateAssociation(context.Background(), CallMeta{}, ProviderAssociationRequest{
			OrganizationID: "org_1", ProviderID: "p1", MembershipID: "pmb_1", IdempotencyKey: "assoc:pmb_1"})
		if err != nil || association.AssociationID != "l1" || association.Status != AssociationAwaitingProvider {
			t.Fatalf("%s create association = %+v %v", client.Category(), association, err)
		}
		// Nothing live to revoke is not an error.
		if err := client.RevokeOrganizationEligibility(context.Background(), CallMeta{RequestID: "r"}, "p1", "org_1", "REMOVED"); err != nil {
			t.Fatalf("%s revoke without a live association: %v", client.Category(), err)
		}
	}
	for path, base := range map[string]string{"dispatch": DispatchManagedProviderBase, "taxi": TaxiManagedProviderBase} {
		header, ok := seen["POST "+base+"/provider-links"]
		if !ok {
			t.Fatalf("%s client did not call %s", path, base)
		}
		if header.Get("Idempotency-Key") != "assoc:pmb_1" || header.Get("X-Correlation-ID") == "" ||
			header.Get("X-Causation-ID") == "" || header.Get("X-Organization-ID") != "org_1" {
			t.Fatalf("%s write headers = %v", path, header)
		}
	}
}
