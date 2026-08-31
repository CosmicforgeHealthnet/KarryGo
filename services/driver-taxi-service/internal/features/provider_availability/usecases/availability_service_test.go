package availabilityusecases

import (
	"context"
	"testing"
	"time"

	marketmodels "cosmicforge/logistics/services/taxi-service/internal/features/markets/models"
	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	providerprofilemodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
)

// ─── fakes ────────────────────────────────────────────────────────────────────

type fakeStore struct {
	statuses map[string]availabilityrepositories.ProviderStatus
	// nearbyEmpty simulates the post-deploy window where drivers hold live
	// status keys but have not yet heartbeated into the spatial index.
	nearbyEmpty bool
	rejected    map[string]map[string]bool
	locks       map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		statuses: map[string]availabilityrepositories.ProviderStatus{},
		rejected: map[string]map[string]bool{},
		locks:    map[string]string{},
	}
}

func (f *fakeStore) SetOnline(_ context.Context, s availabilityrepositories.ProviderStatus) error {
	s.UpdatedAt = time.Now().Unix()
	f.statuses[s.ProviderID] = s
	return nil
}

func (f *fakeStore) SetOffline(_ context.Context, id string) error {
	delete(f.statuses, id)
	return nil
}

func (f *fakeStore) Heartbeat(ctx context.Context, s availabilityrepositories.ProviderStatus) error {
	return f.SetOnline(ctx, s)
}

func (f *fakeStore) GetStatus(_ context.Context, id string) (availabilityrepositories.ProviderStatus, bool, error) {
	s, ok := f.statuses[id]
	return s, ok, nil
}

func (f *fakeStore) GetOnlineProviders(context.Context) ([]availabilityrepositories.ProviderStatus, error) {
	out := make([]availabilityrepositories.ProviderStatus, 0, len(f.statuses))
	for _, s := range f.statuses {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeStore) NearbyProviders(_ context.Context, marketID, vehicleClass string, center geo.Point, radiusKm float64, _ int) ([]availabilityrepositories.ProviderStatus, error) {
	if f.nearbyEmpty {
		return nil, nil
	}
	var out []availabilityrepositories.ProviderStatus
	for _, s := range f.statuses {
		// A real partitioned index only ever returns hits from the requested
		// (market, class) partition. Filtering here too keeps the fake honest,
		// since CheckAvailability now queries once per bookable class and sums
		// the results -- without this filter, a single online driver would be
		// double- or triple-counted.
		if s.MarketID != marketID || s.VehicleClass != vehicleClass {
			continue
		}
		if geo.HaversineKm(center, geo.Point{Lat: s.Lat, Lng: s.Lng}) <= radiusKm {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) SetDiscoverable(context.Context, string, string, string, bool) error { return nil }

func (f *fakeStore) AcquireMatchLock(_ context.Context, providerID, bookingID string, _ time.Duration) (bool, error) {
	if _, held := f.locks[providerID]; held {
		return false, nil
	}
	f.locks[providerID] = bookingID
	return true, nil
}

func (f *fakeStore) ReleaseMatchLock(_ context.Context, providerID string) error {
	delete(f.locks, providerID)
	return nil
}

func (f *fakeStore) MarkRejected(_ context.Context, bookingID, providerID string, _ time.Duration) error {
	if f.rejected[bookingID] == nil {
		f.rejected[bookingID] = map[string]bool{}
	}
	f.rejected[bookingID][providerID] = true
	return nil
}

func (f *fakeStore) GetRejectedProviders(_ context.Context, bookingID string) (map[string]bool, error) {
	return f.rejected[bookingID], nil
}

type fakeProviders struct {
	byID map[string]providerauthmodels.Provider
}

func (f fakeProviders) GetByID(_ context.Context, id string) (providerauthmodels.Provider, error) {
	p, ok := f.byID[id]
	if !ok {
		return providerauthmodels.Provider{}, apperrors.NotFound("Driver could not be found.", nil)
	}
	return p, nil
}

type fakeVehicles struct {
	byProvider map[string][]providerprofilemodels.Vehicle
}

func (f fakeVehicles) CountActiveByProvider(_ context.Context, providerID string) (int, error) {
	n := 0
	for _, v := range f.byProvider[providerID] {
		if v.Status == providerprofilemodels.VehicleStatusActive {
			n++
		}
	}
	return n, nil
}

func (f fakeVehicles) GetByID(_ context.Context, id, providerID string) (providerprofilemodels.Vehicle, error) {
	for _, v := range f.byProvider[providerID] {
		if v.ID == id {
			return v, nil
		}
	}
	return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", nil)
}

func (f fakeVehicles) FirstActiveByProvider(_ context.Context, providerID string) (providerprofilemodels.Vehicle, error) {
	for _, v := range f.byProvider[providerID] {
		if v.Status == providerprofilemodels.VehicleStatusActive {
			return v, nil
		}
	}
	return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", nil)
}

// reasonOf pulls the machine-readable gate reason out of the error envelope.
func reasonOf(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Details == nil {
		t.Fatalf("expected details on the error, got %v", err)
	}
	reason, _ := appErr.Details["reason"].(string)
	return reason
}

const lagosLat, lagosLng = 6.52, 3.37

// compliantProvider returns a Provider that passes every one of
// CanGoAvailable's five checked dimensions, so tests that are not specifically
// about one dimension failing do not need to restate all five every time.
func compliantProvider(id string) providerauthmodels.Provider {
	return providerauthmodels.Provider{
		ID:                         id,
		AccountStatus:              providerauthmodels.AccountStatusActive,
		OnboardingStatus:           providerauthmodels.OnboardingComplete,
		IdentityVerificationStatus: providerauthmodels.IdentityVerificationVerified,
		VehicleVerificationStatus:  providerauthmodels.VehicleVerificationVerified,
		DocumentComplianceStatus:   providerauthmodels.DocumentComplianceCompliant,
	}
}

func readyDriver() (fakeProviders, fakeVehicles) {
	return fakeProviders{byID: map[string]providerauthmodels.Provider{
			"driver-1": compliantProvider("driver-1"),
		}},
		fakeVehicles{byProvider: map[string][]providerprofilemodels.Vehicle{
			"driver-1": {{ID: "veh-1", ProviderID: "driver-1", VehicleClass: "economy", Seats: 4, Status: providerprofilemodels.VehicleStatusActive}},
		}}
}

// fakeOperationalStates is an in-memory OperationalStateRepository. It applies
// the exact same guarded-CAS semantics as the real Postgres implementation
// (see operational_state_repository.go / its integration tests) so a test
// against this fake exercises the same contract SetAvailability depends on.
type fakeOperationalStates struct {
	byProvider map[string]availabilitymodels.State
}

func newFakeOperationalStates() *fakeOperationalStates {
	return &fakeOperationalStates{byProvider: map[string]availabilitymodels.State{}}
}

func (f *fakeOperationalStates) Get(_ context.Context, providerID string) (availabilitymodels.State, error) {
	if s, ok := f.byProvider[providerID]; ok {
		return s, nil
	}
	return availabilitymodels.State{ProviderID: providerID, Current: availabilitymodels.StateOffline, Version: 0}, nil
}

func (f *fakeOperationalStates) Transition(_ context.Context, _ availabilityrepositories.Executor, providerID string, to availabilitymodels.OperationalState, reason string, bookingID *string, expectedVersion int64) (availabilitymodels.State, error) {
	current, _ := f.Get(context.Background(), providerID)
	if current.Version != expectedVersion {
		return availabilitymodels.State{}, apperrors.Conflict("stale version", nil)
	}
	next := availabilitymodels.State{
		ProviderID: providerID, Current: to, Reason: reason, BookingID: bookingID,
		EffectiveAt: time.Now(), Version: current.Version + 1,
	}
	f.byProvider[providerID] = next
	return next, nil
}

// fakeMarkets resolves a single circular test zone centered on the module's
// lagosLat/lagosLng constant, radius 10km -- matching what every existing test
// already assumed implicitly before market resolution was a real gate. A point
// outside that radius (the Abuja fixture several tests use) genuinely fails to
// resolve, exactly as a real unmapped coordinate would.
type fakeMarkets struct {
	zone marketmodels.Zone
}

func newFakeMarkets() *fakeMarkets {
	return &fakeMarkets{zone: marketmodels.Zone{
		ID:               "zone-lagos",
		MarketID:         "market-lagos",
		Name:             "Lagos",
		CenterLat:        lagosLat,
		CenterLng:        lagosLng,
		RadiusKm:         10,
		SupportedClasses: []string{"economy", "comfort", "xl"},
		Status:           "active",
	}}
}

func (f *fakeMarkets) ResolveZone(_ context.Context, point geo.Point) (marketmodels.Zone, error) {
	if geo.HaversineKm(point, geo.Point{Lat: f.zone.CenterLat, Lng: f.zone.CenterLng}) <= f.zone.RadiusKm {
		return f.zone, nil
	}
	return marketmodels.Zone{}, apperrors.NotFound("We do not operate in this area yet.", nil)
}

func newSvc(store *fakeStore, providers fakeProviders, vehicles fakeVehicles) *AvailabilityService {
	return NewAvailabilityService(store, newFakeOperationalStates(), providers, vehicles, newFakeMarkets(), 90*time.Second)
}

// ─── gate tests ───────────────────────────────────────────────────────────────

func TestGoOnlineBlockedWhileOnboardingIncomplete(t *testing.T) {
	provider := compliantProvider("driver-1")
	provider.OnboardingStatus = providerauthmodels.OnboardingProfileNeeded
	providers := fakeProviders{byID: map[string]providerauthmodels.Provider{"driver-1": provider}}
	svc := newSvc(newFakeStore(), providers, fakeVehicles{byProvider: map[string][]providerprofilemodels.Vehicle{}})

	_, err := svc.SetAvailability(context.Background(), SetAvailabilityInput{
		ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng,
	})
	if got := reasonOf(t, err); got != ReasonProfileIncomplete {
		t.Fatalf("reason = %q, want %q", got, ReasonProfileIncomplete)
	}
}

// The other three addendum §11.1 dimensions each block independently too, with
// their own reason code, and in every case before the (unrelated) vehicle
// check ever runs -- proving CanGoAvailable's ordering doesn't mask one
// dimension's failure behind another's.
func TestGoOnlineBlockedByEachRemainingComplianceDimension(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(*providerauthmodels.Provider)
		wantReason string
	}{
		{"identity verification not done", func(p *providerauthmodels.Provider) {
			p.IdentityVerificationStatus = providerauthmodels.IdentityVerificationPending
		}, ReasonIdentityVerificationPending},
		{"vehicle verification not done", func(p *providerauthmodels.Provider) {
			p.VehicleVerificationStatus = providerauthmodels.VehicleVerificationFailed
		}, ReasonVehicleVerificationPending},
		{"document compliance expired", func(p *providerauthmodels.Provider) {
			p.DocumentComplianceStatus = providerauthmodels.DocumentComplianceExpired
		}, ReasonDocumentComplianceRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := compliantProvider("driver-1")
			tc.mutate(&provider)
			providers := fakeProviders{byID: map[string]providerauthmodels.Provider{"driver-1": provider}}
			// A registered active vehicle is present so a vehicle-related failure
			// cannot be mistaken for the dimension actually under test.
			vehicles := fakeVehicles{byProvider: map[string][]providerprofilemodels.Vehicle{
				"driver-1": {{ID: "veh-1", ProviderID: "driver-1", VehicleClass: "economy", Seats: 4, Status: providerprofilemodels.VehicleStatusActive}},
			}}
			svc := newSvc(newFakeStore(), providers, vehicles)

			_, err := svc.SetAvailability(context.Background(), SetAvailabilityInput{
				ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng,
			})
			if got := reasonOf(t, err); got != tc.wantReason {
				t.Fatalf("reason = %q, want %q", got, tc.wantReason)
			}
		})
	}
}

func TestGoOnlineBlockedWithoutActiveVehicle(t *testing.T) {
	providers, _ := readyDriver()
	svc := newSvc(newFakeStore(), providers, fakeVehicles{byProvider: map[string][]providerprofilemodels.Vehicle{}})

	_, err := svc.SetAvailability(context.Background(), SetAvailabilityInput{
		ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng,
	})
	if got := reasonOf(t, err); got != ReasonNoActiveVehicle {
		t.Fatalf("reason = %q, want %q", got, ReasonNoActiveVehicle)
	}
}

func TestGoOnlineBlockedWhenSuspended(t *testing.T) {
	_, vehicles := readyDriver()
	provider := compliantProvider("driver-1")
	provider.AccountStatus = providerauthmodels.AccountStatusSuspended
	providers := fakeProviders{byID: map[string]providerauthmodels.Provider{"driver-1": provider}}
	svc := newSvc(newFakeStore(), providers, vehicles)

	_, err := svc.SetAvailability(context.Background(), SetAvailabilityInput{
		ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng,
	})
	if got := reasonOf(t, err); got != ReasonAccountSuspended {
		t.Fatalf("reason = %q, want %q", got, ReasonAccountSuspended)
	}
}

func TestGoOnlineSucceedsAndBindsVehicle(t *testing.T) {
	providers, vehicles := readyDriver()
	svc := newSvc(newFakeStore(), providers, vehicles)

	state, err := svc.SetAvailability(context.Background(), SetAvailabilityInput{
		ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng,
	})
	if err != nil {
		t.Fatalf("SetAvailability: %v", err)
	}
	if !state.Online {
		t.Fatal("driver should be online")
	}
	// The matcher filters on the vehicle's class and seats, so a driver must
	// never be online without one bound.
	if state.VehicleID != "veh-1" {
		t.Fatalf("expected the active vehicle to be bound, got %q", state.VehicleID)
	}
}

func TestSetAvailabilityRejectsUnknownStatus(t *testing.T) {
	providers, vehicles := readyDriver()
	svc := newSvc(newFakeStore(), providers, vehicles)

	if _, err := svc.SetAvailability(context.Background(), SetAvailabilityInput{
		ProviderID: "driver-1", Status: "busy",
	}); err == nil {
		t.Fatal("only online/offline are accepted")
	}
}

func TestHeartbeatRequiresBeingOnline(t *testing.T) {
	// A heartbeat must not be able to put a driver back into the matching pool
	// without passing the online gate.
	providers, vehicles := readyDriver()
	svc := newSvc(newFakeStore(), providers, vehicles)

	if _, err := svc.Heartbeat(context.Background(), SetAvailabilityInput{
		ProviderID: "driver-1", Lat: lagosLat, Lng: lagosLng,
	}, false); err == nil {
		t.Fatal("a heartbeat from an offline driver must be refused")
	}
}

func TestHeartbeatCadenceTightensOnTrip(t *testing.T) {
	providers, vehicles := readyDriver()
	store := newFakeStore()
	svc := newSvc(store, providers, vehicles)
	ctx := context.Background()

	if _, err := svc.SetAvailability(ctx, SetAvailabilityInput{
		ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng,
	}); err != nil {
		t.Fatalf("SetAvailability: %v", err)
	}

	idle, err := svc.Heartbeat(ctx, SetAvailabilityInput{ProviderID: "driver-1", Lat: lagosLat, Lng: lagosLng}, false)
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	onTrip, err := svc.Heartbeat(ctx, SetAvailabilityInput{ProviderID: "driver-1", Lat: lagosLat, Lng: lagosLng}, true)
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if idle.HeartbeatSeconds != IdleHeartbeatSeconds || onTrip.HeartbeatSeconds != OnTripHeartbeatSeconds {
		t.Fatalf("cadence should tighten on trip: idle=%d ontrip=%d", idle.HeartbeatSeconds, onTrip.HeartbeatSeconds)
	}
}

func TestHeartbeatKeepsBoundVehicleWhenNotResent(t *testing.T) {
	providers, vehicles := readyDriver()
	store := newFakeStore()
	svc := newSvc(store, providers, vehicles)
	ctx := context.Background()

	_, _ = svc.SetAvailability(ctx, SetAvailabilityInput{ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng})
	state, err := svc.Heartbeat(ctx, SetAvailabilityInput{ProviderID: "driver-1", Lat: lagosLat, Lng: lagosLng}, false)
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if state.VehicleID != "veh-1" {
		t.Fatalf("heartbeat must not drop the bound vehicle, got %q", state.VehicleID)
	}
}

// ─── customer availability ────────────────────────────────────────────────────

func TestCheckAvailabilityCountsOnlyDriversWithinRadius(t *testing.T) {
	providers, vehicles := readyDriver()
	store := newFakeStore()
	svc := newSvc(store, providers, vehicles)
	ctx := context.Background()

	_, _ = svc.SetAvailability(ctx, SetAvailabilityInput{ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng})

	near, err := svc.CheckAvailability(ctx, geo.Point{Lat: lagosLat, Lng: lagosLng}, 10)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if !near.Available || near.Count != 1 {
		t.Fatalf("a driver at the pickup point should be available, got %+v", near)
	}

	// Abuja, ~530km away.
	far, err := svc.CheckAvailability(ctx, geo.Point{Lat: 9.06, Lng: 7.49}, 10)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if far.Available || far.Count != 0 {
		t.Fatalf("a driver 500km away must not count as available, got %+v", far)
	}
}

// Regression: the index-empty fallback was lifted from the matcher, where a
// haversine filter runs immediately afterwards. Used unfiltered here it told a
// passenger in Abuja that a ride was available because one driver was online in
// Lagos. The fallback must still respect the radius.
func TestCheckAvailabilityFallbackStillAppliesRadius(t *testing.T) {
	providers, vehicles := readyDriver()
	store := newFakeStore()
	store.nearbyEmpty = true // simulate the post-deploy empty index
	svc := newSvc(store, providers, vehicles)
	ctx := context.Background()

	_, _ = svc.SetAvailability(ctx, SetAvailabilityInput{ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng})

	far, err := svc.CheckAvailability(ctx, geo.Point{Lat: 9.06, Lng: 7.49}, 10)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if far.Available {
		t.Fatal("the scan fallback must not report a distant driver as available")
	}

	// The fallback must still find a genuinely nearby driver, which is the whole
	// reason it exists.
	near, err := svc.CheckAvailability(ctx, geo.Point{Lat: lagosLat, Lng: lagosLng}, 10)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if !near.Available || near.Count != 1 {
		t.Fatalf("the fallback should still find a nearby driver, got %+v", near)
	}
}

func TestCheckAvailabilityWithoutPickupAsksIfServiceIsLive(t *testing.T) {
	providers, vehicles := readyDriver()
	store := newFakeStore()
	svc := newSvc(store, providers, vehicles)
	ctx := context.Background()

	empty, err := svc.CheckAvailability(ctx, geo.Point{}, 0)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if empty.Available {
		t.Fatal("no drivers online should report unavailable")
	}

	_, _ = svc.SetAvailability(ctx, SetAvailabilityInput{ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng})
	live, err := svc.CheckAvailability(ctx, geo.Point{}, 0)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if !live.Available || live.Count != 1 {
		t.Fatalf("expected the global online count, got %+v", live)
	}
}

func TestGoingOfflineRemovesDriverFromAvailability(t *testing.T) {
	providers, vehicles := readyDriver()
	store := newFakeStore()
	svc := newSvc(store, providers, vehicles)
	ctx := context.Background()

	_, _ = svc.SetAvailability(ctx, SetAvailabilityInput{ProviderID: "driver-1", Status: "online", Lat: lagosLat, Lng: lagosLng})
	if _, err := svc.SetAvailability(ctx, SetAvailabilityInput{ProviderID: "driver-1", Status: "offline"}); err != nil {
		t.Fatalf("go offline: %v", err)
	}

	result, err := svc.CheckAvailability(ctx, geo.Point{Lat: lagosLat, Lng: lagosLng}, 10)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if result.Available {
		t.Fatal("an offline driver must not be bookable")
	}
}
