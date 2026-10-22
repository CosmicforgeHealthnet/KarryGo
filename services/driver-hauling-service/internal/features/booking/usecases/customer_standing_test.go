package bookingusecases

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/customerstanding"
	"cosmicforge/logistics/shared/go/customerstanding/standingtest"
)

type standingFixture struct {
	svc      *BookingService
	repo     *trackingRepo
	store    *countingStore
	notifier *recordingNotifier
	srv      *standingtest.Server
}

func newStandingFixture(t *testing.T) standingFixture {
	t.Helper()
	f := standingFixture{repo: newTrackingRepo(), store: newCountingStore(), notifier: &recordingNotifier{},
		srv: standingtest.New(t, "driver-hauling-service")}
	f.store.setProviders(onlineStore().providers)
	f.svc = NewBookingService(Options{
		Bookings: f.repo, Availability: f.store, Trucks: allTrucks{capacity: 10000},
		Notifier:            bookingclients.NewBookingNotifierWith(f.notifier),
		MatchTimeoutSeconds: 1, SearchWindowSeconds: 1, MaxRadiusKm: 25,
		CustomerStanding: f.srv.Gate(300 * time.Millisecond),
	})
	return f
}

func standingInput(customerID string) CreateBookingInput {
	return CreateBookingInput{CustomerID: customerID, PickupAddress: "A", PickupLat: pickupLat, PickupLng: pickupLng,
		DropoffAddress: "B", DropoffLat: nearLat, DropoffLng: nearLng, CargoWeightKg: 100, PaymentMethod: bookingmodels.PaymentMethodWallet}
}

func (f standingFixture) assertNoSideEffects(t *testing.T) {
	t.Helper()
	f.repo.mu.Lock()
	rows := len(f.repo.bookings)
	f.repo.mu.Unlock()
	f.store.mu.Lock()
	nearby := f.store.nearbyCalls
	f.store.mu.Unlock()
	if rows != 0 || nearby != 0 || f.store.lockCount() != 0 || len(f.notifier.sent()) != 0 {
		t.Fatalf("side effects: rows=%d nearby=%d locks=%d notifications=%d", rows, nearby, f.store.lockCount(), len(f.notifier.sent()))
	}
}

func assertHaulingCode(t *testing.T, err error, code apperrors.Code, status int) {
	t.Helper()
	app := apperrors.From(err)
	if app == nil || app.Code != code || app.Status != status {
		t.Fatalf("error = %v, want %s/%d", err, code, status)
	}
}

func TestCustomerStanding_HeldCustomerCreatesNothing(t *testing.T) {
	for _, restriction := range []string{"BOOKING_HOLD", "LOGIN_BLOCKED"} {
		t.Run(restriction, func(t *testing.T) {
			f := newStandingFixture(t)
			f.srv.Hold("c-held", restriction, 2)
			_, err := f.svc.CreateBooking(context.Background(), standingInput("c-held"))
			assertHaulingCode(t, err, customerstanding.CodeBookingHold, http.StatusForbidden)
			f.assertNoSideEffects(t)
		})
	}
}

func TestCustomerStanding_ScheduledBookingIsGated(t *testing.T) {
	f := newStandingFixture(t)
	f.srv.Hold("c-held", "BOOKING_HOLD", 1)
	in := standingInput("c-held")
	future := time.Now().Add(3 * time.Hour)
	in.ScheduledAt = &future
	_, err := f.svc.CreateBooking(context.Background(), in)
	assertHaulingCode(t, err, customerstanding.CodeBookingHold, http.StatusForbidden)
	f.assertNoSideEffects(t)
}

func TestCustomerStanding_OutagesFailClosed(t *testing.T) {
	cases := map[string]func(*standingtest.Server){
		"5xx":     func(s *standingtest.Server) { s.FailWith(http.StatusInternalServerError) },
		"timeout": func(s *standingtest.Server) { s.Delay(2 * time.Second) },
		"stale":   func(s *standingtest.Server) { s.StaleBy(31 * time.Second) },
		"down":    func(s *standingtest.Server) { s.Close() },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			f := newStandingFixture(t)
			breakIt(f.srv)
			_, err := f.svc.CreateBooking(context.Background(), standingInput("c1"))
			assertHaulingCode(t, err, customerstanding.CodeUnavailable, http.StatusServiceUnavailable)
			f.assertNoSideEffects(t)
		})
	}
}

func TestCustomerStanding_EligibleProceedsAndLiftIsImmediate(t *testing.T) {
	f := newStandingFixture(t)
	f.srv.Hold("c1", "BOOKING_HOLD", 1)
	if _, err := f.svc.CreateBooking(context.Background(), standingInput("c1")); !errors.Is(err, customerstanding.ErrBookingHold) {
		t.Fatalf("held: %v", err)
	}
	f.srv.Lift("c1", 2)
	created, err := f.svc.CreateBooking(context.Background(), standingInput("c1"))
	if err != nil {
		t.Fatalf("lifted hold still refused: %v", err)
	}
	if created.ID == "" || f.repo.statusOf(created.ID) == "" {
		t.Fatal("eligible booking not persisted")
	}
	if f.srv.Calls() != 2 {
		t.Fatalf("standing calls = %d, want 2", f.srv.Calls())
	}
}

func TestCustomerStanding_ConcurrentHeldAttemptsAllRefused(t *testing.T) {
	f := newStandingFixture(t)
	f.srv.Hold("c-held", "BOOKING_HOLD", 1)
	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = f.svc.CreateBooking(context.Background(), standingInput("c-held"))
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, customerstanding.ErrBookingHold) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	f.assertNoSideEffects(t)
}

func TestCustomerStanding_ExistingBookingContinuesAfterHold(t *testing.T) {
	f := newStandingFixture(t)
	f.repo.bookings["pre-hold"] = bookingmodels.Booking{ID: "pre-hold", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PaymentMethod: bookingmodels.PaymentMethodWallet, PaymentStatus: bookingmodels.PaymentStatusUnpaid}
	f.srv.Hold("c1", "BOOKING_HOLD", 1)
	if _, err := f.svc.GetBooking(context.Background(), "pre-hold", "c1"); err != nil {
		t.Fatalf("read existing: %v", err)
	}
	if _, err := f.svc.CancelBooking(context.Background(), "pre-hold", "c1", "changed my mind"); err != nil {
		t.Fatalf("existing booking cancel blocked by hold: %v", err)
	}
	if f.repo.statusOf("pre-hold") != bookingmodels.StatusCancelled {
		t.Fatalf("status = %s", f.repo.statusOf("pre-hold"))
	}
	if f.srv.Calls() != 0 {
		t.Fatal("existing booking lifecycle consulted standing")
	}
}

func TestCustomerStanding_DisabledNeverAsks(t *testing.T) {
	srv := standingtest.New(t, "driver-hauling-service")
	srv.Hold("c-held", "BOOKING_HOLD", 1)
	svc := newMatchService(newTrackingRepo(), onlineStore(), allTrucks{capacity: 10000}, 25)
	if _, err := svc.CreateBooking(context.Background(), standingInput("c-held")); err != nil {
		t.Fatal(err)
	}
	if srv.Calls() != 0 {
		t.Fatal("disabled enforcement contacted customer-service")
	}
}
