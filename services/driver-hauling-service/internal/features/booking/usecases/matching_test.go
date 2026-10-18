package bookingusecases

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/handover"
)

var errNotFound = errorString("not found")

type errorString string

func (e errorString) Error() string { return string(e) }

// Lagos-ish coordinates for radius tests.
const (
	pickupLat = 6.5244
	pickupLng = 3.3792
	// ~2 km away — inside the default 25 km radius.
	nearLat = 6.5400
	nearLng = 3.3800
	// ~> 100 km away — outside the radius.
	farLat = 7.5000
	farLng = 4.5000
)

func newMatchService(repo bookingrepo.BookingRepository, store availabilityrepositories.AvailabilityStore, trucks TruckLookup, radiusKm float64) *BookingService {
	// Short search window keeps the wait-and-retry loop fast in tests; the empty
	// cases rescan a couple of times then mark unmatched.
	return newMatchServiceWithWindow(repo, store, trucks, radiusKm, 2)
}

func newMatchServiceWithWindow(repo bookingrepo.BookingRepository, store availabilityrepositories.AvailabilityStore, trucks TruckLookup, radiusKm float64, searchWindowSeconds int) *BookingService {
	return NewBookingService(Options{
		Bookings:            repo,
		Availability:        store,
		Trucks:              trucks,
		Notifier:            bookingclients.NewBookingNotifierWith(&recordingNotifier{}),
		MatchTimeoutSeconds: 1,
		SearchWindowSeconds: searchWindowSeconds,
		MaxRadiusKm:         radiusKm,
	})
}

func TestMatch_ProviderOutsideRadiusNotDispatched(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
	})
	store := &fakeStore{providers: []availabilityrepositories.ProviderStatus{
		{ProviderID: "far", TruckID: "tfar", Lat: farLat, Lng: farLng},
	}}
	svc := newMatchService(repo, store, allTrucks{capacity: 10000}, 25)

	svc.matchBooking(context.Background(), "b1", pickupLat, pickupLng)

	if store.lockCount() != 0 {
		t.Fatalf("expected no provider locked, got %d locks", store.lockCount())
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusUnmatched {
		t.Fatalf("expected unmatched, got %s", got)
	}
}

func TestMatch_TruckTooSmallSkipped(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 5000,
	})
	store := &fakeStore{providers: []availabilityrepositories.ProviderStatus{
		{ProviderID: "near", TruckID: "small", Lat: nearLat, Lng: nearLng},
	}}
	// Truck capacity 1000 kg < 5000 kg cargo.
	svc := newMatchService(repo, store, allTrucks{capacity: 1000}, 25)

	svc.matchBooking(context.Background(), "b1", pickupLat, pickupLng)

	if store.lockCount() != 0 {
		t.Fatalf("expected no provider locked for oversized cargo, got %d", store.lockCount())
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusUnmatched {
		t.Fatalf("expected unmatched, got %s", got)
	}
}

func TestMatch_WrongTruckTypeSkipped(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
		PreferredTruckType: "refrigerated",
	})
	store := &fakeStore{providers: []availabilityrepositories.ProviderStatus{
		{ProviderID: "near", TruckID: "flat", Lat: nearLat, Lng: nearLng},
	}}
	svc := newMatchService(repo, store, allTrucks{capacity: 10000, truckType: "flatbed"}, 25)

	svc.matchBooking(context.Background(), "b1", pickupLat, pickupLng)

	if store.lockCount() != 0 {
		t.Fatalf("expected no provider locked for wrong truck type, got %d", store.lockCount())
	}
}

func TestMatch_EligibleProviderDispatchedAndTimesOutToUnmatched(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
	})
	store := &fakeStore{providers: []availabilityrepositories.ProviderStatus{
		{ProviderID: "near", TruckID: "ok", Lat: nearLat, Lng: nearLng},
	}}
	svc := newMatchService(repo, store, allTrucks{capacity: 10000}, 25)

	svc.matchBooking(context.Background(), "b1", pickupLat, pickupLng)

	if store.matched["near"] == 0 {
		t.Fatalf("expected eligible provider to be locked/matched at least once")
	}
	// Provider never accepted -> exhausted -> unmatched.
	if got := repo.statusOf("b1"); got != bookingmodels.StatusUnmatched {
		t.Fatalf("expected unmatched after timeout, got %s", got)
	}
}

func TestMatch_NoProvidersOnlineWaitsThenUnmatched(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
	})
	// Nobody online for the whole window.
	store := &fakeStore{}
	svc := newMatchServiceWithWindow(repo, store, allTrucks{capacity: 10000}, 25, 1)

	start := time.Now()
	svc.matchBooking(context.Background(), "b1", pickupLat, pickupLng)
	elapsed := time.Since(start)

	// It must NOT give up instantly — it should keep searching for ~the window.
	if elapsed < 800*time.Millisecond {
		t.Fatalf("expected matching to wait out the search window, gave up after %s", elapsed)
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusUnmatched {
		t.Fatalf("expected unmatched after window, got %s", got)
	}
}

func TestMatch_ProviderComesOnlineDuringSearchGetsMatched(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
	})
	// Start with nobody online; a provider appears shortly after the search starts.
	store := &fakeStore{}
	svc := newMatchServiceWithWindow(repo, store, allTrucks{capacity: 10000}, 25, 6)

	// Provider comes online ~1s into the search.
	go func() {
		time.Sleep(1 * time.Second)
		store.setProviders([]availabilityrepositories.ProviderStatus{
			{ProviderID: "near", TruckID: "ok", Lat: nearLat, Lng: nearLng},
		})
	}()
	// Provider accepts shortly after being matched (flip to accepted).
	go func() {
		for i := 0; i < 100; i++ {
			time.Sleep(50 * time.Millisecond)
			if repo.statusOf("b1") == bookingmodels.StatusAwaitingAcceptance {
				_, _ = repo.MarkAccepted(context.Background(), "b1", "near")
				return
			}
		}
	}()

	svc.matchBooking(context.Background(), "b1", pickupLat, pickupLng)

	if store.matched["near"] == 0 {
		t.Fatalf("expected the provider that came online to be matched")
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusAccepted {
		t.Fatalf("expected accepted after provider came online and accepted, got %s", got)
	}
}

func TestMatch_RejectedProviderNotReDispatchedSameBooking(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
	})
	// "near" is closer than "near2", so it would normally be tried first on
	// every re-scan.
	store := &fakeStore{providers: []availabilityrepositories.ProviderStatus{
		{ProviderID: "near", TruckID: "t1", Lat: nearLat, Lng: nearLng},
		{ProviderID: "near2", TruckID: "t2", Lat: nearLat + 0.01, Lng: nearLng + 0.01},
	}}
	svc := newMatchServiceWithWindow(repo, store, allTrucks{capacity: 10000}, 25, 6)

	// Simulate "near" rejecting the booking as soon as it is matched, then let
	// "near2" accept once matched.
	go func() {
		for i := 0; i < 100; i++ {
			time.Sleep(50 * time.Millisecond)
			b, _ := repo.GetByID(context.Background(), "b1")
			if b.Status == bookingmodels.StatusAwaitingAcceptance && b.ProviderID != nil {
				switch *b.ProviderID {
				case "near":
					if _, err := svc.RejectBooking(context.Background(), "b1", "near"); err == nil {
						continue
					}
				case "near2":
					_, _ = repo.MarkAccepted(context.Background(), "b1", "near2")
					return
				}
			}
		}
	}()

	svc.matchBooking(context.Background(), "b1", pickupLat, pickupLng)

	if got := repo.statusOf("b1"); got != bookingmodels.StatusAccepted {
		t.Fatalf("expected booking accepted by near2, got %s", got)
	}
	b, _ := repo.GetByID(context.Background(), "b1")
	if b.ProviderID == nil || *b.ProviderID != "near2" {
		t.Fatalf("expected near2 to hold the accepted booking, got %+v", b.ProviderID)
	}
	rejected, _ := store.GetRejectedProviders(context.Background(), "b1")
	if !rejected["near"] {
		t.Fatalf("expected near to be recorded as rejected for b1")
	}
}

// TestMatch_AcceptAtTimeoutBoundaryNotClobbered pins the accept/timeout race:
// the provider's accept lands in the instant AFTER the match timeout fires but
// BEFORE the matcher resets the booking. The guarded ResetToMatching must refuse
// the reset so the accepted trip is never clobbered back to pending_match and
// re-dispatched ("accepted, then asked to accept again").
func TestMatch_AcceptAtTimeoutBoundaryNotClobbered(t *testing.T) {
	inner := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
		PickupLat: pickupLat, PickupLng: pickupLng, CargoWeightKg: 100,
	})
	repo := &acceptDuringResetRepo{trackingRepo: inner, acceptAs: "near"}
	store := &fakeStore{providers: []availabilityrepositories.ProviderStatus{
		{ProviderID: "near", TruckID: "ok", Lat: nearLat, Lng: nearLng},
	}}
	svc := newMatchServiceWithWindow(repo, store, allTrucks{capacity: 10000}, 25, 6)

	svc.matchBooking(context.Background(), "b1", pickupLat, pickupLng)

	if got := repo.statusOf("b1"); got != bookingmodels.StatusAccepted {
		t.Fatalf("expected booking to stay accepted after boundary accept, got %s", got)
	}
	b, _ := repo.GetByID(context.Background(), "b1")
	if b.ProviderID == nil || *b.ProviderID != "near" {
		t.Fatalf("expected near to keep the accepted booking, got %+v", b.ProviderID)
	}
	if store.matched["near"] != 1 {
		t.Fatalf("expected exactly one dispatch to near, got %d", store.matched["near"])
	}
	store.mu.Lock()
	_, lockHeld := store.locks["near"]
	store.mu.Unlock()
	if !lockHeld {
		t.Fatalf("expected near to keep the match lock after accepting")
	}
}

// acceptDuringResetRepo simulates the provider accepting in the window between
// the matcher's timeout firing and its ResetToMatching call: the first reset
// attempt is preceded by a successful accept, so the guarded reset must fail.
type acceptDuringResetRepo struct {
	*trackingRepo
	acceptAs string
	once     sync.Once
}

func (r *acceptDuringResetRepo) ResetToMatching(ctx context.Context, id, expectedProviderID string) (bookingmodels.Booking, error) {
	r.once.Do(func() {
		_, _ = r.trackingRepo.MarkAccepted(ctx, id, r.acceptAs)
	})
	return r.trackingRepo.ResetToMatching(ctx, id, expectedProviderID)
}

// ─── fakes ──────────────────────────────────────────────────────────────────

// trackingRepo is a concurrency-safe in-memory booking repo that honours the
// status guard on MarkMatched/ResetToMatching, so matcher behaviour can be tested.
type trackingRepo struct {
	mu       sync.Mutex
	bookings map[string]bookingmodels.Booking
	reviews  map[string]bookingmodels.BookingReview
}

func newTrackingRepo(seed ...bookingmodels.Booking) *trackingRepo {
	r := &trackingRepo{
		bookings: map[string]bookingmodels.Booking{},
		reviews:  map[string]bookingmodels.BookingReview{},
	}
	for _, b := range seed {
		r.bookings[b.ID] = b
	}
	return r
}

func (r *trackingRepo) statusOf(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bookings[id].Status
}

func (r *trackingRepo) Create(_ context.Context, b bookingmodels.Booking) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bookings[b.ID] = b
	return b, nil
}
func (r *trackingRepo) GetByID(_ context.Context, id string) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bookings[id], nil
}
func (r *trackingRepo) GetByIDForCustomer(_ context.Context, id, customerID string) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.bookings[id]
	if !ok || b.CustomerID != customerID {
		return bookingmodels.Booking{}, errNotFound
	}
	return b, nil
}
func (r *trackingRepo) ListByCustomer(context.Context, string, int, int) ([]bookingmodels.Booking, error) {
	return nil, nil
}
func (r *trackingRepo) ListByProvider(context.Context, string, int, int) ([]bookingmodels.Booking, error) {
	return nil, nil
}
func (r *trackingRepo) UpdateStatus(_ context.Context, id, status string) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	b.Status = status
	r.bookings[id] = b
	return b, nil
}
func (r *trackingRepo) MarkMatched(_ context.Context, id, p, tr string) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	if b.Status != bookingmodels.StatusPendingMatch {
		return bookingmodels.Booking{}, errNotFound
	}
	b.Status = bookingmodels.StatusAwaitingAcceptance
	b.ProviderID = &p
	b.TruckID = &tr
	r.bookings[id] = b
	return b, nil
}
func (r *trackingRepo) MarkAccepted(_ context.Context, id, providerID string) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	if b.Status != bookingmodels.StatusAwaitingAcceptance || b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.Booking{}, errNotFound
	}
	b.Status = bookingmodels.StatusAccepted
	r.bookings[id] = b
	return b, nil
}
func (r *trackingRepo) MarkEnRoutePickup(_ context.Context, id string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusEnRoutePickup)
}
func (r *trackingRepo) MarkArrivedAtPickup(_ context.Context, id string, rec handover.CodeRecord) (bookingmodels.Booking, error) {
	r.mu.Lock()
	b := r.bookings[id]
	b.Status = bookingmodels.StatusArrivedAtPickup
	b.PickupCode = rec.Code
	b.PickupCodeHash = rec.Hash
	b.PickupCodeExpiresAt = &rec.ExpiresAt
	b.PickupCodeAttempts = 0
	b.PickupCodeVerifiedAt = nil
	r.bookings[id] = b
	r.mu.Unlock()
	return b, nil
}
func (r *trackingRepo) MarkPickedUp(_ context.Context, id string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusPickedUp)
}
func (r *trackingRepo) MarkEnRouteDelivery(_ context.Context, id string, rec handover.CodeRecord) (bookingmodels.Booking, error) {
	r.mu.Lock()
	b := r.bookings[id]
	b.Status = bookingmodels.StatusEnRouteDelivery
	b.DeliveryCode = rec.Code
	b.DeliveryCodeHash = rec.Hash
	b.DeliveryCodeExpiresAt = &rec.ExpiresAt
	b.DeliveryCodeAttempts = 0
	b.DeliveryCodeVerifiedAt = nil
	r.bookings[id] = b
	r.mu.Unlock()
	return b, nil
}
func (r *trackingRepo) IncrementPickupCodeAttempt(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	b.PickupCodeAttempts++
	r.bookings[id] = b
	return nil
}
func (r *trackingRepo) IncrementDeliveryCodeAttempt(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	b.DeliveryCodeAttempts++
	r.bookings[id] = b
	return nil
}
func (r *trackingRepo) MarkPickupVerified(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	now := time.Now()
	b.PickupCodeVerifiedAt = &now
	r.bookings[id] = b
	return nil
}
func (r *trackingRepo) ResendPickupCode(_ context.Context, id string, rec handover.CodeRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	b.PickupCode = rec.Code
	b.PickupCodeHash = rec.Hash
	b.PickupCodeExpiresAt = &rec.ExpiresAt
	b.PickupCodeAttempts = 0
	b.PickupCodeVerifiedAt = nil
	r.bookings[id] = b
	return nil
}
func (r *trackingRepo) ResendDeliveryCode(_ context.Context, id string, rec handover.CodeRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	b.DeliveryCode = rec.Code
	b.DeliveryCodeHash = rec.Hash
	b.DeliveryCodeExpiresAt = &rec.ExpiresAt
	b.DeliveryCodeAttempts = 0
	b.DeliveryCodeVerifiedAt = nil
	r.bookings[id] = b
	return nil
}
func (r *trackingRepo) MarkDelivered(_ context.Context, id, _, _ string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusDelivered)
}
func (r *trackingRepo) MarkCompleted(_ context.Context, id string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusCompleted)
}

// MarkUnmatched honours the pending_match compare-and-set guard like the real repo.
func (r *trackingRepo) MarkUnmatched(_ context.Context, id string) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	if b.Status != bookingmodels.StatusPendingMatch {
		return bookingmodels.Booking{}, errNotFound
	}
	b.Status = bookingmodels.StatusUnmatched
	r.bookings[id] = b
	return b, nil
}

// CancelByCustomer honours the real repo's guard: cancellation is refused once
// the cargo is picked up or the booking is terminal. Guard failures surface as
// apperrors NotFound so the usecase's conflict mapping can be exercised.
func (r *trackingRepo) CancelByCustomer(_ context.Context, id, customerID, _ string) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.bookings[id]
	if !ok || b.CustomerID != customerID {
		return bookingmodels.Booking{}, apperrors.NotFound("Booking could not be found.", nil)
	}
	switch b.Status {
	case bookingmodels.StatusPickedUp, bookingmodels.StatusEnRouteDelivery,
		bookingmodels.StatusDelivered, bookingmodels.StatusCompleted, bookingmodels.StatusCancelled:
		return bookingmodels.Booking{}, apperrors.NotFound("Booking could not be found.", nil)
	}
	b.Status = bookingmodels.StatusCancelled
	r.bookings[id] = b
	return b, nil
}
func (r *trackingRepo) CancelByProvider(_ context.Context, id, _, _ string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusCancelled)
}
func (r *trackingRepo) ResetToMatching(_ context.Context, id, expectedProviderID string) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	if b.Status != bookingmodels.StatusAwaitingAcceptance || b.ProviderID == nil || *b.ProviderID != expectedProviderID {
		return bookingmodels.Booking{}, errNotFound
	}
	b.Status = bookingmodels.StatusPendingMatch
	b.ProviderID = nil
	b.TruckID = nil
	r.bookings[id] = b
	return b, nil
}
func (r *trackingRepo) SetPayment(_ context.Context, id, status, intent string) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	b.PaymentStatus = status
	if intent != "" {
		b.PaymentIntentID = &intent
	}
	r.bookings[id] = b
	return b, nil
}
func (r *trackingRepo) SaveDropoffRoute(_ context.Context, id, polyline string, distanceM, durationS int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	b.RouteDropoffPolyline = polyline
	b.RouteDropoffDistanceM = &distanceM
	b.RouteDropoffDurationS = &durationS
	r.bookings[id] = b
	return nil
}
func (r *trackingRepo) SavePickupRoute(_ context.Context, id, polyline string, distanceM, durationS int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	b.RoutePickupPolyline = polyline
	b.RoutePickupDistanceM = &distanceM
	b.RoutePickupDurationS = &durationS
	r.bookings[id] = b
	return nil
}
func (r *trackingRepo) ListDeliveredForAutoComplete(context.Context, time.Time) ([]bookingmodels.Booking, error) {
	return nil, nil
}
func (r *trackingRepo) ListStaleMatching(_ context.Context, pendingCutoff, awaitingCutoff time.Time) ([]bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []bookingmodels.Booking
	for _, b := range r.bookings {
		switch {
		case b.Status == bookingmodels.StatusPendingMatch && b.CreatedAt.Before(pendingCutoff):
			out = append(out, b)
		case b.Status == bookingmodels.StatusAwaitingAcceptance && b.MatchedAt != nil && b.MatchedAt.Before(awaitingCutoff):
			out = append(out, b)
		}
	}
	return out, nil
}
func (r *trackingRepo) AddEvent(context.Context, bookingmodels.BookingEvent) error { return nil }
func (r *trackingRepo) CreateReview(_ context.Context, rev bookingmodels.BookingReview) (bookingmodels.BookingReview, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.reviews[rev.BookingID]; exists {
		return bookingmodels.BookingReview{}, errNotFound
	}
	rev.ID = "review-" + rev.BookingID
	r.reviews[rev.BookingID] = rev
	return rev, nil
}
func (r *trackingRepo) GetReviewByBooking(_ context.Context, bookingID string) (bookingmodels.BookingReview, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rev, ok := r.reviews[bookingID]
	if !ok {
		return bookingmodels.BookingReview{}, errNotFound
	}
	return rev, nil
}

// fakeStore is a concurrency-safe availability store recording lock activity.
type fakeStore struct {
	mu        sync.Mutex
	providers []availabilityrepositories.ProviderStatus
	locks     map[string]string
	matched   map[string]int
	rejected  map[string]map[string]bool
	// statuses answers GetProviderStatus, which the pickup-leg route lookup
	// uses to find where the assigned driver actually is.
	statuses map[string]availabilityrepositories.ProviderStatus
}

func newFakeStore() *fakeStore { return &fakeStore{} }

func (s *fakeStore) setStatus(status availabilityrepositories.ProviderStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statuses == nil {
		s.statuses = map[string]availabilityrepositories.ProviderStatus{}
	}
	s.statuses[status.ProviderID] = status
}

func (s *fakeStore) lockCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.matched)
}

func (s *fakeStore) SetOnline(context.Context, availabilityrepositories.ProviderStatus, time.Duration) error {
	return nil
}
func (s *fakeStore) SetOffline(context.Context, string) error { return nil }
func (s *fakeStore) Heartbeat(context.Context, string, availabilityrepositories.Telemetry, time.Duration) error {
	return nil
}
func (s *fakeStore) SetDiscoverable(context.Context, string, bool) error { return nil }

// NearbyProviders models the Redis cell index in memory: a radius-bounded,
// nearest-first slice rather than every online provider. Matching reads this,
// so the tests exercise the same path production does.
func (s *fakeStore) NearbyProviders(_ context.Context, center geo.Point, radiusKm float64, limit int) ([]availabilityrepositories.ProviderStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	type scored struct {
		p    availabilityrepositories.ProviderStatus
		dist float64
	}
	var in []scored
	for _, p := range s.providers {
		d := geo.HaversineKm(center, geo.Point{Lat: p.Lat, Lng: p.Lng})
		if d > radiusKm {
			continue
		}
		in = append(in, scored{p: p, dist: d})
	}
	sort.Slice(in, func(i, j int) bool { return in[i].dist < in[j].dist })
	if limit > 0 && len(in) > limit {
		in = in[:limit]
	}

	out := make([]availabilityrepositories.ProviderStatus, len(in))
	for i, s := range in {
		out[i] = s.p
	}
	return out, nil
}
func (s *fakeStore) setProviders(p []availabilityrepositories.ProviderStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers = p
}
func (s *fakeStore) CountOnline(context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.providers)), nil
}
func (s *fakeStore) GetOnlineProviders(context.Context) ([]availabilityrepositories.ProviderStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]availabilityrepositories.ProviderStatus(nil), s.providers...), nil
}
func (s *fakeStore) GetProviderStatus(_ context.Context, providerID string) (availabilityrepositories.ProviderStatus, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, ok := s.statuses[providerID]
	return status, ok, nil
}
func (s *fakeStore) AcquireMatchLock(_ context.Context, providerID, bookingID string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks == nil {
		s.locks = map[string]string{}
		s.matched = map[string]int{}
	}
	if _, held := s.locks[providerID]; held {
		return false, nil
	}
	s.locks[providerID] = bookingID
	s.matched[providerID]++
	return true, nil
}
func (s *fakeStore) ReleaseMatchLock(_ context.Context, providerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.locks, providerID)
	return nil
}
func (s *fakeStore) ReleaseMatchLockFor(_ context.Context, providerID, bookingID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks[providerID] == bookingID {
		delete(s.locks, providerID)
	}
	return nil
}
func (s *fakeStore) MarkRejected(_ context.Context, bookingID, providerID string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rejected == nil {
		s.rejected = map[string]map[string]bool{}
	}
	if s.rejected[bookingID] == nil {
		s.rejected[bookingID] = map[string]bool{}
	}
	s.rejected[bookingID][providerID] = true
	return nil
}
func (s *fakeStore) GetRejectedProviders(_ context.Context, bookingID string) (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]bool, len(s.rejected[bookingID]))
	for id := range s.rejected[bookingID] {
		result[id] = true
	}
	return result, nil
}

// allTrucks resolves every truck id to the same configured type/capacity.
type allTrucks struct {
	capacity  int
	truckType string
}

func (a allTrucks) GetTruck(context.Context, string) (TruckInfo, error) {
	return TruckInfo{TruckType: a.truckType, CapacityKg: a.capacity, Status: "active"}, nil
}
