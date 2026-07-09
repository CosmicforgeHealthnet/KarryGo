package bookingusecases

import (
	"context"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

// TestMatch_TimedOutProviderMarkedRejected pins the silent-provider fix: a
// provider who lets the acceptance window lapse must be marked rejected so the
// next re-scan does not re-offer them the same booking (previously the matcher
// ping-ponged the nearest silent provider for the whole search window).
func TestMatch_TimedOutProviderMarkedRejected(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
	})
	store := &fakeStore{providers: []availabilityrepositories.ProviderStatus{
		{ProviderID: "silent", TruckID: "t1", Lat: nearLat, Lng: nearLng},
	}}
	// Window fits several 1s acceptance attempts; the provider never responds.
	svc := newMatchServiceWithWindow(repo, store, allTrucks{capacity: 10000}, 25, 5)

	svc.matchBooking(context.Background(), "b1", pickupLat, pickupLng)

	if got := store.matched["silent"]; got != 1 {
		t.Fatalf("expected silent provider to be dispatched exactly once, got %d", got)
	}
	rejected, _ := store.GetRejectedProviders(context.Background(), "b1")
	if !rejected["silent"] {
		t.Fatalf("expected timed-out provider to be recorded as rejected")
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusUnmatched {
		t.Fatalf("expected unmatched after window, got %s", got)
	}
}

// TestMarkUnmatched_GuardRefusesOverAccepted pins the compare-and-set on the
// unmatched transition: a stale deadline must not clobber an accepted booking.
func TestMarkUnmatched_GuardRefusesOverAccepted(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusAccepted,
	})
	svc := lifecycleService(repo)

	svc.markUnmatched(context.Background(), "b1", "c1")

	if got := repo.statusOf("b1"); got != bookingmodels.StatusAccepted {
		t.Fatalf("markUnmatched clobbered an accepted booking to %s", got)
	}
}

// TestRecovery_ResumesStalePendingMatch: a pending_match booking with no live
// matcher (as after a restart) and search window remaining must be re-adopted
// and dispatched again.
func TestRecovery_ResumesStalePendingMatch(t *testing.T) {
	created := time.Now().Add(-20 * time.Second)
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
		CreatedAt: created,
	})
	store := &fakeStore{providers: []availabilityrepositories.ProviderStatus{
		{ProviderID: "near", TruckID: "ok", Lat: nearLat, Lng: nearLng},
	}}
	svc := newMatchServiceWithWindow(repo, store, allTrucks{capacity: 10000}, 25, 60)

	// Provider accepts as soon as the resumed matcher offers them the booking.
	go func() {
		for i := 0; i < 100; i++ {
			time.Sleep(50 * time.Millisecond)
			if repo.statusOf("b1") == bookingmodels.StatusAwaitingAcceptance {
				_, _ = repo.MarkAccepted(context.Background(), "b1", "near")
				return
			}
		}
	}()

	svc.recoverStaleMatching(context.Background())

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if repo.statusOf("b1") == bookingmodels.StatusAccepted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusAccepted {
		t.Fatalf("expected recovered booking to be matched and accepted, got %s", got)
	}
	svc.cancelMatch("b1")
}

// TestRecovery_UnmatchesExpiredPendingMatch: a stranded booking whose search
// window (from creation) already lapsed goes unmatched instead of resuming.
func TestRecovery_UnmatchesExpiredPendingMatch(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
		CreatedAt: time.Now().Add(-20 * time.Second),
	})
	store := &fakeStore{}
	svc := newMatchServiceWithWindow(repo, store, allTrucks{capacity: 10000}, 25, 2)

	svc.recoverStaleMatching(context.Background())

	if got := repo.statusOf("b1"); got != bookingmodels.StatusUnmatched {
		t.Fatalf("expected expired stranded booking to be unmatched, got %s", got)
	}
}

// TestRecovery_ResetsOrphanedAwaitingAcceptance: an awaiting_acceptance booking
// whose offer window lapsed with no live matcher is reset (freeing the provider)
// and, with its search window spent, marked unmatched.
func TestRecovery_ResetsOrphanedAwaitingAcceptance(t *testing.T) {
	matched := time.Now().Add(-60 * time.Second)
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusAwaitingAcceptance,
		ProviderID: ptr("gone"), MatchedAt: &matched,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
		CreatedAt: time.Now().Add(-70 * time.Second),
	})
	store := &fakeStore{}
	svc := newMatchServiceWithWindow(repo, store, allTrucks{capacity: 10000}, 25, 2)

	svc.recoverStaleMatching(context.Background())

	if got := repo.statusOf("b1"); got != bookingmodels.StatusUnmatched {
		t.Fatalf("expected orphaned offer to end unmatched, got %s", got)
	}
	b, _ := repo.GetByID(context.Background(), "b1")
	if b.ProviderID != nil {
		t.Fatalf("expected provider assignment to be cleared, got %v", *b.ProviderID)
	}
}

// TestRecovery_SkipsBookingWithLiveMatcher: the worker must never touch a
// booking whose matcher goroutine is still alive in this process.
func TestRecovery_SkipsBookingWithLiveMatcher(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
		CreatedAt: time.Now().Add(-20 * time.Second),
	})
	store := &fakeStore{}
	svc := newMatchServiceWithWindow(repo, store, allTrucks{capacity: 10000}, 25, 2)
	svc.activeMatches.Store("b1", &matchHandle{cancel: func() {}})

	svc.recoverStaleMatching(context.Background())

	if got := repo.statusOf("b1"); got != bookingmodels.StatusPendingMatch {
		t.Fatalf("recovery touched a booking with a live matcher: %s", got)
	}
}

// TestCancelBooking_BlockedAfterPickup pins the cancellation policy: once the
// cargo is picked up the customer cancel is refused with a conflict, not a
// bare not-found, and the booking keeps its status.
func TestCancelBooking_BlockedAfterPickup(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", ProviderID: ptr("p1"),
		Status: bookingmodels.StatusPickedUp,
	})
	svc := lifecycleService(repo)

	_, err := svc.CancelBooking(context.Background(), "b1", "c1", "changed my mind")
	if err == nil {
		t.Fatal("expected mid-transit cancel to be refused")
	}
	if appErr := apperrors.From(err); appErr == nil || appErr.Code != apperrors.CodeConflict {
		t.Fatalf("expected conflict error, got %v", err)
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusPickedUp {
		t.Fatalf("cancel must not change status, got %s", got)
	}
}

// TestCreateBooking_CardRejected pins the card-path removal: bookings cannot be
// created with payment_method=card until card is properly supported.
func TestCreateBooking_CardRejected(t *testing.T) {
	repo := newTrackingRepo()
	svc := newMatchService(repo, &fakeStore{}, allTrucks{capacity: 10000}, 25)

	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID:     "c1",
		PickupAddress:  "A", PickupLat: pickupLat, PickupLng: pickupLng,
		DropoffAddress: "B", DropoffLat: nearLat, DropoffLng: nearLng,
		CargoWeightKg:  100,
		PaymentMethod:  bookingmodels.PaymentMethodCard,
	})
	if err == nil {
		t.Fatal("expected card booking to be rejected")
	}
	if appErr := apperrors.From(err); appErr == nil || appErr.Code != apperrors.CodeValidationFailed {
		t.Fatalf("expected validation error, got %v", err)
	}
}
