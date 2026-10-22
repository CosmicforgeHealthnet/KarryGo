package booking

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/customerstanding"
	"cosmicforge/logistics/shared/go/customerstanding/standingtest"
)

type standingHarness struct {
	service   *Service
	repo      *fakeRepo
	broadcast *fakeBroadcast
	payments  *fakePayments
	observer  *fakeObserver
	srv       *standingtest.Server
}

func newStandingHarness(t *testing.T) standingHarness {
	t.Helper()
	repo := newFakeRepo()
	h := standingHarness{repo: repo, broadcast: &fakeBroadcast{repo: repo}, payments: &fakePayments{}, observer: &fakeObserver{},
		srv: standingtest.New(t, "driver-dispatch-delivery-service")}
	h.service = newTestService(repo, h.broadcast, &fakeTrips{}, &fakeLive{}, h.payments).
		WithCustomerStanding(h.srv.Gate(300 * time.Millisecond))
	h.service.WithServiceabilityObserver(h.observer)
	return h
}

func (h standingHarness) assertNoSideEffects(t *testing.T) {
	t.Helper()
	h.repo.mu.Lock()
	rows := len(h.repo.bookings)
	h.repo.mu.Unlock()
	if rows != 0 || len(h.payments.holds) != 0 || len(h.broadcast.started) != 0 || h.observer.called {
		t.Fatalf("side effects: rows=%d holds=%d broadcasts=%d observed=%v", rows, len(h.payments.holds), len(h.broadcast.started), h.observer.called)
	}
}

func assertStandingCode(t *testing.T, err error, code apperrors.Code, status int) {
	t.Helper()
	app := apperrors.From(err)
	if app == nil || app.Code != code || app.Status != status {
		t.Fatalf("error = %v, want %s/%d", err, code, status)
	}
}

func TestCustomerStandingHeldCustomerCreatesNothing(t *testing.T) {
	for _, restriction := range []string{"BOOKING_HOLD", "LOGIN_BLOCKED"} {
		t.Run(restriction, func(t *testing.T) {
			h := newStandingHarness(t)
			h.srv.Hold(testCustomer, restriction, 2)
			_, err := h.service.CreateBookingWithKey(context.Background(), testCustomer, "key-1", validCreateInput())
			assertStandingCode(t, err, customerstanding.CodeBookingHold, http.StatusForbidden)
			h.assertNoSideEffects(t)
		})
	}
}

func TestCustomerStandingOutagesFailClosed(t *testing.T) {
	cases := map[string]func(*standingtest.Server){
		"5xx":     func(s *standingtest.Server) { s.FailWith(http.StatusServiceUnavailable) },
		"timeout": func(s *standingtest.Server) { s.Delay(2 * time.Second) },
		"stale":   func(s *standingtest.Server) { s.StaleBy(time.Minute) },
		"down":    func(s *standingtest.Server) { s.Close() },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			h := newStandingHarness(t)
			breakIt(h.srv)
			_, err := h.service.CreateBooking(context.Background(), testCustomer, validCreateInput())
			assertStandingCode(t, err, customerstanding.CodeUnavailable, http.StatusServiceUnavailable)
			h.assertNoSideEffects(t)
		})
	}
}

func TestCustomerStandingEligibleProceedsAndLiftIsImmediate(t *testing.T) {
	h := newStandingHarness(t)
	h.srv.Hold(testCustomer, "BOOKING_HOLD", 1)
	if _, err := h.service.CreateBooking(context.Background(), testCustomer, validCreateInput()); !errors.Is(err, customerstanding.ErrBookingHold) {
		t.Fatalf("held: %v", err)
	}
	h.srv.Lift(testCustomer, 2)
	view, err := h.service.CreateBooking(context.Background(), testCustomer, validCreateInput())
	if err != nil {
		t.Fatalf("lifted hold still refused: %v", err)
	}
	if view.Status != WireSearching || len(h.payments.holds) != 1 || len(h.broadcast.started) != 1 {
		t.Fatalf("eligible booking: status=%s holds=%v broadcasts=%v", view.Status, h.payments.holds, h.broadcast.started)
	}
}

func TestCustomerStandingConcurrentHeldAttemptsAllRefused(t *testing.T) {
	h := newStandingHarness(t)
	h.srv.Hold(testCustomer, "BOOKING_HOLD", 1)
	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = h.service.CreateBookingWithKey(context.Background(), testCustomer, "", validCreateInput())
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, customerstanding.ErrBookingHold) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	h.assertNoSideEffects(t)
	if h.srv.Calls() != n {
		t.Fatalf("standing calls = %d, want %d", h.srv.Calls(), n)
	}
}

func TestCustomerStandingExistingBookingContinuesAfterHold(t *testing.T) {
	h := newStandingHarness(t)
	view, err := h.service.CreateBooking(context.Background(), testCustomer, validCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	h.srv.Hold(testCustomer, "BOOKING_HOLD", 1)
	calls := h.srv.Calls()
	if _, err := h.service.GetBooking(context.Background(), testCustomer, view.ID); err != nil {
		t.Fatalf("read existing: %v", err)
	}
	cancelled, err := h.service.CancelBooking(context.Background(), testCustomer, view.ID, "changed_mind")
	if err != nil || cancelled.Status != WireCancelled {
		t.Fatalf("existing booking cancel blocked by hold: %v", err)
	}
	if len(h.payments.refunds) != 1 {
		t.Fatalf("refund not issued for pre-hold booking: %v", h.payments.refunds)
	}
	if h.srv.Calls() != calls {
		t.Fatal("existing booking lifecycle consulted standing")
	}
}

func TestCustomerStandingDisabledNeverAsks(t *testing.T) {
	repo := newFakeRepo()
	srv := standingtest.New(t, "driver-dispatch-delivery-service")
	srv.Hold(testCustomer, "BOOKING_HOLD", 1)
	service := newTestService(repo, &fakeBroadcast{repo: repo}, &fakeTrips{}, &fakeLive{}, &fakePayments{})
	if _, err := service.CreateBooking(context.Background(), testCustomer, validCreateInput()); err != nil {
		t.Fatal(err)
	}
	if srv.Calls() != 0 {
		t.Fatal("disabled enforcement contacted customer-service")
	}
}
