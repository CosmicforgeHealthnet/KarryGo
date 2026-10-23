package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	"cosmicforge/logistics/shared/go/customerstanding"
	"cosmicforge/logistics/shared/go/customerstanding/standingtest"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func standingOrderService(t *testing.T, f section17EFixture, gate *customerstanding.Gate) *orders.Service {
	t.Helper()
	evaluator := staffusecases.NewAuthorizationEvaluator(staffrepositories.NewAssignmentRepository(f.env.pool))
	s, err := orders.NewService(orders.Options{Store: orders.NewStore(f.env.pool), Quotes: f.quotes, Scopes: section17EScopeAdapter{evaluator: evaluator}, Enabled: true, CustomerStanding: gate})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestVendorCustomerStandingSignedPostgresGuardAndCommittedReadback(t *testing.T) {
	f := newSection17EFixture(t, true)
	server := standingtest.New(t, "vendor-service")
	s := standingOrderService(t, f, server.Gate(time.Second))
	ctx := context.Background()
	for _, restriction := range []string{"BOOKING_HOLD", "LOGIN_BLOCKED"} {
		server.Hold(f.customerID, restriction, 2)
		if _, _, err := s.CreateFromQuote(ctx, f.customerID, f.quoteID, f.finger); !errors.Is(err, customerstanding.ErrBookingHold) {
			t.Fatalf("%s: %v", restriction, err)
		}
	}
	server.Lift(f.customerID, 3)
	server.StaleBy(time.Minute)
	if _, _, err := s.CreateFromQuote(ctx, f.customerID, f.quoteID, f.finger); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatal("stale standing allowed", err)
	}
	server.StaleBy(0)
	server.FailWith(http.StatusServiceUnavailable)
	if _, _, err := s.CreateFromQuote(ctx, f.customerID, f.quoteID, f.finger); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatal("outage allowed", err)
	}
	var count int
	if err := f.env.pool.QueryRow(ctx, `SELECT count(*) FROM commerce_parent_orders WHERE quote_id=$1`, f.quoteID).Scan(&count); err != nil || count != 0 {
		t.Fatal("refusal created order", count, err)
	}
	server.FailWith(0)
	bad := server.Client(time.Second)
	bad.Secret = []byte("untrusted-standing-key")
	if _, _, err := standingOrderService(t, f, customerstanding.NewGate(bad)).CreateFromQuote(ctx, f.customerID, f.quoteID, f.finger); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatal("unsigned authority allowed", err)
	}
	before := server.Calls()
	if _, _, err := s.CreateFromQuote(ctx, "foreign-customer", f.quoteID, f.finger); err == nil {
		t.Fatal("foreign customer ordered")
	}
	if server.Calls() != before {
		t.Fatal("unowned quote used to query standing")
	}
	var wg sync.WaitGroup
	ids := make(chan string, 6)
	failures := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			order, _, err := s.CreateFromQuote(ctx, f.customerID, f.quoteID, f.finger)
			ids <- order.ID
			failures <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	id := ""
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for got := range ids {
		if id == "" {
			id = got
		}
		if got == "" || got != id {
			t.Fatal("duplicate order", got, id)
		}
	}
	// The quote may expire and standing may change after this committed order.
	server.Hold(f.customerID, "BOOKING_HOLD", 4)
	server.FailWith(503)
	before = server.Calls()
	existing, created, err := s.CreateFromQuote(ctx, f.customerID, f.quoteID, f.finger)
	if err != nil || created || existing.ID != id || server.Calls() != before {
		t.Fatal("committed readback interrupted by hold/outage", created, err)
	}
	if _, _, err := s.CreateFromQuote(ctx, f.customerID, f.quoteID, "wrong-fingerprint"); !errors.Is(err, orders.ErrFingerprintMismatch) {
		t.Fatal("readback agreement changed", err)
	}
}

func TestVendorCustomerStandingLiftIsNotCached(t *testing.T) {
	f := newSection17EFixture(t, false)
	server := standingtest.New(t, "vendor-service")
	s := standingOrderService(t, f, server.Gate(time.Second))
	ctx := context.Background()
	server.Hold(f.customerID, "BOOKING_HOLD", 1)
	if _, _, err := s.CreateFromQuote(ctx, f.customerID, f.quoteID, f.finger); !errors.Is(err, customerstanding.ErrBookingHold) {
		t.Fatal(err)
	}
	server.Lift(f.customerID, 2)
	if _, created, err := s.CreateFromQuote(ctx, f.customerID, f.quoteID, f.finger); err != nil || !created {
		t.Fatal("lift not current", created, err)
	}
}

func TestVendorStandingNeverFollowsCredentialRedirect(t *testing.T) {
	var forwarded atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1); w.WriteHeader(200) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	gate, err := configuredCustomerStanding(config.Config{CustomerStandingEnforced: true, CustomerStandingURL: origin.URL, CustomerStandingSecret: "test-only-standing-workload-secret-32"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = gate.Allow(context.Background(), "test-customer"); !errors.Is(err, customerstanding.ErrUnavailable) || forwarded.Load() != 0 {
		t.Fatal("standing redirect did not fail closed", forwarded.Load(), err)
	}
}
