package bookingusecases

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/customerstanding"
	"cosmicforge/logistics/shared/go/customerstanding/standingtest"
)

// Administrative account standing (customer-service owned) is enforced on
// every NEW customer booking, before any read-for-write, quote or insert.

func newStandingService(t *testing.T) (*BookingService, *fakeBookingRepo, *fakePricing, *standingtest.Server) {
	t.Helper()
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	srv := standingtest.New(t, "driver-taxi-service")
	svc := NewBookingService(Options{
		Bookings: repo, Markets: markets, Pricing: pc, OperationalState: fakeOperationalState{},
		CustomerStanding: srv.Gate(300 * time.Millisecond),
	})
	return svc, repo, pc, srv
}

func immediateInput(customerID string) CreateBookingInput {
	return CreateBookingInput{CustomerID: customerID, VehicleClass: "economy", Stops: validStops()}
}

func assertNoBookingSideEffects(t *testing.T, repo *fakeBookingRepo, pc *fakePricing) {
	t.Helper()
	if repo.createdSeq != 0 || len(repo.byID) != 0 {
		t.Fatalf("booking rows written: %d", repo.createdSeq)
	}
	if len(pc.lastReq.VehicleClasses) != 0 {
		t.Fatalf("a fare quote was requested: %+v", pc.lastReq)
	}
}

func assertAppCode(t *testing.T, err error, code apperrors.Code, status int) {
	t.Helper()
	app := apperrors.From(err)
	if app == nil || app.Code != code || app.Status != status {
		t.Fatalf("error = %v (%+v), want %s/%d", err, app, code, status)
	}
}

func TestCustomerStanding_HeldCustomerCreatesNothing(t *testing.T) {
	for _, restriction := range []string{"BOOKING_HOLD", "LOGIN_BLOCKED"} {
		t.Run(restriction, func(t *testing.T) {
			svc, repo, pc, srv := newStandingService(t)
			srv.Hold("cust-held", restriction, 3)
			_, err := svc.CreateBooking(context.Background(), immediateInput("cust-held"))
			assertAppCode(t, err, customerstanding.CodeBookingHold, http.StatusForbidden)
			assertNoBookingSideEffects(t, repo, pc)
		})
	}
}

func TestCustomerStanding_ScheduledAndParcelBookingsAreGated(t *testing.T) {
	svc, repo, pc, srv := newStandingService(t)
	srv.Hold("cust-held", "BOOKING_HOLD", 1)
	future := time.Now().Add(2 * time.Hour)
	scheduled := immediateInput("cust-held")
	scheduled.ScheduledAt = &future
	_, err := svc.CreateBooking(context.Background(), scheduled)
	assertAppCode(t, err, customerstanding.CodeBookingHold, http.StatusForbidden)
	parcel := parcelInput(func(in *CreateBookingInput) { in.CustomerID = "cust-held" })
	_, err = svc.CreateBooking(context.Background(), parcel)
	assertAppCode(t, err, customerstanding.CodeBookingHold, http.StatusForbidden)
	assertNoBookingSideEffects(t, repo, pc)
}

func TestCustomerStanding_OutagesFailClosed(t *testing.T) {
	cases := map[string]func(*standingtest.Server){
		"5xx":     func(s *standingtest.Server) { s.FailWith(http.StatusInternalServerError) },
		"timeout": func(s *standingtest.Server) { s.Delay(2 * time.Second) },
		"stale":   func(s *standingtest.Server) { s.StaleBy(45 * time.Second) },
		"down":    func(s *standingtest.Server) { s.Close() },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			svc, repo, pc, srv := newStandingService(t)
			breakIt(srv)
			_, err := svc.CreateBooking(context.Background(), immediateInput("cust-1"))
			assertAppCode(t, err, customerstanding.CodeUnavailable, http.StatusServiceUnavailable)
			if !errors.Is(err, customerstanding.ErrUnavailable) {
				t.Fatalf("not an unavailable error: %v", err)
			}
			assertNoBookingSideEffects(t, repo, pc)
		})
	}
}

func TestCustomerStanding_EligibleCustomerProceeds(t *testing.T) {
	svc, repo, _, srv := newStandingService(t)
	got, err := svc.CreateBooking(context.Background(), immediateInput("cust-ok"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != bookingmodels.StatusPendingMatch || repo.createdSeq != 1 || srv.Calls() != 1 {
		t.Fatalf("eligible booking: status=%s rows=%d calls=%d", got.Status, repo.createdSeq, srv.Calls())
	}
}

func TestCustomerStanding_LiftedHoldAllowsTheNextBookingImmediately(t *testing.T) {
	svc, repo, _, srv := newStandingService(t)
	srv.Hold("cust-1", "BOOKING_HOLD", 1)
	if _, err := svc.CreateBooking(context.Background(), immediateInput("cust-1")); err == nil {
		t.Fatal("held customer booked")
	}
	srv.Lift("cust-1", 2)
	if _, err := svc.CreateBooking(context.Background(), immediateInput("cust-1")); err != nil {
		t.Fatalf("lifted hold still refused (cached?): %v", err)
	}
	if repo.createdSeq != 1 || srv.Calls() != 2 {
		t.Fatalf("rows=%d calls=%d", repo.createdSeq, srv.Calls())
	}
}

func TestCustomerStanding_ConcurrentAttemptsByHeldCustomerAllRefused(t *testing.T) {
	svc, repo, pc, srv := newStandingService(t)
	srv.Hold("cust-held", "BOOKING_HOLD", 1)
	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.CreateBooking(context.Background(), immediateInput("cust-held"))
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, customerstanding.ErrBookingHold) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	assertNoBookingSideEffects(t, repo, pc)
	if srv.Calls() != n {
		t.Fatalf("calls = %d, want %d (every attempt asks the owner)", srv.Calls(), n)
	}
}

func TestCustomerStanding_DisabledEnforcementNeverAsks(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	srv := standingtest.New(t, "driver-taxi-service")
	srv.Hold("cust-held", "BOOKING_HOLD", 1)
	svc := NewBookingService(Options{Bookings: repo, Markets: markets, Pricing: pc, OperationalState: fakeOperationalState{}})
	if _, err := svc.CreateBooking(context.Background(), immediateInput("cust-held")); err != nil {
		t.Fatal(err)
	}
	if srv.Calls() != 0 {
		t.Fatal("disabled enforcement contacted customer-service")
	}
}

func TestCustomerStanding_ValidationStillRunsFirst(t *testing.T) {
	svc, _, _, srv := newStandingService(t)
	in := immediateInput("cust-1")
	in.VehicleClass = "spaceship"
	_, err := svc.CreateBooking(context.Background(), in)
	assertAppCode(t, err, apperrors.CodeValidationFailed, http.StatusUnprocessableEntity)
	if srv.Calls() != 0 {
		t.Fatal("an invalid request should not need the standing")
	}
}
