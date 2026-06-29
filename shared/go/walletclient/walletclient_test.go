package walletclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	testService = "taxi-service"
	testSecret  = "shared-wallet-secret"
)

// newTestServer returns an httptest server that verifies the incoming request's
// HMAC service-auth signature and records the method + path it was called with.
func newTestServer(t *testing.T, respond func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *recordedRequest) {
	t.Helper()
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{testService: []byte(testSecret)}, 5*time.Minute)
	rec := &recordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, err := verifier.VerifyRequest(r)
		if err != nil {
			t.Errorf("signature verification failed: %v", err)
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]string{"message": "bad signature"}})
			return
		}
		rec.serviceName = name
		rec.method = r.Method
		rec.path = r.URL.Path
		rec.escapedPath = r.URL.EscapedPath()
		respond(w, r)
	}))
	t.Cleanup(server.Close)
	return server, rec
}

type recordedRequest struct {
	serviceName string
	method      string
	path        string
	escapedPath string
}

func successEnvelope(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

func newClient(baseURL string) Client {
	return Client{
		BaseURL:     baseURL,
		ServiceName: testService,
		Secret:      []byte(testSecret),
	}
}

func TestCreatePaymentIntentSignsAndHitsCorrectPath(t *testing.T) {
	server, rec := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		successEnvelope(w, PaymentIntent{ID: "pi-1", Reference: "ref-1", Status: "requires_payment", AmountKobo: 500000})
	})

	got, err := newClient(server.URL).CreatePaymentIntent(context.Background(), PaymentIntentRequest{
		SourceService:   "taxi-service",
		SourceReference: "trip-1",
		CustomerID:      "cust-1",
		AmountKobo:      500000,
		PaymentMethod:   MethodWallet,
		IdempotencyKey:  "idem-1",
	})
	if err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}
	if got.ID != "pi-1" || got.AmountKobo != 500000 {
		t.Fatalf("unexpected payment intent: %+v", got)
	}
	if rec.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", rec.method)
	}
	if rec.path != "/api/v1/payment-wallet/internal/payment-intents" {
		t.Fatalf("path = %s, want internal/payment-intents", rec.path)
	}
	if rec.serviceName != testService {
		t.Fatalf("service = %s, want %s", rec.serviceName, testService)
	}
}

func TestClientAcceptsFullAPIBaseURLWithoutDuplicatingPath(t *testing.T) {
	server, rec := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		successEnvelope(w, PaymentIntent{ID: "pi-full-base"})
	})
	client := newClient(server.URL + "/api/v1/payment-wallet")

	if _, err := client.CreatePaymentIntent(context.Background(), PaymentIntentRequest{}); err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}
	if rec.path != "/api/v1/payment-wallet/internal/payment-intents" {
		t.Fatalf("path = %s, want a single API base prefix", rec.path)
	}
}

func TestPayFromWalletEscapesIDInPath(t *testing.T) {
	server, rec := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		successEnvelope(w, PaymentIntent{ID: "pi-9", Status: "held"})
	})

	_, err := newClient(server.URL).PayFromWallet(context.Background(), "pi-9", "idem-2")
	if err != nil {
		t.Fatalf("PayFromWallet: %v", err)
	}
	if rec.path != "/api/v1/payment-wallet/internal/payment-intents/pi-9/pay-from-wallet" {
		t.Fatalf("path = %s", rec.path)
	}
}

func TestCompleteJobEscapesPathSegments(t *testing.T) {
	server, rec := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		successEnvelope(w, PaymentIntent{Status: "completed"})
	})

	_, err := newClient(server.URL).CompleteJob(context.Background(), "taxi-service", "trip/42")
	if err != nil {
		t.Fatalf("CompleteJob: %v", err)
	}
	// "trip/42" must be path-escaped so it stays a single segment on the wire.
	if rec.escapedPath != "/api/v1/payment-wallet/internal/jobs/taxi-service/trip%2F42/complete" {
		t.Fatalf("escapedPath = %s", rec.escapedPath)
	}
}

func TestGetPaymentUsesGET(t *testing.T) {
	server, rec := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		successEnvelope(w, PaymentIntent{Reference: "ref-7"})
	})

	got, err := newClient(server.URL).GetPayment(context.Background(), "ref-7")
	if err != nil {
		t.Fatalf("GetPayment: %v", err)
	}
	if got.Reference != "ref-7" {
		t.Fatalf("reference = %s", got.Reference)
	}
	if rec.method != http.MethodGet {
		t.Fatalf("method = %s, want GET", rec.method)
	}
	if rec.path != "/api/v1/payment-wallet/internal/payments/ref-7" {
		t.Fatalf("path = %s", rec.path)
	}
}

func TestErrorEnvelopeSurfacesMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]string{"message": "duplicate idempotency key"},
		})
	}))
	defer server.Close()

	_, err := newClient(server.URL).RequestRefund(context.Background(), RefundRequest{
		PaymentReference: "ref-1", AmountKobo: 100, IdempotencyKey: "idem-3",
	})
	if err == nil {
		t.Fatal("expected error from conflict envelope, got nil")
	}
	if err.Error() != "duplicate idempotency key" {
		t.Fatalf("error = %q, want surfaced server message", err.Error())
	}
}
