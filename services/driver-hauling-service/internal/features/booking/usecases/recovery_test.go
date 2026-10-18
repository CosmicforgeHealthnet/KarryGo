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

// TestCancelBooking_FreesAcceptedProvider: an accepted provider keeps the
// match lock, so a customer cancel must free it or the matcher skips that
// provider for new jobs until the lock expires. A lock the provider holds for
// a different booking is left alone.
func TestCancelBooking_FreesAcceptedProvider(t *testing.T) {
	repo := newTrackingRepo(
		bookingmodels.Booking{ID: "b1", CustomerID: "c1", ProviderID: ptr("p1"), Status: bookingmodels.StatusAccepted},
		bookingmodels.Booking{ID: "b2", CustomerID: "c1", ProviderID: ptr("p2"), Status: bookingmodels.StatusAccepted},
	)
	store := &fakeStore{locks: map[string]string{"p1": "b1", "p2": "other-booking"}, matched: map[string]int{}}
	svc := newMatchServiceWithWindow(repo, store, allTrucks{capacity: 10000}, 25, 2)

	for _, id := range []string{"b1", "b2"} {
		if _, err := svc.CancelBooking(context.Background(), id, "c1", "changed my mind"); err != nil {
			t.Fatalf("cancel %s: %v", id, err)
		}
	}
	if _, held := store.locks["p1"]; held {
		t.Fatal("the cancelled booking's provider must be free for new jobs")
	}
	if store.locks["p2"] != "other-booking" {
		t.Fatal("a lock held for another booking must not be freed")
	}
}

// TestCreateBooking_CardRejected pins the card-path removal: bookings cannot be
// created with payment_method=card until card is properly supported.
func TestCreateBooking_CardRejected(t *testing.T) {
	repo := newTrackingRepo()
	svc := newMatchService(repo, &fakeStore{}, allTrucks{capacity: 10000}, 25)

	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID:    "c1",
		PickupAddress: "A", PickupLat: pickupLat, PickupLng: pickupLng,
		DropoffAddress: "B", DropoffLat: nearLat, DropoffLng: nearLng,
		CargoWeightKg: 100,
		PaymentMethod: bookingmodels.PaymentMethodCard,
	})
	if err == nil {
		t.Fatal("expected card booking to be rejected")
	}
	if appErr := apperrors.From(err); appErr == nil || appErr.Code != apperrors.CodeValidationFailed {
		t.Fatalf("expected validation error, got %v", err)
	}
}

// onlineStore returns a store with one nearby provider, so CreateBooking gets
// past its "any providers online?" gate and reaches the input validation.
func onlineStore() *fakeStore {
	return &fakeStore{providers: []availabilityrepositories.ProviderStatus{
		{ProviderID: "near", TruckID: "t1", Lat: nearLat, Lng: nearLng},
	}}
}

// TestCreateBooking_PreferredTruckTypeValidated guards the customer/backend
// truck-type contract. Matching compares preferred_truck_type to the provider's
// trucks.truck_type with exact string equality, so an unknown value would be
// stored happily and then never match anything — the booking would just expire.
// Empty stays valid: it means "any truck" and skips the matcher's type filter.
func TestCreateBooking_PreferredTruckTypeValidated(t *testing.T) {
	newInput := func(truckType string) CreateBookingInput {
		return CreateBookingInput{
			CustomerID:    "c1",
			PickupAddress: "A", PickupLat: pickupLat, PickupLng: pickupLng,
			DropoffAddress: "B", DropoffLat: nearLat, DropoffLng: nearLng,
			CargoWeightKg:      100,
			PreferredTruckType: truckType,
			PaymentMethod:      bookingmodels.PaymentMethodWallet,
		}
	}

	t.Run("unknown type rejected", func(t *testing.T) {
		svc := newMatchService(newTrackingRepo(), onlineStore(), allTrucks{capacity: 10000}, 25)
		_, err := svc.CreateBooking(context.Background(), newInput("spaceship"))
		if err == nil {
			t.Fatal("expected unknown truck type to be rejected")
		}
		appErr := apperrors.From(err)
		if appErr == nil || appErr.Code != apperrors.CodeValidationFailed {
			t.Fatalf("expected validation error, got %v", err)
		}
		var found bool
		for _, f := range appErr.Fields {
			if f.Field == "preferred_truck_type" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected a preferred_truck_type field violation, got %+v", appErr.Fields)
		}
	})

	// Every type the customer app can emit must be accepted, otherwise picking it
	// in the dropdown would 422 the booking.
	for _, truckType := range []string{
		"pickup", "box", "flatbed", "refrigerated", "tanker", "trailer",
		"dump", "lowbed", "crane", "container", "van", "tipper", "",
	} {
		t.Run("accepts "+truckType, func(t *testing.T) {
			svc := newMatchService(newTrackingRepo(), onlineStore(), allTrucks{capacity: 10000}, 25)
			if _, err := svc.CreateBooking(context.Background(), newInput(truckType)); err != nil {
				t.Fatalf("expected %q to be accepted, got %v", truckType, err)
			}
		})
	}

	t.Run("normalized to lowercase on store", func(t *testing.T) {
		repo := newTrackingRepo()
		svc := newMatchService(repo, onlineStore(), allTrucks{capacity: 10000}, 25)
		created, err := svc.CreateBooking(context.Background(), newInput("  Van  "))
		if err != nil {
			t.Fatalf("expected mixed-case truck type to be accepted, got %v", err)
		}
		if created.PreferredTruckType != "van" {
			t.Fatalf("expected stored truck type %q, got %q", "van", created.PreferredTruckType)
		}
	})
}
