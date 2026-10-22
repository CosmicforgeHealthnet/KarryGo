// Package standingtest is an in-process stand-in for customer-service's
// internal context route answering the account_standing scope. It verifies
// the real HMAC signature and returns the real wire contract, so booking
// authorities can test enforcement through the real customerstanding.Client.
// Test-only: never import it from production code.
package standingtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/customerstanding"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Secret is the HMAC secret the fake trusts for its configured caller.
const Secret = "standingtest-secret"

type record struct {
	restriction string
	version     int64
}

// Server is a fake customer-service standing endpoint.
type Server struct {
	*httptest.Server
	caller string

	mu       sync.Mutex
	standing map[string]record
	status   int
	delay    time.Duration
	asOfAge  time.Duration
	calls    int64
}

// New starts a fake trusting caller. Unknown customers are eligible
// (restriction NONE, version 0), exactly like customer-service.
func New(t testing.TB, caller string) *Server {
	t.Helper()
	s := &Server{caller: caller, standing: map[string]record{}}
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{caller: []byte(Secret)}, 5*time.Minute)
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&s.calls, 1)
		if name, err := verifier.VerifyRequest(r); err != nil || name != caller {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		s.mu.Lock()
		status, delay, age := s.status, s.delay, s.asOfAge
		s.mu.Unlock()
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":"service_unavailable"}}`))
			return
		}
		const prefix = "/api/v1/customer/internal/customers/"
		if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, "/context") || r.URL.Query().Get("scopes") != "account_standing" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		customerID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/context")
		s.mu.Lock()
		rec, ok := s.standing[customerID]
		s.mu.Unlock()
		if !ok {
			rec = record{restriction: "NONE"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
			"customer_id": customerID, "scopes": []string{"account_standing"},
			"account_standing": map[string]any{"booking_eligible": rec.restriction == "NONE", "restriction": rec.restriction,
				"standing_version": rec.version, "as_of": time.Now().Add(-age).UTC().Format(time.RFC3339Nano)},
			"context_version": "standingtest",
		}})
	}))
	t.Cleanup(s.Close)
	return s
}

// Hold places customerID under restriction ("BOOKING_HOLD" or
// "LOGIN_BLOCKED") at version.
func (s *Server) Hold(customerID, restriction string, version int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.standing[customerID] = record{restriction: restriction, version: version}
}

// Lift restores customerID to NONE at version.
func (s *Server) Lift(customerID string, version int64) { s.Hold(customerID, "NONE", version) }

// FailWith makes every answer use status (0 restores normal answers).
func (s *Server) FailWith(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

// Delay makes every answer wait d (or until the caller gives up).
func (s *Server) Delay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delay = d
}

// StaleBy makes as_of lag the real clock by age.
func (s *Server) StaleBy(age time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asOfAge = age
}

// Calls is the number of requests received.
func (s *Server) Calls() int64 { return atomic.LoadInt64(&s.calls) }

// Client returns a real customerstanding.Client signed as the trusted caller.
func (s *Server) Client(timeout time.Duration) *customerstanding.Client {
	return &customerstanding.Client{BaseURL: s.URL, ServiceName: s.caller, Secret: []byte(Secret), Timeout: timeout}
}

// Gate returns an enforcing gate backed by Client(timeout).
func (s *Server) Gate(timeout time.Duration) *customerstanding.Gate {
	return customerstanding.NewGate(s.Client(timeout))
}
