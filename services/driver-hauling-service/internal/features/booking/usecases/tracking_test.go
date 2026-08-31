package bookingusecases

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/routing"
)

// countingStore records how the matcher asked for providers, so the tests can
// prove matching reads the bounded spatial index rather than the whole online
// set.
type countingStore struct {
	*fakeStore
	mu           sync.Mutex
	nearbyCalls  int
	scanCalls    int
	nearbyRadius float64
	nearbyErr    error
	nearbyEmpty  bool
}

func newCountingStore() *countingStore {
	return &countingStore{fakeStore: newFakeStore()}
}

func (s *countingStore) NearbyProviders(ctx context.Context, center geo.Point, radiusKm float64, limit int) ([]availabilityrepositories.ProviderStatus, error) {
	s.mu.Lock()
	s.nearbyCalls++
	s.nearbyRadius = radiusKm
	err, empty := s.nearbyErr, s.nearbyEmpty
	s.mu.Unlock()

	if err != nil {
		return nil, err
	}
	if empty {
		return nil, nil
	}
	return s.fakeStore.NearbyProviders(ctx, center, radiusKm, limit)
}

func (s *countingStore) GetOnlineProviders(ctx context.Context) ([]availabilityrepositories.ProviderStatus, error) {
	s.mu.Lock()
	s.scanCalls++
	s.mu.Unlock()
	return s.fakeStore.GetOnlineProviders(ctx)
}

func (s *countingStore) counts() (nearby, scan int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nearbyCalls, s.scanCalls
}

func (s *countingStore) failNearby(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nearbyErr = err
}

func (s *countingStore) emptyNearby() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nearbyEmpty = true
}

func trackingTestService(t *testing.T, store availabilityrepositories.AvailabilityStore, repo *trackingRepo, routes routing.Fetcher) *BookingService {
	t.Helper()
	return NewBookingService(Options{
		Bookings:            repo,
		Availability:        store,
		Trucks:              allTrucks{capacity: 10000},
		Notifier:            bookingclients.NewBookingNotifierWith(&recordingNotifier{}),
		MatchTimeoutSeconds: 1,
		SearchWindowSeconds: 1,
		MaxRadiusKm:         25,
		Routes:              routes,
	})
}

// Matching must query the index, not scan every online provider. This is the
// whole point of the spatial partition.
func TestMatching_ReadsTheSpatialIndex(t *testing.T) {
	store := newCountingStore()
	store.setProviders([]availabilityrepositories.ProviderStatus{
		{ProviderID: "p1", Lat: 6.6018, Lng: 3.3515},
	})
	repo := newTrackingRepo()
	booking := repo.put(bookingmodels.Booking{
		ID:         "b1",
		CustomerID: "c1",
		Status:     bookingmodels.StatusPendingMatch,
		PickupLat:  6.6018,
		PickupLng:  3.3515,
		CreatedAt:  time.Now(),
	})

	svc := trackingTestService(t, store, repo, nil)
	svc.matchBooking(context.Background(), booking.ID, booking.PickupLat, booking.PickupLng)

	nearby, scan := store.counts()
	if nearby == 0 {
		t.Fatal("matching never consulted the spatial index")
	}
	if scan != 0 {
		t.Fatalf("matching fell back to a full online scan %d time(s) despite the index returning providers", scan)
	}
	if store.nearbyRadius != 25 {
		t.Fatalf("index queried with radius %v, want the configured 25 km match radius", store.nearbyRadius)
	}
}

// During a deploy, providers who went online before this build have an
// availability key but no index entry. Matching must still find them.
func TestMatching_FallsBackToScanWhenIndexIsEmpty(t *testing.T) {
	store := newCountingStore()
	store.emptyNearby()
	store.setProviders([]availabilityrepositories.ProviderStatus{
		{ProviderID: "p1", Lat: 6.6018, Lng: 3.3515},
	})
	repo := newTrackingRepo()
	booking := repo.put(bookingmodels.Booking{
		ID:         "b1",
		CustomerID: "c1",
		Status:     bookingmodels.StatusPendingMatch,
		PickupLat:  6.6018,
		PickupLng:  3.3515,
		CreatedAt:  time.Now(),
	})

	svc := trackingTestService(t, store, repo, nil)
	svc.matchBooking(context.Background(), booking.ID, booking.PickupLat, booking.PickupLng)

	nearby, scan := store.counts()
	if nearby == 0 {
		t.Fatal("the index was never consulted")
	}
	if scan == 0 {
		t.Fatal("an empty index did not fall back to the online scan, so a pre-deploy provider would be unreachable")
	}
	// The offer is what matters here. The booking ends up unmatched because
	// the fake provider never accepts inside the 1s window, so assert on the
	// match lock the dispatch acquired rather than on the final status.
	if store.lockCount() == 0 {
		t.Fatal("no provider was offered the booking via the fallback path")
	}
}

func TestMatching_IndexErrorAbortsRatherThanScanning(t *testing.T) {
	store := newCountingStore()
	store.failNearby(errors.New("redis down"))
	repo := newTrackingRepo()
	booking := repo.put(bookingmodels.Booking{
		ID:         "b1",
		CustomerID: "c1",
		Status:     bookingmodels.StatusPendingMatch,
		PickupLat:  6.6018,
		PickupLng:  3.3515,
		CreatedAt:  time.Now(),
	})

	svc := trackingTestService(t, store, repo, nil)
	svc.matchBooking(context.Background(), booking.ID, booking.PickupLat, booking.PickupLng)

	if _, scan := store.counts(); scan != 0 {
		t.Fatalf("an index failure fell through to a full scan %d time(s); a Redis outage should not be masked", scan)
	}
}

// stubRoutes is a Fetcher that returns a fixed route and records its calls.
type stubRoutes struct {
	mu     sync.Mutex
	calls  [][2]geo.Point
	route  routing.Route
	err    error
	called chan struct{}
}

func newStubRoutes(route routing.Route, err error) *stubRoutes {
	return &stubRoutes{route: route, err: err, called: make(chan struct{}, 4)}
}

func (s *stubRoutes) Route(_ context.Context, origin, destination geo.Point, _ routing.TravelMode) (routing.Route, error) {
	s.mu.Lock()
	s.calls = append(s.calls, [2]geo.Point{origin, destination})
	s.mu.Unlock()
	select {
	case s.called <- struct{}{}:
	default:
	}
	return s.route, s.err
}

func (s *stubRoutes) waitForCall(t *testing.T) {
	t.Helper()
	select {
	case <-s.called:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a route lookup")
	}
}

const testPolyline = "_p~iF~ps|U_ulLnnqC_mqNvxq`@"

func TestCreateBooking_StoresTheDropoffRoute(t *testing.T) {
	store := newFakeStore()
	repo := newTrackingRepo()
	routes := newStubRoutes(routing.Route{Polyline: testPolyline, DistanceM: 24500, DurationS: 1830}, nil)

	svc := trackingTestService(t, store, repo, routes)
	pickup := geo.Point{Lat: 6.6018, Lng: 3.3515}
	dropoff := geo.Point{Lat: 6.4281, Lng: 3.4219}

	booking := repo.put(bookingmodels.Booking{ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch})
	svc.fetchDropoffRoute(booking.ID, pickup, dropoff)
	routes.waitForCall(t)

	waitForRouteStored(t, func() string { return repo.snapshot(booking.ID).RouteDropoffPolyline })
	stored := repo.snapshot(booking.ID)
	if stored.RouteDropoffPolyline != testPolyline {
		t.Fatalf("RouteDropoffPolyline = %q, want the fetched polyline", stored.RouteDropoffPolyline)
	}
	if stored.RouteDropoffDistanceM == nil || *stored.RouteDropoffDistanceM != 24500 {
		t.Fatalf("RouteDropoffDistanceM = %v, want 24500", stored.RouteDropoffDistanceM)
	}

	routes.mu.Lock()
	defer routes.mu.Unlock()
	if len(routes.calls) != 1 || routes.calls[0][0] != pickup || routes.calls[0][1] != dropoff {
		t.Fatalf("route fetched for %v, want pickup->dropoff %v -> %v", routes.calls, pickup, dropoff)
	}
}

// A routing outage must leave the booking untouched rather than failing it:
// the trip works with a straight line on the map.
func TestRouteFetchFailureLeavesBookingUsable(t *testing.T) {
	store := newFakeStore()
	repo := newTrackingRepo()
	routes := newStubRoutes(routing.Route{}, errors.New("routes api 403"))

	svc := trackingTestService(t, store, repo, routes)
	booking := repo.put(bookingmodels.Booking{ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch})

	svc.fetchDropoffRoute(booking.ID, geo.Point{Lat: 6.6, Lng: 3.35}, geo.Point{Lat: 6.42, Lng: 3.42})
	routes.waitForCall(t)
	time.Sleep(50 * time.Millisecond)

	if got := repo.snapshot(booking.ID).RouteDropoffPolyline; got != "" {
		t.Fatalf("RouteDropoffPolyline = %q, want it left empty after a routing failure", got)
	}
	if got := repo.snapshot(booking.ID).Status; got != bookingmodels.StatusPendingMatch {
		t.Fatalf("status = %q, want the booking unaffected by a routing failure", got)
	}
}

// With no API key the Routes fetcher is nil. Nothing must panic and no route
// may be stored.
func TestRoutesDisabledIsANoop(t *testing.T) {
	store := newFakeStore()
	repo := newTrackingRepo()
	svc := trackingTestService(t, store, repo, nil)

	booking := repo.put(bookingmodels.Booking{ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch})
	svc.fetchDropoffRoute(booking.ID, geo.Point{Lat: 6.6, Lng: 3.35}, geo.Point{Lat: 6.42, Lng: 3.42})
	svc.fetchPickupRoute(booking.ID, "p1", geo.Point{Lat: 6.6, Lng: 3.35})
	time.Sleep(50 * time.Millisecond)

	stored := repo.snapshot(booking.ID)
	if stored.RouteDropoffPolyline != "" || stored.RoutePickupPolyline != "" {
		t.Fatalf("routes were stored with no fetcher configured: %+v", stored)
	}
}

// The pickup leg starts wherever the driver is, so it can only be routed once
// their position is known.
func TestFetchPickupRouteUsesTheDriverPosition(t *testing.T) {
	store := newFakeStore()
	driver := availabilityrepositories.ProviderStatus{ProviderID: "p1", Lat: 6.5, Lng: 3.3}
	store.setStatus(driver)

	repo := newTrackingRepo()
	routes := newStubRoutes(routing.Route{Polyline: testPolyline, DistanceM: 5000, DurationS: 600}, nil)
	svc := trackingTestService(t, store, repo, routes)

	booking := repo.put(bookingmodels.Booking{ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusAccepted})
	pickup := geo.Point{Lat: 6.6018, Lng: 3.3515}
	svc.fetchPickupRoute(booking.ID, "p1", pickup)
	routes.waitForCall(t)

	if got := waitForRouteStored(t, func() string { return repo.snapshot(booking.ID).RoutePickupPolyline }); got != testPolyline {
		t.Fatalf("RoutePickupPolyline = %q, want the fetched polyline", got)
	}

	routes.mu.Lock()
	defer routes.mu.Unlock()
	if len(routes.calls) != 1 {
		t.Fatalf("expected exactly one route lookup, got %d", len(routes.calls))
	}
	if origin := routes.calls[0][0]; origin.Lat != driver.Lat || origin.Lng != driver.Lng {
		t.Fatalf("pickup route started at %v, want the driver position %v", origin, driver)
	}
	if dest := routes.calls[0][1]; dest != pickup {
		t.Fatalf("pickup route ended at %v, want the pickup %v", dest, pickup)
	}
}

// Without a driver fix there is nothing to route from; the lookup must be
// skipped rather than routed from null island.
func TestFetchPickupRouteSkippedWithoutADriverFix(t *testing.T) {
	store := newFakeStore()
	repo := newTrackingRepo()
	routes := newStubRoutes(routing.Route{Polyline: testPolyline}, nil)
	svc := trackingTestService(t, store, repo, routes)

	booking := repo.put(bookingmodels.Booking{ID: "b1", CustomerID: "c1", Status: bookingmodels.StatusAccepted})
	svc.fetchPickupRoute(booking.ID, "unknown-provider", geo.Point{Lat: 6.6018, Lng: 3.3515})
	time.Sleep(100 * time.Millisecond)

	routes.mu.Lock()
	defer routes.mu.Unlock()
	if len(routes.calls) != 0 {
		t.Fatalf("routed from a provider with no known position: %v", routes.calls)
	}
	if got := repo.snapshot(booking.ID).RoutePickupPolyline; got != "" {
		t.Fatalf("RoutePickupPolyline = %q, want empty", got)
	}
}

// The customer live-track socket authorises on booking ownership; without this
// check any customer could stream any driver by guessing an id.
func TestResolveTrackTarget(t *testing.T) {
	store := newFakeStore()
	repo := newTrackingRepo()
	svc := trackingTestService(t, store, repo, nil)

	providerID := "p1"
	assigned := repo.put(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", ProviderID: &providerID, Status: bookingmodels.StatusAccepted,
	})
	unassigned := repo.put(bookingmodels.Booking{
		ID: "b2", CustomerID: "c1", Status: bookingmodels.StatusPendingMatch,
	})

	target, err := svc.ResolveTrackTarget(context.Background(), assigned.ID, "c1")
	if err != nil {
		t.Fatalf("ResolveTrackTarget for the owner returned error: %v", err)
	}
	if target.ProviderID != providerID {
		t.Fatalf("ProviderID = %q, want %q", target.ProviderID, providerID)
	}

	if _, err := svc.ResolveTrackTarget(context.Background(), assigned.ID, "someone-else"); err == nil {
		t.Fatal("a non-owner was allowed to resolve a track target")
	}
	if _, err := svc.ResolveTrackTarget(context.Background(), unassigned.ID, "c1"); err == nil {
		t.Fatal("a booking with no assigned driver resolved a track target")
	}
}

// bookingFor stores a booking and returns it, mirroring how the other matcher
// tests seed the in-memory repo.
func (r *trackingRepo) put(b bookingmodels.Booking) bookingmodels.Booking {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bookings[b.ID] = b
	return b
}

func (r *trackingRepo) snapshot(id string) bookingmodels.Booking {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bookings[id]
}

// waitForRouteStored polls until a route lands or the deadline passes, because
// route fetching is deliberately asynchronous.
func waitForRouteStored(t *testing.T, read func() string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if value := read(); value != "" {
			return value
		}
		time.Sleep(10 * time.Millisecond)
	}
	return read()
}
