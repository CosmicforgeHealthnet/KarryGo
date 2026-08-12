package platformclients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/circuitbreaker"
)

// Section 29: retry classification, backoff, Retry-After and write-safety
// gating, all exercised through the real Client.Exchange path (not a
// reimplementation) so what is tested is what production actually runs.

func fastRetryOptions(serviceName, url string) Options {
	return Options{
		ServiceName: serviceName, BaseURL: url, Secret: []byte("s"), Timeout: time.Second,
		MaxRetries: 3, RetryBackoffBase: 5 * time.Millisecond, RetryBackoffMax: 50 * time.Millisecond,
	}
}

func TestRetry_TransientGETRetriedUntilSuccess(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(fastRetryOptions("customer-service", server.URL))
	err := client.Ping(context.Background(), CallMeta{})
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Fatalf("hits = %d, want exactly 3 (2 failures + 1 success)", hits)
	}
}

func TestRetry_ValidationErrorNeverRetried(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := New(fastRetryOptions("customer-service", server.URL))
	err := client.Ping(context.Background(), CallMeta{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits = %d, want exactly 1 — a caller validation error must never be retried", hits)
	}
}

func TestRetry_WriteWithoutIdempotencyKeyNotRetried(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := New(fastRetryOptions("customer-service", server.URL))
	err := client.Do(context.Background(), CallMeta{}, http.MethodPost, "/things", map[string]string{"a": "b"}, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits = %d, want exactly 1 — a write without an idempotency key must never be auto-retried", hits)
	}
}

func TestRetry_WriteWithIdempotencyKeyIsRetried(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(fastRetryOptions("customer-service", server.URL))
	err := client.Do(context.Background(), CallMeta{IdempotencyKey: "idem-1"}, http.MethodPost, "/things", map[string]string{"a": "b"}, nil)
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("hits = %d, want exactly 2 — an idempotent write IS safe to auto-retry on a transient outcome", hits)
	}
}

func TestRetry_ExplicitReadBackContractDisablesWriteRetry(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := New(fastRetryOptions("dispatch-delivery-service", server.URL))
	err := client.Exchange(context.Background(), CallMeta{IdempotencyKey: "execution-1"}, Exchange{
		Method:         http.MethodPost,
		Path:           "/executions",
		JSONBody:       map[string]string{"assignmentId": "assignment-1"},
		DisableRetries: true,
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits = %d, want exactly 1 before authoritative read-back", hits)
	}
}

func TestRetry_UnknownOutcomeNeverAutoRetried(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	opts := fastRetryOptions("customer-service", server.URL)
	opts.Timeout = 20 * time.Millisecond
	client := New(opts)
	err := client.Do(context.Background(), CallMeta{IdempotencyKey: "idem-1"}, http.MethodPost, "/things", map[string]string{"a": "b"}, nil)
	clientErr, ok := err.(*Error)
	if !ok || clientErr.Outcome != OutcomeUnknown {
		t.Fatalf("expected OutcomeUnknown, got %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits = %d, want exactly 1 — an UNKNOWN outcome (timeout after transmission) must never be blindly retried", hits)
	}
}

func TestRetry_ExhaustsBoundedAttempts(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	opts := fastRetryOptions("customer-service", server.URL)
	opts.MaxRetries = 2
	client := New(opts)
	err := client.Ping(context.Background(), CallMeta{})
	if err == nil {
		t.Fatal("expected the call to still fail after exhausting retries")
	}
	if atomic.LoadInt32(&hits) != 3 { // 1 initial + 2 retries, bounded
		t.Fatalf("hits = %d, want exactly 3 (initial + MaxRetries)", hits)
	}
}

func TestRetry_RespectsRetryAfterHeader(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Backoff base/max are tiny (would retry near-instantly on its own), so
	// an observed ~1s delay can only come from honoring Retry-After.
	opts := fastRetryOptions("customer-service", server.URL)
	opts.RetryBackoffBase = 1 * time.Millisecond
	opts.RetryBackoffMax = 3 * time.Second
	client := New(opts)

	start := time.Now()
	err := client.Ping(context.Background(), CallMeta{})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("elapsed = %s, want >= ~1s honoring Retry-After: 1", elapsed)
	}
}

func TestRetry_ContextCancellationStopsRetryLoop(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	opts := fastRetryOptions("customer-service", server.URL)
	opts.RetryBackoffBase = 200 * time.Millisecond
	opts.RetryBackoffMax = 200 * time.Millisecond
	client := New(opts)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := client.Ping(ctx, CallMeta{})
	if err == nil {
		t.Fatal("expected an error once the context is cancelled")
	}
	if atomic.LoadInt32(&hits) >= 4 {
		t.Fatalf("hits = %d, cancellation should have stopped the retry loop well before exhausting attempts", hits)
	}
}

func TestCircuitBreaker_OpensAfterThresholdAndBlocksFurtherCalls(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var transitions []circuitbreaker.Transition
	opts := Options{
		ServiceName: "customer-service", BaseURL: server.URL, Secret: []byte("s"), Timeout: time.Second,
		MaxRetries:          0, // isolate breaker behavior from the retry loop
		Breaker:             circuitbreaker.Config{FailureThreshold: 2, ObservationWindow: time.Minute, OpenDuration: time.Minute, HalfOpenProbes: 1, SuccessThreshold: 1},
		OnBreakerTransition: func(tr circuitbreaker.Transition) { transitions = append(transitions, tr) },
	}
	client := New(opts)

	for i := 0; i < 2; i++ {
		_ = client.Ping(context.Background(), CallMeta{})
	}
	if client.BreakerState() != circuitbreaker.Open {
		t.Fatalf("breaker state = %s, want OPEN after 2 failures at threshold 2", client.BreakerState())
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("hits = %d, want exactly 2 before the breaker opened", hits)
	}

	err := client.Ping(context.Background(), CallMeta{})
	if err == nil {
		t.Fatal("expected the open breaker to refuse the call")
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("hits = %d, an OPEN breaker must refuse without a network attempt", hits)
	}
	if len(transitions) != 1 || transitions[0].To != circuitbreaker.Open {
		t.Fatalf("expected one CLOSED->OPEN transition, got %+v", transitions)
	}
}

func TestCircuitBreaker_HalfOpenRecoversToClose(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts := Options{
		ServiceName: "customer-service", BaseURL: server.URL, Secret: []byte("s"), Timeout: time.Second,
		MaxRetries: 0,
		Breaker:    circuitbreaker.Config{FailureThreshold: 2, ObservationWindow: time.Minute, OpenDuration: 20 * time.Millisecond, HalfOpenProbes: 1, SuccessThreshold: 1},
	}
	client := New(opts)
	for i := 0; i < 2; i++ {
		_ = client.Ping(context.Background(), CallMeta{})
	}
	if client.BreakerState() != circuitbreaker.Open {
		t.Fatal("breaker should be OPEN after 2 failures")
	}

	time.Sleep(30 * time.Millisecond)
	fail.Store(false)
	if err := client.Ping(context.Background(), CallMeta{}); err != nil {
		t.Fatalf("HALF_OPEN probe should have succeeded: %v", err)
	}
	if client.BreakerState() != circuitbreaker.Closed {
		t.Fatalf("breaker state = %s, want CLOSED after a successful HALF_OPEN probe", client.BreakerState())
	}
}

func TestCircuitBreaker_ValidationErrorsNeverTripIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	opts := Options{
		ServiceName: "customer-service", BaseURL: server.URL, Secret: []byte("s"), Timeout: time.Second,
		MaxRetries: 0,
		Breaker:    circuitbreaker.Config{FailureThreshold: 2, ObservationWindow: time.Minute, OpenDuration: time.Minute, HalfOpenProbes: 1, SuccessThreshold: 1},
	}
	client := New(opts)
	for i := 0; i < 10; i++ {
		_ = client.Ping(context.Background(), CallMeta{})
	}
	if client.BreakerState() != circuitbreaker.Closed {
		t.Fatalf("breaker state = %s, want CLOSED — caller validation errors must never count as dependency failures", client.BreakerState())
	}
}
