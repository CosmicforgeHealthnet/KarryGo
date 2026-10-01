package managedsettlement_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cosmicforge/logistics/shared/go/managedsettlement"
)

func walletStub(t *testing.T, status int, payload any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Signature") == "" && r.Header.Get("X-Service-Name") == "" {
			t.Errorf("request reached Wallet unsigned: %v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(server.Close)
	return server
}

func clientFor(server *httptest.Server) managedsettlement.Client {
	return managedsettlement.Client{
		BaseURL:     server.URL,
		ServiceName: "driver-taxi-service",
		Secret:      []byte("secret"),
		HTTPClient:  server.Client(),
	}
}

func TestPostedSettlementReadsAsSettled(t *testing.T) {
	server := walletStub(t, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{"settlements": []map[string]any{{
			"instruction_id": "msi-1", "instruction_version": 1,
			"source_service": "TAXI", "source_job_id": "book-1",
			"status": managedsettlement.StatusPosted, "provider_share_minor": 1500,
		}}},
	})
	outcome, err := clientFor(server).ForJob(context.Background(), "TAXI", "book-1")
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Settled || outcome.Pending || outcome.Unknown {
		t.Fatalf("outcome = %+v, want settled", outcome)
	}
}

// A reversed settlement moved money and then moved it back. A caller asking
// "is this job paid?" must hear no, or a correction turns into a non-payment
// nobody notices.
func TestReversedSettlementIsNotSettled(t *testing.T) {
	server := walletStub(t, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{"settlements": []map[string]any{{
			"instruction_id": "msi-1", "status": managedsettlement.StatusReversed,
		}}},
	})
	outcome, err := clientFor(server).ForJob(context.Background(), "TAXI", "book-1")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Settled {
		t.Fatal("a reversed settlement reported the job as paid")
	}
}

// A retried instruction leaves its failed attempts behind. Those are history,
// not a reason to pay the job again.
func TestOnePostedRowAmongFailuresStillMeansSettled(t *testing.T) {
	server := walletStub(t, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{"settlements": []map[string]any{
			{"instruction_id": "msi-1", "instruction_version": 1, "status": managedsettlement.StatusFailedRetryable},
			{"instruction_id": "msi-1", "instruction_version": 2, "status": managedsettlement.StatusPosted},
		}},
	})
	outcome, err := clientFor(server).ForJob(context.Background(), "TAXI", "book-1")
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Settled || outcome.Pending {
		t.Fatalf("outcome = %+v, want settled and not pending", outcome)
	}
}

func TestOpenReconciliationIsPendingNotSettled(t *testing.T) {
	server := walletStub(t, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{"settlements": []map[string]any{
			{"instruction_id": "msi-1", "status": managedsettlement.StatusReconciliationRequired},
		}},
	})
	outcome, err := clientFor(server).ForJob(context.Background(), "TAXI", "book-1")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Settled || !outcome.Pending {
		t.Fatalf("outcome = %+v, want pending", outcome)
	}
}

// No settlement rows is a definite answer -- Fleet has not instructed yet --
// and must not be confused with Wallet being unreachable.
func TestNoSettlementsIsResolvedAndNotSettled(t *testing.T) {
	server := walletStub(t, http.StatusOK, map[string]any{
		"success": true, "data": map[string]any{"settlements": []any{}},
	})
	outcome, err := clientFor(server).ForJob(context.Background(), "TAXI", "book-1")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Settled || outcome.Pending || outcome.Unknown || !outcome.Resolved() {
		t.Fatalf("outcome = %+v, want resolved, nothing settled", outcome)
	}
}

// This is the invariant a timeout must not violate: an unreachable Wallet is
// never "nothing settled". Reading it that way is how a job gets paid twice.
func TestUnreachableWalletIsUnknownNotUnsettled(t *testing.T) {
	server := walletStub(t, http.StatusInternalServerError, map[string]any{
		"success": false, "error": map[string]any{"message": "boom"},
	})
	outcome, err := clientFor(server).ForJob(context.Background(), "TAXI", "book-1")
	if err == nil {
		t.Fatal("a failed read returned no error")
	}
	if !outcome.Unknown || outcome.Resolved() {
		t.Fatalf("outcome = %+v, want unknown", outcome)
	}
	if outcome.Settled {
		t.Fatal("a failed read claimed the job was settled")
	}
}

func TestUnconfiguredClientRefusesRatherThanAnsweringUnsettled(t *testing.T) {
	outcome, err := managedsettlement.Client{}.ForJob(context.Background(), "TAXI", "book-1")
	if err == nil {
		t.Fatal("an unconfigured client answered a settlement question")
	}
	if !outcome.Unknown {
		t.Fatalf("outcome = %+v, want unknown", outcome)
	}
}

// WAL-011A. The synchronous completion path and the reconciliation worker must
// address the same job under the same name, whatever spelling each happens to
// hold.
func TestIdentityIsStableAcrossSpellings(t *testing.T) {
	fromCompletion := managedsettlement.IdentityFor("TAXI", "book-1")
	fromReconciler := managedsettlement.IdentityFor("  taxi ", " book-1 ")
	if fromCompletion.Key() != fromReconciler.Key() {
		t.Fatalf("identity diverged: %q vs %q", fromCompletion.Key(), fromReconciler.Key())
	}
	if !fromCompletion.Valid() {
		t.Fatal("a complete identity reported itself invalid")
	}
	if managedsettlement.IdentityFor("TAXI", "").Valid() {
		t.Fatal("an identity with no job id reported itself valid")
	}
}

func TestLookupUsesTheIdentityItWasGiven(t *testing.T) {
	var requested string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "data": map[string]any{"settlements": []any{}},
		})
	}))
	defer server.Close()

	client := managedsettlement.Client{
		BaseURL: server.URL, ServiceName: "driver-dispatch-delivery-service",
		Secret: []byte("secret"), HTTPClient: server.Client(),
	}
	if _, err := client.Lookup(context.Background(), managedsettlement.IdentityFor("dispatch", "trip-9")); err != nil {
		t.Fatal(err)
	}
	want := "/api/v1/payment-wallet/internal/finance/v1/managed-settlements/by-job/DISPATCH/trip-9"
	if requested != want {
		t.Fatalf("requested %q, want %q", requested, want)
	}
}
