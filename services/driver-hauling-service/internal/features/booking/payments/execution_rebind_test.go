package payments

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeWallet serves the Section 17 binding read and rebind for one booking.
type fakeWallet struct {
	mu         sync.Mutex
	status     string
	providerID string
	version    int64
	missing    bool
	rebinds    []map[string]any
}

func (w *fakeWallet) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if !strings.HasSuffix(r.URL.Path, "/internal/jobs/driver-hauling-service/booking-1/execution-provider") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		rw.Header().Set("Content-Type", "application/json")
		if w.missing {
			rw.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(rw, `{"success":false,"error":{"code":"not_found","message":"Payment not found."}}`)
			return
		}
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(rw).Encode(map[string]any{"success": true, "data": map[string]any{
				"execution_binding_version": w.version, "provider_type": "truck_provider",
				"provider_id": w.providerID, "status": w.status}})
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.rebinds = append(w.rebinds, body)
		w.providerID, w.version = body["provider_id"].(string), w.version+1
		_ = json.NewEncoder(rw).Encode(map[string]any{"success": true, "data": map[string]any{
			"id": "rebind-1", "new_provider_id": w.providerID, "to_version": w.version}})
	})
}

func newTestClient(t *testing.T, wallet *fakeWallet, cases RecoveryCaseLookup) *WalletPaymentClient {
	server := httptest.NewServer(wallet.handler(t))
	t.Cleanup(server.Close)
	return NewWalletPaymentClient(server.URL, "driver-hauling-service", []byte("secret")).WithRecoveryCases(cases)
}

func caseFor(ref string) RecoveryCaseLookup {
	return func(context.Context, string, string) (string, error) { return ref, nil }
}

func TestEnsureExecutionProviderIsOneReadWhenAlreadyBound(t *testing.T) {
	wallet := &fakeWallet{status: "held", providerID: "repl", version: 2}
	client := newTestClient(t, wallet, caseFor("case-1"))
	if err := client.EnsureExecutionProvider(context.Background(), "booking-1", "repl"); err != nil {
		t.Fatal(err)
	}
	if len(wallet.rebinds) != 0 {
		t.Fatal("an already-correct binding must not be rebound")
	}
}

func TestEnsureExecutionProviderRebindsWithTheRecoveryCase(t *testing.T) {
	wallet := &fakeWallet{status: "held", providerID: "orig", version: 1}
	client := newTestClient(t, wallet, caseFor("case-1"))
	if err := client.EnsureExecutionProvider(context.Background(), "booking-1", "repl"); err != nil {
		t.Fatal(err)
	}
	if len(wallet.rebinds) != 1 {
		t.Fatalf("expected one rebind, got %d", len(wallet.rebinds))
	}
	body := wallet.rebinds[0]
	if body["provider_id"] != "repl" || body["recovery_case_ref"] != "case-1" ||
		body["expected_version"] != float64(1) || body["idempotency_key"] != "rebind-booking-1-case-1-v1" {
		t.Fatalf("unexpected rebind body %v", body)
	}
	if _, carried := body["amount_kobo"]; carried {
		t.Fatal("a rebind must never carry an amount")
	}
}

// A mismatch nobody can explain is refused rather than resolved by guessing.
func TestEnsureExecutionProviderRefusesAnUnexplainedMismatch(t *testing.T) {
	wallet := &fakeWallet{status: "held", providerID: "orig", version: 1}
	client := newTestClient(t, wallet, caseFor(""))
	if err := client.EnsureExecutionProvider(context.Background(), "booking-1", "someone"); err == nil {
		t.Fatal("an unexplained binding mismatch must be refused")
	}
	if len(wallet.rebinds) != 0 {
		t.Fatal("no rebind may be sent without a recovery case")
	}
}

func TestNothingHeldIsNothingToRebind(t *testing.T) {
	for name, wallet := range map[string]*fakeWallet{
		"no wallet payment": {missing: true},
		"refunded":          {status: "refunded", providerID: "orig", version: 1},
	} {
		client := newTestClient(t, wallet, caseFor("case-1"))
		if err := client.EnsureExecutionProvider(context.Background(), "booking-1", "repl"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := client.RebindToReplacement(context.Background(), "booking-1", "repl", "case-1"); err != nil {
			t.Fatalf("%s: recovery rebind: %v", name, err)
		}
		if len(wallet.rebinds) != 0 {
			t.Fatalf("%s: rebound a payment that holds nothing", name)
		}
	}
}
