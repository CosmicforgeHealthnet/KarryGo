package platformclients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

func TestClassifyStatus(t *testing.T) {
	cases := map[int]Outcome{
		200: OutcomeSuccess,
		201: OutcomeSuccess,
		400: OutcomeValidation,
		422: OutcomeValidation,
		401: OutcomeUnauthenticated,
		403: OutcomeForbidden,
		404: OutcomeNotFound,
		409: OutcomeConflict,
		429: OutcomeRateLimited,
		502: OutcomeTransient,
		503: OutcomeTransient,
		504: OutcomeTransient,
		500: OutcomePermanent,
	}
	for status, want := range cases {
		if got := ClassifyStatus(status); got != want {
			t.Fatalf("ClassifyStatus(%d) = %s, want %s", status, got, want)
		}
	}
	if !OutcomeTransient.Retryable() {
		t.Fatal("transient must be retryable")
	}
	for _, outcome := range []Outcome{OutcomeValidation, OutcomeUnknown, OutcomePermanent, OutcomeConflict} {
		if outcome.Retryable() {
			t.Fatalf("%s must not be retryable", outcome)
		}
	}
}

func TestUnconfiguredClientFailsExplicitly(t *testing.T) {
	client := New(Options{ServiceName: "customer-service"})
	err := client.Ping(context.Background(), CallMeta{})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestSignedRequestWithTracingHeaders(t *testing.T) {
	secret := []byte("client-secret")
	var got *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := New(Options{
		ServiceName: "customer-service",
		BaseURL:     server.URL,
		Secret:      secret,
		Timeout:     2 * time.Second,
		Metrics:     observability.NewMetrics(),
	})

	var out map[string]bool
	err := client.Do(context.Background(), CallMeta{RequestID: "req_1", CorrelationID: "cor_1"}, http.MethodGet, "/health", nil, &out)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if !out["ok"] {
		t.Fatalf("response not decoded: %v", out)
	}
	if got.Header.Get("X-Request-ID") != "req_1" || got.Header.Get("X-Correlation-ID") != "cor_1" {
		t.Fatalf("tracing headers missing: %v", got.Header)
	}
	if got.Header.Get(serviceauth.HeaderServiceName) != config.ServiceName {
		t.Fatalf("service auth name = %q", got.Header.Get(serviceauth.HeaderServiceName))
	}
	if got.Header.Get(serviceauth.HeaderSignature) == "" || got.Header.Get(serviceauth.HeaderTimestamp) == "" {
		t.Fatal("service auth signature headers missing")
	}

	// The signature must verify against the shared verifier contract.
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{config.ServiceName: secret}, time.Minute)
	callerName, err := verifier.VerifyRequest(got)
	if err != nil || callerName != config.ServiceName {
		t.Fatalf("signature does not verify: %s, %v", callerName, err)
	}
}

func TestErrorClassificationFromResponses(t *testing.T) {
	for status, want := range map[int]Outcome{
		http.StatusUnprocessableEntity: OutcomeValidation,
		http.StatusUnauthorized:        OutcomeUnauthenticated,
		http.StatusForbidden:           OutcomeForbidden,
		http.StatusNotFound:            OutcomeNotFound,
		http.StatusConflict:            OutcomeConflict,
		http.StatusTooManyRequests:     OutcomeRateLimited,
		http.StatusServiceUnavailable:  OutcomeTransient,
		http.StatusInternalServerError: OutcomePermanent,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"x","message":"downstream private detail"}}`))
		}))
		client := New(Options{ServiceName: "customer-service", BaseURL: server.URL, Secret: []byte("s"), Timeout: time.Second})
		err := client.Ping(context.Background(), CallMeta{})
		server.Close()

		clientErr, ok := err.(*Error)
		if !ok {
			t.Fatalf("status %d: error type %T", status, err)
		}
		if clientErr.Outcome != want {
			t.Fatalf("status %d: outcome = %s, want %s", status, clientErr.Outcome, want)
		}
		if strings.Contains(clientErr.Error(), "downstream private detail") {
			t.Fatalf("status %d: downstream body leaked into error", status)
		}
	}
}

func TestTimeoutClassifiedUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer server.Close()

	client := New(Options{ServiceName: "customer-service", BaseURL: server.URL, Secret: []byte("s"), Timeout: 50 * time.Millisecond})
	err := client.Ping(context.Background(), CallMeta{})
	clientErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type %T", err)
	}
	if clientErr.Outcome != OutcomeUnknown {
		t.Fatalf("timeout outcome = %s, want %s", clientErr.Outcome, OutcomeUnknown)
	}
	if clientErr.Outcome.Retryable() {
		t.Fatal("unknown outcome must not be blindly retryable")
	}
}

func TestInvalidResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	client := New(Options{ServiceName: "customer-service", BaseURL: server.URL, Secret: []byte("s"), Timeout: time.Second})
	var out map[string]any
	err := client.Do(context.Background(), CallMeta{}, http.MethodGet, "/x", nil, &out)
	clientErr, ok := err.(*Error)
	if !ok || clientErr.Outcome != OutcomeUnknown {
		t.Fatalf("invalid body should be unknown outcome, got %v", err)
	}
}

func TestNewSetBuildsAllClients(t *testing.T) {
	cfg := config.Config{
		HTTPClientTimeout: time.Second,
		Clients: map[string]config.ClientConfig{
			"customer-service": {BaseURL: "http://localhost:1", Secret: []byte("s")},
		},
	}
	set := NewSet(cfg, observability.NewMetrics())
	if !set.Customer.Configured() {
		t.Fatal("customer client should be configured")
	}
	for name, client := range map[string]interface{ Configured() bool }{
		"dispatch": set.Dispatch, "taxi": set.Taxi, "hauling": set.Hauling,
		"payment": set.Payment, "notification": set.Notification, "media": set.Media,
		"verification": set.Verification, "support": set.Support,
		"analytics": set.Analytics, "admin": set.Admin,
	} {
		if client.Configured() {
			t.Fatalf("%s client should be unconfigured", name)
		}
	}
}

func TestMetricsInstrumented(t *testing.T) {
	metrics := observability.NewMetrics()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(Options{ServiceName: "customer-service", BaseURL: server.URL, Secret: []byte("s"), Timeout: time.Second, Metrics: metrics})
	_ = client.Ping(context.Background(), CallMeta{})

	count := metrics.CounterValue("fleet_partner_client_calls_total", map[string]string{
		"service": "customer-service", "method": "GET", "path": "/health", "outcome": "success",
	})
	if count != 1 {
		t.Fatalf("client call counter = %g", count)
	}
}
