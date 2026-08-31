package commercequote

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func validRequest() Request {
	return Request{
		QuoteReference: "vq_test_1",
		VehicleClass:   VehicleMotorcycle,
		Pickup:         Point{Latitude: 6.4550, Longitude: 3.4100},
		Dropoff:        Point{Latitude: 6.4300, Longitude: 3.4500},
		Load:           Load{PackageCount: 2, TotalWeightGrams: 3000},
		Currency:       "NGN",
	}
}

func serveQuote(t *testing.T, quote Quote) *httptest.Server {
	t.Helper()

	verifier := serviceauth.NewVerifier(
		serviceauth.Secrets{SignatureCaller: []byte("secret")},
		5*time.Minute,
	)

	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			// The signature must actually verify; a test that accepted an
			// unsigned request would prove nothing about the real path.
			if _, err := verifier.VerifyRequest(r); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Envelope{Success: true, Data: quote})
		},
	))
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	return New(Options{
		BaseURL: baseURL,
		APIBase: "/api/v1/test",
		Service: "TEST",
		Secret:  "secret",
	})
}

func TestClient_ServiceableQuoteRoundTrip(t *testing.T) {
	want := Quote{
		QuoteID:            "dq_1",
		QuoteReference:     "vq_test_1",
		Revision:           1,
		Service:            "DISPATCH",
		Serviceable:        true,
		ReasonCode:         ReasonServiceable,
		VehicleClass:       VehicleMotorcycle,
		PickupRef:          "dq_1:pickup",
		DropoffRef:         "dq_1:dropoff",
		DistanceKm:         4.9,
		EstimatedMinutes:   26,
		FeeMinor:           168000,
		Currency:           "NGN",
		ExpiresAt:          time.Now().Add(15 * time.Minute),
		AvailableProviders: 3,
	}

	server := serveQuote(t, want)
	defer server.Close()

	quote, err := newTestClient(t, server.URL).
		RequestQuote(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}

	if quote.QuoteID != "dq_1" || quote.FeeMinor != 168000 {
		t.Fatalf("unexpected quote: %+v", quote)
	}
}

// An unserviceable answer is a normal outcome, not an error: the planner must
// be able to report it to the customer.
func TestClient_UnserviceableIsAnAnswerNotAnError(t *testing.T) {
	server := serveQuote(t, Quote{
		QuoteReference: "vq_test_1",
		Serviceable:    false,
		ReasonCode:     ReasonNoProviderAvailable,
	})
	defer server.Close()

	quote, err := newTestClient(t, server.URL).
		RequestQuote(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("an unserviceable answer must not be an error: %v", err)
	}

	if quote.Serviceable || quote.ReasonCode != ReasonNoProviderAvailable {
		t.Fatalf("unexpected quote: %+v", quote)
	}
}

// The invariant this whole section turns on: a fee may never accompany an
// answer that cannot be served.
func TestClient_RejectsFeeOnUnserviceableQuote(t *testing.T) {
	server := serveQuote(t, Quote{
		QuoteReference: "vq_test_1",
		Serviceable:    false,
		ReasonCode:     ReasonNoProviderAvailable,
		FeeMinor:       50000,
	})
	defer server.Close()

	_, err := newTestClient(t, server.URL).
		RequestQuote(context.Background(), validRequest())

	if !errors.Is(err, ErrContractViolation) {
		t.Fatalf("expected a contract violation, got %v", err)
	}
}

func TestClient_RejectsMalformedAnswers(t *testing.T) {
	base := Quote{
		QuoteID:            "dq_1",
		QuoteReference:     "vq_test_1",
		Serviceable:        true,
		ReasonCode:         ReasonServiceable,
		VehicleClass:       VehicleMotorcycle,
		FeeMinor:           1000,
		Currency:           "NGN",
		ExpiresAt:          time.Now().Add(time.Minute),
		AvailableProviders: 1,
	}

	cases := map[string]func(*Quote){
		"wrong reference":     func(q *Quote) { q.QuoteReference = "someone_else" },
		"no authority id":     func(q *Quote) { q.QuoteID = "" },
		"wrong currency":      func(q *Quote) { q.Currency = "USD" },
		"wrong vehicle class": func(q *Quote) { q.VehicleClass = VehicleTruck },
		"no expiry":           func(q *Quote) { q.ExpiresAt = time.Time{} },
		"no provider":         func(q *Quote) { q.AvailableProviders = 0 },
		"negative fee":        func(q *Quote) { q.FeeMinor = -1 },
		"unserviceable without reason": func(q *Quote) {
			q.Serviceable = false
			q.ReasonCode = ""
			q.FeeMinor = 0
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			quote := base
			mutate(&quote)

			server := serveQuote(t, quote)
			defer server.Close()

			_, err := newTestClient(t, server.URL).
				RequestQuote(context.Background(), validRequest())

			if !errors.Is(err, ErrContractViolation) {
				t.Fatalf("expected a contract violation, got %v", err)
			}
		})
	}
}

func TestClient_NotConfigured(t *testing.T) {
	client := New(Options{APIBase: "/api/v1/test"})

	if client.Configured() {
		t.Fatal("a client with no base URL or secret must not report configured")
	}

	if _, err := client.RequestQuote(
		context.Background(), validRequest(),
	); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestClient_RefusesInvalidRequestsBeforeSending(t *testing.T) {
	server := serveQuote(t, Quote{})
	defer server.Close()

	client := newTestClient(t, server.URL)

	cases := map[string]func(*Request){
		"no reference":     func(r *Request) { r.QuoteReference = "" },
		"bad class":        func(r *Request) { r.VehicleClass = "HELICOPTER" },
		"car class":        func(r *Request) { r.VehicleClass = "CAR" },
		"null island":      func(r *Request) { r.Dropoff = Point{} },
		"bad currency":     func(r *Request) { r.Currency = "N" },
		"no packages":      func(r *Request) { r.Load.PackageCount = 0 },
		"negative measure": func(r *Request) { r.Load.TotalWeightGrams = -1 },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := validRequest()
			mutate(&request)

			if _, err := client.RequestQuote(
				context.Background(), request,
			); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("expected ErrInvalidRequest, got %v", err)
			}
		})
	}
}

func TestClient_ServerFaultIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	))
	defer server.Close()

	if _, err := newTestClient(t, server.URL).RequestQuote(
		context.Background(), validRequest(),
	); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

// The base URL is a bare origin and the client appends the API base itself.
// This is the doubled-path bug the platform has already hit once.
func TestClient_BuildsPathFromBareOrigin(t *testing.T) {
	var seenPath string

	verifier := serviceauth.NewVerifier(
		serviceauth.Secrets{SignatureCaller: []byte("secret")}, 5*time.Minute,
	)

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			seenPath = r.URL.Path

			if _, err := verifier.VerifyRequest(r); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			_ = json.NewEncoder(w).Encode(Envelope{
				Success: true,
				Data: Quote{
					QuoteReference: "vq_test_1",
					Serviceable:    false,
					ReasonCode:     ReasonNoProviderAvailable,
				},
			})
		},
	))
	defer server.Close()

	client := New(Options{
		BaseURL: server.URL,
		APIBase: "api/v1/hauling",
		Secret:  "secret",
	})

	if _, err := client.RequestQuote(
		context.Background(), validRequest(),
	); err != nil {
		t.Fatal(err)
	}

	if seenPath != "/api/v1/hauling"+QuotePath {
		t.Fatalf("path = %q, want %q", seenPath, "/api/v1/hauling"+QuotePath)
	}
}

func TestQuote_Expired(t *testing.T) {
	now := time.Now()

	if (Quote{ExpiresAt: now.Add(time.Minute)}).Expired(now) {
		t.Fatal("a future expiry must not read as expired")
	}

	if !(Quote{ExpiresAt: now}).Expired(now) {
		t.Fatal("an expiry exactly now must read as expired")
	}
}
