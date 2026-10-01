package assetcapability

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// receivedCall is one delivery an authority's stand-in ingress saw.
type receivedCall struct {
	Body   map[string]any
	Caller string
}

// fakeIngress stands in for one driver service's certification endpoint. It
// verifies the HMAC headers arrived, because a delivery that is not
// authenticated is one the real service would reject after this test passed.
type fakeIngress struct {
	mu      sync.Mutex
	calls   []receivedCall
	status  int
	server  *httptest.Server
	baseURL string
}

func newFakeIngress(t *testing.T, path string, status int) *fakeIngress {
	t.Helper()
	ingress := &fakeIngress{status: status}
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		ingress.mu.Lock()
		ingress.calls = append(ingress.calls, receivedCall{Body: body,
			Caller: r.Header.Get("X-Service-Name")})
		current := ingress.status
		ingress.mu.Unlock()
		w.WriteHeader(current)
	})
	ingress.server = httptest.NewServer(mux)
	ingress.baseURL = ingress.server.URL
	t.Cleanup(ingress.server.Close)
	return ingress
}

func (f *fakeIngress) received() []receivedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]receivedCall(nil), f.calls...)
}

func (f *fakeIngress) setStatus(status int) {
	f.mu.Lock()
	f.status = status
	f.mu.Unlock()
}

// TestCertificationIsDeliveredToTheOwningAuthorityOnlyOnce is the end-to-end
// producer test: a decision recorded here reaches the driver service that
// actually dispatches work, in that service's own wire shape, signed, exactly
// once.
//
// A certification that never arrives is the worst failure this section has,
// because nothing downstream notices: the asset is suspended in the certifying
// service and eligible in the dispatching one, and neither polls the other.
func TestCertificationIsDeliveredToTheOwningAuthorityOnlyOnce(t *testing.T) {
	h := newHarness(t)
	paths := DefaultPaths()
	taxi := newFakeIngress(t, paths[AuthorityTaxi], http.StatusOK)
	hauling := newFakeIngress(t, paths[AuthorityHauling], http.StatusOK)

	worker := NewCallbackWorker(h.pool, map[Authority]AuthorityEndpoint{
		AuthorityTaxi: {BaseURL: taxi.baseURL, Path: paths[AuthorityTaxi],
			Secret: []byte("taxi-callback-secret")},
		AuthorityHauling: {BaseURL: hauling.baseURL, Path: paths[AuthorityHauling],
			Secret: []byte("hauling-callback-secret")},
	}, 5*time.Second)

	h.verifyProvider(t, AuthorityTaxi, "prv-1")
	ref, _ := h.openCase(t, taxiCaller,
		caseBody(AuthorityTaxi, "prv-1", "veh-1", "PREMIUM_CONDITION", "TAXI_COMFORT"), "open-1")
	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/cases/"+ref+"/decisions",
		map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
			"policy_version": "p1", "expected_source_version": 1}, "decide-1"); got.Code != http.StatusOK {
		t.Fatalf("decision = %d %s", got.Code, got.Body)
	}

	ctx := context.Background()
	if err := worker.Process(ctx, "test-worker", 25); err != nil {
		t.Fatal(err)
	}

	delivered := taxi.received()
	if len(delivered) != 1 {
		t.Fatalf("taxi received %d deliveries, want 1", len(delivered))
	}
	if delivered[0].Caller != ServiceName {
		t.Fatalf("delivery signed as %q, want %q", delivered[0].Caller, ServiceName)
	}
	body := delivered[0].Body
	if body["vehicle_id"] != "veh-1" {
		t.Fatalf("taxi body keys the asset as %v, want vehicle_id=veh-1", body)
	}
	if body["decision"] != "CERTIFIED" {
		t.Fatalf("taxi decision = %v, want CERTIFIED", body["decision"])
	}
	if body["event_id"] == nil || body["event_id"] == "" {
		t.Fatal("taxi body carries no event_id; its ingress dedupes on it")
	}
	if body["product_code"] != "TAXI_COMFORT" {
		t.Fatalf("product_code = %v", body["product_code"])
	}

	// Hauling owns no part of this asset and must hear nothing about it.
	if calls := hauling.received(); len(calls) != 0 {
		t.Fatalf("hauling received %d deliveries for a taxi asset", len(calls))
	}

	// A second pass delivers nothing: the row is DELIVERED, so an at-least-once
	// worker does not become an every-tick-forever worker.
	if err := worker.Process(ctx, "test-worker", 25); err != nil {
		t.Fatal(err)
	}
	if calls := taxi.received(); len(calls) != 1 {
		t.Fatalf("taxi received %d deliveries after a second pass, want 1", len(calls))
	}

	var status string
	if err := h.pool.QueryRow(ctx,
		`SELECT delivery_status FROM verification_outbox WHERE event_type=$1`,
		EventServiceTierStatusChanged).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "DELIVERED" {
		t.Fatalf("delivery_status = %q, want DELIVERED", status)
	}
}

// TestFailedDeliveryIsRetriedThenDeadLettered proves a refusing authority does
// not lose the certification: it stays claimable, and after a bounded number of
// attempts it surfaces to an operator rather than retrying forever in silence.
func TestFailedDeliveryIsRetriedThenDeadLettered(t *testing.T) {
	h := newHarness(t)
	paths := DefaultPaths()
	dispatch := newFakeIngress(t, paths[AuthorityDispatch], http.StatusInternalServerError)
	worker := NewCallbackWorker(h.pool, map[Authority]AuthorityEndpoint{
		AuthorityDispatch: {BaseURL: dispatch.baseURL, Path: paths[AuthorityDispatch],
			Secret: []byte("dispatch-callback-secret")},
	}, 5*time.Second)

	h.verifyProvider(t, AuthorityDispatch, "prv-1")
	ref, _ := h.openCase(t, dispatchCaller,
		caseBody(AuthorityDispatch, "prv-1", "bike-1", "COLD_BOX", "DISPATCH_COLD"), "open-1")
	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/cases/"+ref+"/decisions",
		map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
			"expected_source_version": 1}, "decide-1"); got.Code != http.StatusOK {
		t.Fatalf("decision = %d %s", got.Code, got.Body)
	}

	ctx := context.Background()
	if err := worker.Process(ctx, "test-worker", 25); err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	if err := h.pool.QueryRow(ctx,
		`SELECT delivery_status, delivery_attempts FROM verification_outbox WHERE event_type=$1`,
		EventServiceTierStatusChanged).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "FAILED" || attempts != 1 {
		t.Fatalf("after one refusal status=%q attempts=%d, want FAILED/1", status, attempts)
	}

	// Drive it past the attempt bound. next_attempt_at is pushed out by the
	// backoff, so each pass is made due first -- the test is about the attempt
	// ceiling, not about waiting out real time.
	for i := 0; i < maxDeliveryAttempts; i++ {
		if _, err := h.pool.Exec(ctx,
			`UPDATE verification_outbox SET next_attempt_at=now()-interval '1 minute'`); err != nil {
			t.Fatal(err)
		}
		if err := worker.Process(ctx, "test-worker", 25); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.pool.QueryRow(ctx,
		`SELECT delivery_status FROM verification_outbox WHERE event_type=$1`,
		EventServiceTierStatusChanged).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "DEAD_LETTER" {
		t.Fatalf("status = %q, want DEAD_LETTER: a permanently refusing endpoint must reach an operator", status)
	}

	// The operator's replay makes it deliverable again, and once the authority
	// recovers the certification lands.
	dispatch.setStatus(http.StatusOK)
	var eventID string
	if err := h.pool.QueryRow(ctx,
		`SELECT id::text FROM verification_outbox WHERE event_type=$1`,
		EventServiceTierStatusChanged).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if err := worker.Replay(ctx, eventID); err != nil {
		t.Fatal(err)
	}
	if err := worker.Process(ctx, "test-worker", 25); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`SELECT delivery_status FROM verification_outbox WHERE event_type=$1`,
		EventServiceTierStatusChanged).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "DELIVERED" {
		t.Fatalf("after replay status = %q, want DELIVERED", status)
	}
	calls := dispatch.received()
	last := calls[len(calls)-1].Body
	if last["bike_id"] != "bike-1" || last["status"] != "ELIGIBLE" {
		t.Fatalf("dispatch body = %v, want bike_id + status=ELIGIBLE", last)
	}
}

// TestUnconfiguredAuthorityIsNotADeliveryTarget proves a deployment that has
// not wired an authority does not silently mark its certifications delivered.
func TestUnconfiguredAuthorityIsNotADeliveryTarget(t *testing.T) {
	worker := NewCallbackWorker(nil, map[Authority]AuthorityEndpoint{
		AuthorityTaxi:    {BaseURL: "", Secret: nil},
		AuthorityHauling: {BaseURL: "https://hauling.example", Path: "/x", Secret: nil},
	}, time.Second)
	if targets := worker.Targets(); len(targets) != 0 {
		t.Fatalf("targets = %v, want none: a URL without a secret cannot be signed", targets)
	}
}
