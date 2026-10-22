package customerstanding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	caller   = "driver-taxi-service"
	secret   = "standing-secret"
	customer = "11111111-1111-1111-1111-111111111111"
)

// fixedNow is captured once so as_of arithmetic is deterministic while HMAC
// signatures stay within the verifier's clock skew.
var fixedNow = time.Now().UTC()

type answer struct {
	status int
	raw    string
	body   map[string]any
	delay  time.Duration
}

func standingBody(customerID string, eligible bool, restriction string, version int64, asOf time.Time) map[string]any {
	return map[string]any{"success": true, "data": map[string]any{
		"customer_id": customerID, "scopes": []string{"account_standing"},
		"account_standing": map[string]any{"booking_eligible": eligible, "restriction": restriction,
			"standing_version": version, "as_of": asOf.UTC().Format(time.RFC3339Nano)},
		"context_version": "v",
	}}
}

// server verifies the HMAC signature and the requested scope exactly like
// customer-service does, then returns the configured answer.
func server(t *testing.T, a *answer, calls *int32) *httptest.Server {
	t.Helper()
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{caller: []byte(secret)}, 5*time.Minute)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			atomic.AddInt32(calls, 1)
		}
		if name, err := verifier.VerifyRequest(r); err != nil || name != caller {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("scopes") != "account_standing" || !strings.HasPrefix(r.URL.Path, "/api/v1/customer/internal/customers/") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if a.delay > 0 {
			select {
			case <-time.After(a.delay):
			case <-r.Context().Done():
				return
			}
		}
		status := a.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if a.raw != "" {
			_, _ = w.Write([]byte(a.raw))
			return
		}
		_ = json.NewEncoder(w).Encode(a.body)
	}))
}

func client(url string) *Client {
	return &Client{BaseURL: url, ServiceName: caller, Secret: []byte(secret), Now: func() time.Time { return fixedNow }}
}

func assertCode(t *testing.T, err error, sentinel error, code apperrors.Code, status int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %v, got nil", sentinel)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error %v is not %v", err, sentinel)
	}
	app := apperrors.From(err)
	if app.Code != code || app.Status != status {
		t.Fatalf("app error = %s/%d, want %s/%d", app.Code, app.Status, code, status)
	}
}

func TestEligibleCustomerIsAllowed(t *testing.T) {
	a := &answer{body: standingBody(customer, true, "NONE", 3, fixedNow.Add(-time.Second))}
	srv := server(t, a, nil)
	defer srv.Close()
	got, err := client(srv.URL).Check(context.Background(), customer)
	if err != nil {
		t.Fatal(err)
	}
	if !got.BookingEligible || got.Restriction != "NONE" || got.StandingVersion != 3 || got.CustomerID != customer {
		t.Fatalf("standing = %+v", got)
	}
}

func TestRestrictedCustomersAreHeld(t *testing.T) {
	for _, restriction := range []string{"BOOKING_HOLD", "LOGIN_BLOCKED"} {
		t.Run(restriction, func(t *testing.T) {
			srv := server(t, &answer{body: standingBody(customer, false, restriction, 4, fixedNow)}, nil)
			defer srv.Close()
			got, err := client(srv.URL).Check(context.Background(), customer)
			assertCode(t, err, ErrBookingHold, CodeBookingHold, http.StatusForbidden)
			if errors.Is(err, ErrUnavailable) {
				t.Fatal("a hold must not read as unavailable")
			}
			if got.StandingVersion != 4 || got.Restriction != restriction {
				t.Fatalf("hold standing = %+v", got)
			}
			if strings.Contains(apperrors.From(err).Message, restriction) {
				t.Fatal("customer-facing message must not disclose the restriction")
			}
		})
	}
}

func TestFailClosedBranches(t *testing.T) {
	good := func() map[string]any { return standingBody(customer, true, "NONE", 1, fixedNow) }
	cases := map[string]*answer{
		"non-200 5xx":              {status: http.StatusInternalServerError, body: good()},
		"non-200 403":              {status: http.StatusForbidden, body: map[string]any{"success": false}},
		"non-200 404":              {status: http.StatusNotFound, body: map[string]any{"success": false}},
		"malformed json":           {raw: "{not json"},
		"success false":            {body: map[string]any{"success": false, "data": map[string]any{"customer_id": customer}}},
		"missing account_standing": {body: map[string]any{"success": true, "data": map[string]any{"customer_id": customer}}},
		"customer mismatch":        {body: standingBody("22222222-2222-2222-2222-222222222222", true, "NONE", 1, fixedNow)},
		"stale as_of":              {body: standingBody(customer, true, "NONE", 1, fixedNow.Add(-31*time.Second))},
		"future as_of":             {body: standingBody(customer, true, "NONE", 1, fixedNow.Add(10*time.Second))},
		"missing as_of": {body: map[string]any{"success": true, "data": map[string]any{"customer_id": customer,
			"account_standing": map[string]any{"booking_eligible": true, "restriction": "NONE", "standing_version": 1}}}},
		"missing booking_eligible": {body: map[string]any{"success": true, "data": map[string]any{"customer_id": customer,
			"account_standing": map[string]any{"restriction": "NONE", "standing_version": 1, "as_of": fixedNow.Format(time.RFC3339Nano)}}}},
		"contradictory eligible+hold": {body: standingBody(customer, true, "BOOKING_HOLD", 1, fixedNow)},
	}
	for name, a := range cases {
		t.Run(name, func(t *testing.T) {
			srv := server(t, a, nil)
			defer srv.Close()
			got, err := client(srv.URL).Check(context.Background(), customer)
			assertCode(t, err, ErrUnavailable, CodeUnavailable, http.StatusServiceUnavailable)
			if got.BookingEligible {
				t.Fatal("an unavailable answer must never read as eligible")
			}
			if !apperrors.From(err).Retryable {
				t.Fatal("unavailable must be retryable")
			}
		})
	}
}

func TestAsOfWithinToleranceIsAccepted(t *testing.T) {
	for name, asOf := range map[string]time.Time{"29s old": fixedNow.Add(-29 * time.Second), "3s ahead": fixedNow.Add(3 * time.Second)} {
		t.Run(name, func(t *testing.T) {
			srv := server(t, &answer{body: standingBody(customer, true, "NONE", 1, asOf)}, nil)
			defer srv.Close()
			if _, err := client(srv.URL).Check(context.Background(), customer); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWrongSecretIsUnavailable(t *testing.T) {
	srv := server(t, &answer{body: standingBody(customer, true, "NONE", 1, fixedNow)}, nil)
	defer srv.Close()
	c := client(srv.URL)
	c.Secret = []byte("wrong")
	_, err := c.Check(context.Background(), customer)
	assertCode(t, err, ErrUnavailable, CodeUnavailable, http.StatusServiceUnavailable)
}

func TestTimeoutIsUnavailable(t *testing.T) {
	srv := server(t, &answer{delay: 2 * time.Second, body: standingBody(customer, true, "NONE", 1, fixedNow)}, nil)
	defer srv.Close()
	c := client(srv.URL)
	c.Timeout = 50 * time.Millisecond
	started := time.Now()
	_, err := c.Check(context.Background(), customer)
	assertCode(t, err, ErrUnavailable, CodeUnavailable, http.StatusServiceUnavailable)
	if time.Since(started) > time.Second {
		t.Fatal("timeout was not honoured")
	}
}

func TestTransportErrorIsUnavailable(t *testing.T) {
	srv := server(t, &answer{}, nil)
	url := srv.URL
	srv.Close()
	_, err := client(url).Check(context.Background(), customer)
	assertCode(t, err, ErrUnavailable, CodeUnavailable, http.StatusServiceUnavailable)
}

func TestUnconfiguredClientAndEmptyCustomerAreUnavailable(t *testing.T) {
	var nilClient *Client
	_, err := nilClient.Check(context.Background(), customer)
	assertCode(t, err, ErrUnavailable, CodeUnavailable, http.StatusServiceUnavailable)
	_, err = (&Client{BaseURL: "http://x"}).Check(context.Background(), customer)
	assertCode(t, err, ErrUnavailable, CodeUnavailable, http.StatusServiceUnavailable)
	_, err = client("http://127.0.0.1:1").Check(context.Background(), "  ")
	assertCode(t, err, ErrUnavailable, CodeUnavailable, http.StatusServiceUnavailable)
}

func TestNoCachingEveryCheckAsksTheOwner(t *testing.T) {
	var calls int32
	a := &answer{body: standingBody(customer, false, "BOOKING_HOLD", 1, fixedNow)}
	srv := server(t, a, &calls)
	defer srv.Close()
	c := client(srv.URL)
	if _, err := c.Check(context.Background(), customer); !errors.Is(err, ErrBookingHold) {
		t.Fatal("expected hold", err)
	}
	a.body = standingBody(customer, true, "NONE", 2, fixedNow)
	got, err := c.Check(context.Background(), customer)
	if err != nil || got.StandingVersion != 2 {
		t.Fatalf("lifted hold not observed immediately: %+v %v", got, err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestCustomerIDIsPathEscaped(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, _ = client(srv.URL).Check(context.Background(), "a/b?scopes=mobility_preferences")
	if !strings.HasSuffix(seen, "/a%2Fb%3Fscopes=mobility_preferences/context") {
		t.Fatalf("customer id not escaped: %s", seen)
	}
}

func TestNewClientValidation(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{BaseURL: "http://localhost:8101", ServiceName: caller},
		{BaseURL: "http://localhost:8101", Secret: []byte("s")},
		{BaseURL: "localhost:8101", ServiceName: caller, Secret: []byte("s")},
		{BaseURL: "http://localhost:8101/api/v1/customer", ServiceName: caller, Secret: []byte("s")},
	} {
		if _, err := NewClient(cfg); err == nil {
			t.Fatalf("config %+v accepted", cfg)
		}
	}
	if _, err := NewClient(Config{BaseURL: "http://localhost:8101/", ServiceName: caller, Secret: []byte("s")}); err != nil {
		t.Fatal(err)
	}
}

type fakeChecker struct {
	standing Standing
	err      error
}

func (f fakeChecker) Check(context.Context, string) (Standing, error) { return f.standing, f.err }

func TestGate(t *testing.T) {
	var disabled *Gate
	if disabled.Enabled() {
		t.Fatal("nil gate enabled")
	}
	if NewGate(nil) != nil {
		t.Fatal("NewGate(nil) must be nil")
	}
	if _, err := disabled.Allow(context.Background(), customer); err != nil {
		t.Fatal("disabled gate must allow", err)
	}
	if _, err := NewGate(fakeChecker{standing: Standing{BookingEligible: true}}).Allow(context.Background(), customer); err != nil {
		t.Fatal(err)
	}
	_, err := NewGate(fakeChecker{standing: Standing{Restriction: "BOOKING_HOLD"}}).Allow(context.Background(), customer)
	assertCode(t, err, ErrBookingHold, CodeBookingHold, http.StatusForbidden)
	_, err = NewGate(fakeChecker{err: fmt.Errorf("boom")}).Allow(context.Background(), customer)
	assertCode(t, err, ErrUnavailable, CodeUnavailable, http.StatusServiceUnavailable)
	_, err = NewGate(fakeChecker{err: BookingHold(Standing{})}).Allow(context.Background(), customer)
	assertCode(t, err, ErrBookingHold, CodeBookingHold, http.StatusForbidden)
}
