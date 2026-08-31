package bookingusecases

import (
	"context"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ─── searchDeadlineFor (the exact B9 fix) ───────────────────────────────────

func TestSearchDeadlineFor_UsesMatchingStartedAtWhenSet(t *testing.T) {
	createdAt := time.Now().Add(-48 * time.Hour) // a ride booked 2 days ago
	matchingStartedAt := time.Now()              // promoted just now
	booking := bookingmodels.Booking{CreatedAt: createdAt, MatchingStartedAt: &matchingStartedAt}

	window := 60 * time.Second
	got := searchDeadlineFor(booking, window)
	want := matchingStartedAt.Add(window)
	if !got.Equal(want) {
		t.Errorf("deadline = %v, want %v (anchored to MatchingStartedAt)", got, want)
	}
	// The exact bug this test guards against: a deadline anchored to
	// CreatedAt would already be ~48 hours in the past.
	if got.Before(time.Now()) {
		t.Error("deadline is already in the past -- this is the exact B9 bug: a scheduled booking's search window must not be expired the instant it is promoted")
	}
}

func TestSearchDeadlineFor_FallsBackToCreatedAtWhenNil(t *testing.T) {
	createdAt := time.Now()
	booking := bookingmodels.Booking{CreatedAt: createdAt, MatchingStartedAt: nil}

	window := 60 * time.Second
	got := searchDeadlineFor(booking, window)
	want := createdAt.Add(window)
	if !got.Equal(want) {
		t.Errorf("deadline = %v, want %v (fallback to CreatedAt for a booking with no MatchingStartedAt)", got, want)
	}
}

// ─── CreateBooking scheduling branch ────────────────────────────────────────

func newSchedulingTestService(bookings *fakeBookingRepo, leadTime time.Duration) *BookingService {
	return NewBookingService(Options{
		Bookings: bookings, Markets: newFakeMarkets(), Pricing: newFakePricing(),
		Scheduling: SchedulingConfig{LeadTime: leadTime},
	})
}

func validCreateInput(customerID string) CreateBookingInput {
	return CreateBookingInput{
		CustomerID: customerID, VehicleClass: "economy", PaymentMethod: "cash",
		Stops: []StopInput{{Address: "A", Lat: 6.5, Lng: 3.4}, {Address: "B", Lat: 6.6, Lng: 3.5}},
	}
}

func TestCreateBooking_RejectsScheduledAtInThePast(t *testing.T) {
	svc := newSchedulingTestService(newFakeBookingRepo(), 10*time.Minute)
	past := time.Now().Add(-time.Hour)
	input := validCreateInput("customer-1")
	input.ScheduledAt = &past

	if _, err := svc.CreateBooking(context.Background(), input); err == nil {
		t.Fatal("expected a validation error for a scheduled_at in the past")
	}
}

func TestCreateBooking_RejectsScheduledAtExactlyNow(t *testing.T) {
	svc := newSchedulingTestService(newFakeBookingRepo(), 10*time.Minute)
	now := time.Now()
	input := validCreateInput("customer-1")
	input.ScheduledAt = &now

	if _, err := svc.CreateBooking(context.Background(), input); err == nil {
		t.Fatal("expected a validation error for scheduled_at == now (not strictly in the future)")
	}
}

func TestCreateBooking_ScheduledBookingGetsScheduledStatusAndComputedDispatchStart(t *testing.T) {
	bookings := newFakeBookingRepo()
	svc := newSchedulingTestService(bookings, 15*time.Minute)
	future := time.Now().Add(2 * time.Hour)
	input := validCreateInput("customer-1")
	input.ScheduledAt = &future

	booking, err := svc.CreateBooking(context.Background(), input)
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}
	if booking.Status != bookingmodels.StatusScheduled {
		t.Errorf("status = %q, want scheduled", booking.Status)
	}
	if booking.ScheduledAt == nil || !booking.ScheduledAt.Equal(future) {
		t.Errorf("ScheduledAt = %v, want %v", booking.ScheduledAt, future)
	}
	wantDispatch := future.Add(-15 * time.Minute)
	if booking.DispatchStartAt == nil || !booking.DispatchStartAt.Equal(wantDispatch) {
		t.Errorf("DispatchStartAt = %v, want %v (ScheduledAt - 15min lead time)", booking.DispatchStartAt, wantDispatch)
	}
	if booking.MatchingStartedAt != nil {
		t.Error("MatchingStartedAt should stay nil at creation for a scheduled booking -- it is set at PROMOTION time")
	}
}

func TestCreateBooking_ImmediateBookingGetsPendingMatchAndMatchingStartedAtNow(t *testing.T) {
	bookings := newFakeBookingRepo()
	svc := newSchedulingTestService(bookings, 15*time.Minute)
	input := validCreateInput("customer-1") // ScheduledAt left nil

	before := time.Now()
	booking, err := svc.CreateBooking(context.Background(), input)
	after := time.Now()
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}
	if booking.Status != bookingmodels.StatusPendingMatch {
		t.Errorf("status = %q, want pending_match", booking.Status)
	}
	if booking.MatchingStartedAt == nil {
		t.Fatal("MatchingStartedAt is nil, want it set to roughly now for an immediate booking")
	}
	if booking.MatchingStartedAt.Before(before) || booking.MatchingStartedAt.After(after) {
		t.Errorf("MatchingStartedAt = %v, want it between %v and %v", booking.MatchingStartedAt, before, after)
	}
	if booking.ScheduledAt != nil || booking.DispatchStartAt != nil {
		t.Errorf("ScheduledAt/DispatchStartAt should stay nil for an immediate booking, got %v/%v", booking.ScheduledAt, booking.DispatchStartAt)
	}
}

// ─── PromoteDueScheduledBookings ────────────────────────────────────────────

func TestPromoteDueScheduledBookings_PromotesOnlyDueBookings(t *testing.T) {
	bookings := newFakeBookingRepo()
	past := time.Now().Add(-time.Minute)
	future := time.Now().Add(time.Hour)
	pid1, pid2 := "p1", "p2"
	bookings.byID["due"] = bookingmodels.Booking{ID: "due", ProviderID: &pid1, Status: bookingmodels.StatusScheduled, DispatchStartAt: &past}
	bookings.byID["not-due"] = bookingmodels.Booking{ID: "not-due", ProviderID: &pid2, Status: bookingmodels.StatusScheduled, DispatchStartAt: &future}
	svc := NewBookingService(Options{Bookings: bookings})

	promoted, err := svc.PromoteDueScheduledBookings(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("PromoteDueScheduledBookings: %v", err)
	}
	if promoted != 1 {
		t.Errorf("promoted = %d, want 1", promoted)
	}
	if bookings.byID["due"].Status != bookingmodels.StatusPendingMatch {
		t.Errorf("due booking status = %q, want pending_match", bookings.byID["due"].Status)
	}
	if bookings.byID["due"].MatchingStartedAt == nil {
		t.Error("due booking MatchingStartedAt was not set by promotion")
	}
	if bookings.byID["not-due"].Status != bookingmodels.StatusScheduled {
		t.Errorf("not-due booking status = %q, want unchanged scheduled", bookings.byID["not-due"].Status)
	}
}

// fakePromoteConflictRepo wraps fakeBookingRepo to simulate a concurrent
// sweep having already promoted a booking between ListDueScheduled and this
// sweep's own PromoteScheduledToPendingMatch call.
type fakePromoteConflictRepo struct {
	*fakeBookingRepo
	conflictFor map[string]bool
}

func (f *fakePromoteConflictRepo) PromoteScheduledToPendingMatch(ctx context.Context, bookingID string, matchingStartedAt time.Time) (bookingmodels.Booking, error) {
	if f.conflictFor[bookingID] {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	return f.fakeBookingRepo.PromoteScheduledToPendingMatch(ctx, bookingID, matchingStartedAt)
}

func TestPromoteDueScheduledBookings_ConflictOnOnePromotionDoesNotStopTheSweep(t *testing.T) {
	inner := newFakeBookingRepo()
	past := time.Now().Add(-time.Minute)
	pid1, pid2 := "p1", "p2"
	inner.byID["already-promoted"] = bookingmodels.Booking{ID: "already-promoted", ProviderID: &pid1, Status: bookingmodels.StatusScheduled, DispatchStartAt: &past}
	inner.byID["still-due"] = bookingmodels.Booking{ID: "still-due", ProviderID: &pid2, Status: bookingmodels.StatusScheduled, DispatchStartAt: &past}
	repo := &fakePromoteConflictRepo{fakeBookingRepo: inner, conflictFor: map[string]bool{"already-promoted": true}}
	svc := NewBookingService(Options{Bookings: repo})

	promoted, err := svc.PromoteDueScheduledBookings(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("PromoteDueScheduledBookings must not fail on a single conflicted promotion: %v", err)
	}
	if promoted != 1 {
		t.Errorf("promoted = %d, want 1 (the other booking, despite one conflict)", promoted)
	}
	if inner.byID["still-due"].Status != bookingmodels.StatusPendingMatch {
		t.Error("the non-conflicting booking should still have been promoted")
	}
}

type fakePromoteFailsHardRepo struct {
	*fakeBookingRepo
}

func (f *fakePromoteFailsHardRepo) PromoteScheduledToPendingMatch(ctx context.Context, bookingID string, matchingStartedAt time.Time) (bookingmodels.Booking, error) {
	return bookingmodels.Booking{}, apperrors.Unavailable("database unreachable", nil)
}

func TestPromoteDueScheduledBookings_PropagatesARealNonConflictError(t *testing.T) {
	inner := newFakeBookingRepo()
	past := time.Now().Add(-time.Minute)
	pid := "p1"
	inner.byID["due"] = bookingmodels.Booking{ID: "due", ProviderID: &pid, Status: bookingmodels.StatusScheduled, DispatchStartAt: &past}
	repo := &fakePromoteFailsHardRepo{fakeBookingRepo: inner}
	svc := NewBookingService(Options{Bookings: repo})

	_, err := svc.PromoteDueScheduledBookings(context.Background(), time.Now())
	if err == nil {
		t.Fatal("expected a real (non-conflict) repository error to propagate")
	}
}

func TestPromoteDueScheduledBookings_NoDueBookingsIsANoop(t *testing.T) {
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo()})
	promoted, err := svc.PromoteDueScheduledBookings(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("PromoteDueScheduledBookings: %v", err)
	}
	if promoted != 0 {
		t.Errorf("promoted = %d, want 0", promoted)
	}
}
