package bookingusecases

import (
	"context"
	"sync"
	"testing"
	"time"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
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

func newMatchService(repo *trackingRepo, store availabilityrepositories.AvailabilityStore, trucks TruckLookup, radiusKm float64) *BookingService {
	// Short search window keeps the wait-and-retry loop fast in tests; the empty
	// cases rescan a couple of times then mark unmatched.
	return newMatchServiceWithWindow(repo, store, trucks, radiusKm, 2)
}

func newMatchServiceWithWindow(repo *trackingRepo, store availabilityrepositories.AvailabilityStore, trucks TruckLookup, radiusKm float64, searchWindowSeconds int) *BookingService {
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
				_, _ = repo.MarkAccepted(context.Background(), "b1")
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

// ─── fakes ──────────────────────────────────────────────────────────────────

// trackingRepo is a concurrency-safe in-memory booking repo that honours the
// status guard on MarkMatched/ResetToMatching, so matcher behaviour can be tested.
type trackingRepo struct {
	mu       sync.Mutex
	bookings map[string]bookingmodels.Booking
}

func newTrackingRepo(seed ...bookingmodels.Booking) *trackingRepo {
	r := &trackingRepo{bookings: map[string]bookingmodels.Booking{}}
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
func (r *trackingRepo) GetByIDForCustomer(_ context.Context, id, _ string) (bookingmodels.Booking, error) {
	return r.GetByID(context.Background(), id)
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
func (r *trackingRepo) AssignProvider(_ context.Context, id, p, tr string) (bookingmodels.Booking, error) {
	return r.MarkMatched(context.Background(), id, p, tr)
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
func (r *trackingRepo) MarkAccepted(_ context.Context, id string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusAccepted)
}
func (r *trackingRepo) MarkEnRoutePickup(_ context.Context, id string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusEnRoutePickup)
}
func (r *trackingRepo) MarkArrivedAtPickup(_ context.Context, id string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusArrivedAtPickup)
}
func (r *trackingRepo) MarkPickedUp(_ context.Context, id string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusPickedUp)
}
func (r *trackingRepo) MarkEnRouteDelivery(_ context.Context, id string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusEnRouteDelivery)
}
func (r *trackingRepo) MarkDelivered(_ context.Context, id string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusDelivered)
}
func (r *trackingRepo) MarkCompleted(_ context.Context, id string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusCompleted)
}
func (r *trackingRepo) CancelByCustomer(_ context.Context, id, _, _ string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusCancelled)
}
func (r *trackingRepo) CancelByProvider(_ context.Context, id, _, _ string) (bookingmodels.Booking, error) {
	return r.UpdateStatus(context.Background(), id, bookingmodels.StatusCancelled)
}
func (r *trackingRepo) ResetToMatching(_ context.Context, id string) (bookingmodels.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.bookings[id]
	if b.Status == bookingmodels.StatusCancelled || b.Status == bookingmodels.StatusCompleted || b.Status == bookingmodels.StatusDelivered {
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
func (r *trackingRepo) ListDeliveredForAutoComplete(context.Context, time.Time) ([]bookingmodels.Booking, error) {
	return nil, nil
}
func (r *trackingRepo) AddEvent(context.Context, bookingmodels.BookingEvent) error { return nil }
func (r *trackingRepo) CreateReview(_ context.Context, rev bookingmodels.BookingReview) (bookingmodels.BookingReview, error) {
	return rev, nil
}
func (r *trackingRepo) GetReviewByBooking(context.Context, string) (bookingmodels.BookingReview, error) {
	return bookingmodels.BookingReview{}, nil
}

// fakeStore is a concurrency-safe availability store recording lock activity.
type fakeStore struct {
	mu        sync.Mutex
	providers []availabilityrepositories.ProviderStatus
	locks     map[string]string
	matched   map[string]int
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
func (s *fakeStore) Heartbeat(context.Context, string, float64, float64, time.Duration) error {
	return nil
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
func (s *fakeStore) GetProviderStatus(context.Context, string) (availabilityrepositories.ProviderStatus, bool, error) {
	return availabilityrepositories.ProviderStatus{}, false, nil
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

// allTrucks resolves every truck id to the same configured type/capacity.
type allTrucks struct {
	capacity  int
	truckType string
}

func (a allTrucks) GetTruck(context.Context, string) (TruckInfo, error) {
	return TruckInfo{TruckType: a.truckType, CapacityKg: a.capacity, Status: "active"}, nil
}
