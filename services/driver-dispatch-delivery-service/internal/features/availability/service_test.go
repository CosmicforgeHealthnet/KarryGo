package availability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"karrygo/shared/go/apperrors"
)

func TestProviderCanOnlySetOnlineOrOffline(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	service := NewService(newFakeAvailabilityRepository(), newFakeLiveStore())

	for _, status := range []AvailabilityStatus{"", "paused", StatusBusy} {
		t.Run(string(status), func(t *testing.T) {
			_, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: status})
			if err == nil {
				t.Fatal("expected validation error")
			}
			var appErr *apperrors.Error
			if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeValidationFailed || appErr.Status != 400 {
				t.Fatalf("error = %#v, want 400 validation_failed", err)
			}
		})
	}
}

func TestOnlineGateChecksRunInOrderAndDoNotMutate(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()

	t.Run("not verified stops first", func(t *testing.T) {
		repo := newFakeAvailabilityRepository()
		live := newFakeLiveStore()
		events := &fakeEventPublisher{}
		seedProvider(repo, providerID, true, providerVerifiedStatus)
		service := NewService(repo, live, WithEventPublisher(events))

		_, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOnline})
		assertGateError(t, err, GateNotVerified)
		if got := repo.calls; !equalStrings(got, []string{"ensure_availability"}) {
			t.Fatalf("calls = %#v, want only ensure_availability", got)
		}
		assertNoOnlineMutation(t, repo, live, events, providerID)
	})

	t.Run("inactive account stops before bike check", func(t *testing.T) {
		repo := newFakeAvailabilityRepository()
		live := newFakeLiveStore()
		events := &fakeEventPublisher{}
		seedProvider(repo, providerID, false, providerVerifiedStatus)
		availability := repo.availability[providerID]
		availability.VerifiedToGoOnline = true
		repo.availability[providerID] = availability
		service := NewService(repo, live, WithEventPublisher(events))

		_, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOnline})
		assertGateError(t, err, GateAccountSuspended)
		if got := repo.calls; !equalStrings(got, []string{"ensure_availability", "get_provider_gate_state"}) {
			t.Fatalf("calls = %#v, want ensure then provider gate", got)
		}
		assertNoOnlineMutation(t, repo, live, events, providerID)
	})

	t.Run("no verified vehicle stops third", func(t *testing.T) {
		repo := newFakeAvailabilityRepository()
		live := newFakeLiveStore()
		events := &fakeEventPublisher{}
		seedEligibleProvider(repo, providerID)
		repo.hasBike[providerID] = false
		service := NewService(repo, live, WithEventPublisher(events))

		_, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOnline})
		assertGateError(t, err, GateNoVerifiedVehicle)
		if got := repo.calls; !equalStrings(got, []string{"ensure_availability", "get_provider_gate_state", "has_verified_active_bike"}) {
			t.Fatalf("calls = %#v, want ordered gate calls", got)
		}
		assertNoOnlineMutation(t, repo, live, events, providerID)
	})
}

func TestOnlineCreatesOneSessionSetsRedisAndPublishesOnce(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	events := &fakeEventPublisher{}
	seedEligibleProvider(repo, providerID)
	service := NewService(repo, live, WithClock(fixedClock("2026-06-06T10:00:00Z")), WithEventPublisher(events))

	first, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOnline})
	if err != nil {
		t.Fatalf("SetStatus first error = %v", err)
	}
	second, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOnline})
	if err != nil {
		t.Fatalf("SetStatus second error = %v", err)
	}
	if first.Status != StatusOnline || first.Message != "You are now online and visible to customers." {
		t.Fatalf("first response = %#v", first)
	}
	if second.Status != StatusOnline || second.Message != "You are already online." {
		t.Fatalf("second response = %#v", second)
	}
	if repo.sessionsCreated != 1 {
		t.Fatalf("sessionsCreated = %d, want 1", repo.sessionsCreated)
	}
	if events.onlineCount != 1 {
		t.Fatalf("online events = %d, want 1", events.onlineCount)
	}
	if live.statuses[providerID] != StatusOnline {
		t.Fatalf("redis status = %s, want online", live.statuses[providerID])
	}
	if live.discoverable[providerID] {
		t.Fatal("provider became discoverable before any location existed")
	}
	if repo.availability[providerID].Status != StatusOnline || repo.availability[providerID].SessionStart == nil {
		t.Fatalf("availability row not online with session_start: %#v", repo.availability[providerID])
	}
}

func TestOnlineRestoresGeoOnlyWhenLastLocationExists(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	seedEligibleProvider(repo, providerID)
	live.locations[providerID] = Location{ProviderID: providerID, Lat: 6.5244, Lng: 3.3792}
	service := NewService(repo, live)

	if _, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOnline}); err != nil {
		t.Fatalf("online error = %v", err)
	}
	if !live.discoverable[providerID] {
		t.Fatal("online provider with last location was not restored to GEO")
	}
}

func TestOfflineClosesSessionClearsLocationAndPublishesOnce(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	events := &fakeEventPublisher{}
	seedEligibleProvider(repo, providerID)
	service := NewService(repo, live, WithClock(fixedClock("2026-06-06T10:00:00Z")), WithEventPublisher(events))

	if _, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOnline}); err != nil {
		t.Fatalf("online error = %v", err)
	}
	live.locations[providerID] = Location{ProviderID: providerID, Lat: 6, Lng: 3}
	live.discoverable[providerID] = true

	service.now = fixedClock("2026-06-06T10:08:10Z")
	first, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOffline})
	if err != nil {
		t.Fatalf("offline error = %v", err)
	}
	second, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOffline})
	if err != nil {
		t.Fatalf("second offline error = %v", err)
	}
	if first.Message != "You are now offline." || second.Message != "You are already offline." {
		t.Fatalf("offline messages = %q/%q", first.Message, second.Message)
	}
	if live.statuses[providerID] != StatusOffline {
		t.Fatalf("redis status = %s, want offline", live.statuses[providerID])
	}
	if _, ok := live.locations[providerID]; ok {
		t.Fatal("location was not deleted")
	}
	if live.discoverable[providerID] {
		t.Fatal("provider remained discoverable")
	}
	if repo.sessionsEnded != 1 {
		t.Fatalf("sessionsEnded = %d, want 1", repo.sessionsEnded)
	}
	if repo.lastDurationMinutes != 9 {
		t.Fatalf("duration = %d, want ceil 9", repo.lastDurationMinutes)
	}
	if repo.lastForcedOffline {
		t.Fatal("manual offline should not force the session")
	}
	if events.offlineCount != 1 {
		t.Fatalf("offline events = %d, want 1", events.offlineCount)
	}
}

func TestGetAvailabilityDefaultsOfflineAndCleansStaleGeo(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	seedProvider(repo, providerID, true, providerVerifiedStatus)
	live.discoverable[providerID] = true
	service := NewService(repo, live, WithClock(fixedClock("2026-06-06T12:00:00Z")))

	response, err := service.GetStatus(ctx, providerID)
	if err != nil {
		t.Fatalf("GetStatus error = %v", err)
	}
	if response.Status != StatusOffline || response.SessionStart != nil || response.SessionDurationMinutes != 0 {
		t.Fatalf("offline response = %#v", response)
	}
	if response.HoursOnlineToday != 0 || response.TripsToday != 0 {
		t.Fatalf("empty stats = %#v", response)
	}
	if live.discoverable[providerID] {
		t.Fatal("stale GEO member was not removed when Redis status was missing")
	}
}

func TestGetAvailabilityReturnsOnlineDurationAndTodayStats(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	seedEligibleProvider(repo, providerID)
	now := fixedClock("2026-06-06T12:00:00Z")
	service := NewService(repo, live, WithClock(now))
	start := time.Date(2026, 6, 6, 11, 13, 0, 0, time.UTC)
	repo.availability[providerID] = Availability{
		ID:                 uuid.NewString(),
		ProviderID:         providerID,
		Status:             StatusOnline,
		VerifiedToGoOnline: true,
		SessionStart:       &start,
		LastChangedAt:      start,
		CreatedAt:          start,
	}
	live.statuses[providerID] = StatusOnline
	repo.closedMinutesToday[providerID] = 145
	repo.closedTripsToday[providerID] = 4
	repo.openSessions[providerID] = AvailabilitySession{
		ID:             uuid.NewString(),
		ProviderID:     providerID,
		WentOnlineAt:   start,
		TripsInSession: 2,
		CreatedAt:      start,
	}

	response, err := service.GetStatus(ctx, providerID)
	if err != nil {
		t.Fatalf("GetStatus error = %v", err)
	}
	if response.Status != StatusOnline {
		t.Fatalf("status = %s, want online", response.Status)
	}
	if response.SessionDurationMinutes != 47 {
		t.Fatalf("session duration = %d, want 47", response.SessionDurationMinutes)
	}
	if response.HoursOnlineToday != 3.2 {
		t.Fatalf("hours online = %v, want 3.2", response.HoursOnlineToday)
	}
	if response.TripsToday != 6 {
		t.Fatalf("trips today = %d, want 6", response.TripsToday)
	}
}

func TestBusyLocationIsNotDiscoverable(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	seedEligibleProvider(repo, providerID)
	// Phase 5F: UpdateLocation checks Redis status first, so we must seed it.
	live.statuses[providerID] = StatusBusy
	service := NewService(repo, live)

	if _, err := service.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 6, Lng: 3}); err != nil {
		t.Fatalf("UpdateLocation error = %v", err)
	}
	if live.discoverable[providerID] {
		t.Fatal("busy provider should not be discoverable")
	}
	if live.statuses[providerID] != StatusBusy {
		t.Fatalf("live status = %s, want busy", live.statuses[providerID])
	}
}

func TestUpdateLocationRequiresValidLocationAndOnlineStatus(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	seedEligibleProvider(repo, providerID)
	live := newFakeLiveStore()
	service := NewService(repo, live)

	// Validation runs before Redis check — invalid lat must fail regardless.
	if _, err := service.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 120, Lng: 3}); err == nil {
		t.Fatal("expected invalid latitude error")
	}
	// No Redis status → treat as offline → must reject.
	if _, err := service.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 6, Lng: 3}); err == nil {
		t.Fatal("expected offline bad_request error")
	}
}

func TestEndOpenSessionIsIdempotentAndDurationNeverNegative(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	seedEligibleProvider(repo, providerID)

	session, created, err := repo.CreateSessionIfNoneOpen(ctx, providerID, time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC))
	if err != nil || !created || session.ID == "" {
		t.Fatalf("CreateSessionIfNoneOpen = %#v %v %v", session, created, err)
	}
	_, created, err = repo.CreateSessionIfNoneOpen(ctx, providerID, time.Date(2026, 6, 6, 10, 1, 0, 0, time.UTC))
	if err != nil || created {
		t.Fatalf("duplicate CreateSessionIfNoneOpen created=%v err=%v", created, err)
	}
	ended, ok, err := repo.EndOpenSession(ctx, providerID, time.Date(2026, 6, 6, 9, 59, 0, 0, time.UTC), false)
	if err != nil || !ok {
		t.Fatalf("EndOpenSession ok=%v err=%v", ok, err)
	}
	if ended.DurationMinutes == nil || *ended.DurationMinutes != 0 {
		t.Fatalf("duration = %#v, want 0", ended.DurationMinutes)
	}
	_, ok, err = repo.EndOpenSession(ctx, providerID, time.Date(2026, 6, 6, 10, 2, 0, 0, time.UTC), false)
	if err != nil || ok {
		t.Fatalf("idempotent EndOpenSession ok=%v err=%v", ok, err)
	}
}

func assertGateError(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected gate error")
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T, want app error", err)
	}
	if appErr.Status != 403 || appErr.Code != apperrors.CodeForbidden {
		t.Fatalf("status/code = %d/%s, want 403/forbidden", appErr.Status, appErr.Code)
	}
	reason, ok := appErr.Reason.(GateError)
	if !ok {
		t.Fatalf("reason = %#v, want GateError", appErr.Reason)
	}
	if reason.Code != code {
		t.Fatalf("reason code = %s, want %s", reason.Code, code)
	}
}

func assertNoOnlineMutation(t *testing.T, repo *fakeAvailabilityRepository, live *fakeLiveStore, events *fakeEventPublisher, providerID string) {
	t.Helper()
	if repo.availability[providerID].Status == StatusOnline {
		t.Fatal("availability status mutated to online")
	}
	if repo.sessionsCreated != 0 {
		t.Fatalf("sessionsCreated = %d, want 0", repo.sessionsCreated)
	}
	if live.statuses[providerID] != "" {
		t.Fatalf("live status = %s, want empty", live.statuses[providerID])
	}
	if events.onlineCount != 0 {
		t.Fatalf("online events = %d, want 0", events.onlineCount)
	}
}

func seedProvider(repo *fakeAvailabilityRepository, providerID string, active bool, verificationStatus string) {
	repo.providers[providerID] = ProviderGateState{
		ProviderID:         providerID,
		IsActive:           active,
		VerificationStatus: verificationStatus,
	}
	now := time.Date(2026, 6, 6, 9, 0, 0, 0, time.UTC)
	repo.availability[providerID] = Availability{
		ID:            uuid.NewString(),
		ProviderID:    providerID,
		Status:        StatusOffline,
		LastChangedAt: now,
		CreatedAt:     now,
	}
}

func seedEligibleProvider(repo *fakeAvailabilityRepository, providerID string) {
	seedProvider(repo, providerID, true, providerVerifiedStatus)
	repo.hasBike[providerID] = true
	availability := repo.availability[providerID]
	availability.VerifiedToGoOnline = true
	repo.availability[providerID] = availability
}

type fakeAvailabilityRepository struct {
	providers           map[string]ProviderGateState
	availability        map[string]Availability
	hasBike             map[string]bool
	vehicleStep         map[string]bool
	openSessions        map[string]AvailabilitySession
	closedMinutesToday  map[string]int
	closedTripsToday    map[string]int
	calls               []string
	sessionsCreated     int
	sessionsEnded       int
	lastForcedOffline   bool
	lastDurationMinutes int
}

func newFakeAvailabilityRepository() *fakeAvailabilityRepository {
	return &fakeAvailabilityRepository{
		providers:          map[string]ProviderGateState{},
		availability:       map[string]Availability{},
		hasBike:            map[string]bool{},
		vehicleStep:        map[string]bool{},
		openSessions:       map[string]AvailabilitySession{},
		closedMinutesToday: map[string]int{},
		closedTripsToday:   map[string]int{},
	}
}

func (r *fakeAvailabilityRepository) EnsureAvailability(_ context.Context, providerID string) (Availability, error) {
	r.calls = append(r.calls, "ensure_availability")
	if availability, ok := r.availability[providerID]; ok {
		return availability, nil
	}
	now := time.Date(2026, 6, 6, 9, 0, 0, 0, time.UTC)
	availability := Availability{
		ID:            uuid.NewString(),
		ProviderID:    providerID,
		Status:        StatusOffline,
		LastChangedAt: now,
		CreatedAt:     now,
	}
	r.availability[providerID] = availability
	return availability, nil
}

func (r *fakeAvailabilityRepository) GetAvailability(_ context.Context, providerID string) (Availability, bool, error) {
	availability, ok := r.availability[providerID]
	return availability, ok, nil
}

func (r *fakeAvailabilityRepository) SetVerifiedToGoOnline(ctx context.Context, providerID string, verified bool) (Availability, error) {
	availability, err := r.EnsureAvailability(ctx, providerID)
	if err != nil {
		return Availability{}, err
	}
	availability.VerifiedToGoOnline = verified
	r.availability[providerID] = availability
	return availability, nil
}

func (r *fakeAvailabilityRepository) SetOnline(ctx context.Context, providerID string, changedAt time.Time) (Availability, error) {
	availability, err := r.EnsureAvailability(ctx, providerID)
	if err != nil {
		return Availability{}, err
	}
	availability.Status = StatusOnline
	if availability.SessionStart == nil {
		start := changedAt
		availability.SessionStart = &start
	}
	availability.LastChangedAt = changedAt
	r.availability[providerID] = availability
	return availability, nil
}

func (r *fakeAvailabilityRepository) SetOffline(ctx context.Context, providerID string, changedAt time.Time) (Availability, error) {
	availability, err := r.EnsureAvailability(ctx, providerID)
	if err != nil {
		return Availability{}, err
	}
	availability.Status = StatusOffline
	availability.SessionStart = nil
	availability.LastChangedAt = changedAt
	r.availability[providerID] = availability
	return availability, nil
}

func (r *fakeAvailabilityRepository) GetProviderGateState(_ context.Context, providerID string) (ProviderGateState, bool, error) {
	r.calls = append(r.calls, "get_provider_gate_state")
	state, ok := r.providers[providerID]
	return state, ok, nil
}

func (r *fakeAvailabilityRepository) HasVerifiedActiveBike(_ context.Context, providerID string) (bool, error) {
	r.calls = append(r.calls, "has_verified_active_bike")
	return r.hasBike[providerID], nil
}

func (r *fakeAvailabilityRepository) IsVehicleStepApproved(_ context.Context, providerID string) (bool, error) {
	return r.vehicleStep[providerID], nil
}

func (r *fakeAvailabilityRepository) CreateSessionIfNoneOpen(_ context.Context, providerID string, wentOnlineAt time.Time) (AvailabilitySession, bool, error) {
	if session, ok := r.openSessions[providerID]; ok {
		return session, false, nil
	}
	session := AvailabilitySession{
		ID:           uuid.NewString(),
		ProviderID:   providerID,
		WentOnlineAt: wentOnlineAt,
		CreatedAt:    wentOnlineAt,
	}
	r.sessionsCreated++
	r.openSessions[providerID] = session
	return session, true, nil
}

func (r *fakeAvailabilityRepository) GetOpenSession(_ context.Context, providerID string) (AvailabilitySession, bool, error) {
	session, ok := r.openSessions[providerID]
	return session, ok, nil
}

func (r *fakeAvailabilityRepository) EndOpenSession(_ context.Context, providerID string, wentOfflineAt time.Time, forced bool) (AvailabilitySession, bool, error) {
	session, ok := r.openSessions[providerID]
	if !ok {
		return AvailabilitySession{}, false, nil
	}
	duration := ceilMinutes(wentOfflineAt.Sub(session.WentOnlineAt))
	if duration < 0 {
		duration = 0
	}
	session.WentOfflineAt = &wentOfflineAt
	session.DurationMinutes = &duration
	session.ForcedOffline = forced
	delete(r.openSessions, providerID)
	r.sessionsEnded++
	r.lastForcedOffline = forced
	r.lastDurationMinutes = duration
	return session, true, nil
}

func (r *fakeAvailabilityRepository) SetBusy(ctx context.Context, providerID string, changedAt time.Time) (Availability, error) {
	availability, err := r.EnsureAvailability(ctx, providerID)
	if err != nil {
		return Availability{}, err
	}
	availability.Status = StatusBusy
	availability.LastChangedAt = changedAt
	r.availability[providerID] = availability
	return availability, nil
}

func (r *fakeAvailabilityRepository) IncrementOpenSessionTrips(_ context.Context, providerID string) error {
	if session, ok := r.openSessions[providerID]; ok {
		session.TripsInSession++
		r.openSessions[providerID] = session
	}
	return nil
}

func (r *fakeAvailabilityRepository) GetTodayAvailabilityStats(_ context.Context, providerID string, _ time.Time, now time.Time) (TodayAvailabilityStats, error) {
	minutes := r.closedMinutesToday[providerID]
	trips := r.closedTripsToday[providerID]
	if session, ok := r.openSessions[providerID]; ok {
		minutes += int(now.Sub(session.WentOnlineAt).Minutes())
		trips += session.TripsInSession
	}
	return TodayAvailabilityStats{MinutesOnline: minutes, Trips: trips}, nil
}

type fakeLiveStore struct {
	statuses     map[string]AvailabilityStatus
	locations    map[string]Location
	discoverable map[string]bool
	nearby       []NearbyProvider
}

func newFakeLiveStore() *fakeLiveStore {
	return &fakeLiveStore{
		statuses:     map[string]AvailabilityStatus{},
		locations:    map[string]Location{},
		discoverable: map[string]bool{},
	}
}

func (s *fakeLiveStore) SetStatus(_ context.Context, providerID string, status AvailabilityStatus) error {
	s.statuses[providerID] = status
	return nil
}

func (s *fakeLiveStore) GetStatus(_ context.Context, providerID string) (AvailabilityStatus, bool, error) {
	status, ok := s.statuses[providerID]
	return status, ok, nil
}

func (s *fakeLiveStore) ClearProvider(_ context.Context, providerID string) error {
	s.statuses[providerID] = StatusOffline
	delete(s.locations, providerID)
	s.discoverable[providerID] = false
	return nil
}

func (s *fakeLiveStore) RemoveFromGeo(_ context.Context, providerID string) error {
	s.discoverable[providerID] = false
	return nil
}

func (s *fakeLiveStore) RestoreGeoFromLocation(ctx context.Context, providerID string) (bool, error) {
	if _, ok, _ := s.GetLocation(ctx, providerID); !ok {
		return false, nil
	}
	s.discoverable[providerID] = true
	return true, nil
}

func (s *fakeLiveStore) SetLocation(_ context.Context, providerID string, location Location, discoverable bool) error {
	s.locations[providerID] = location
	s.discoverable[providerID] = discoverable
	return nil
}

func (s *fakeLiveStore) GetLocation(_ context.Context, providerID string) (Location, bool, error) {
	location, ok := s.locations[providerID]
	return location, ok, nil
}

func (s *fakeLiveStore) GetNearby(context.Context, NearbyProvidersRequest) ([]NearbyProvider, error) {
	return s.nearby, nil
}

type fakeEventPublisher struct {
	onlineCount   int
	offlineCount  int
	locationCount int
}

func (p *fakeEventPublisher) PublishProviderWentOnline(context.Context, ProviderWentOnlineEvent) error {
	p.onlineCount++
	return nil
}

func (p *fakeEventPublisher) PublishProviderWentOffline(context.Context, ProviderWentOfflineEvent) error {
	p.offlineCount++
	return nil
}

func (p *fakeEventPublisher) PublishProviderLocationUpdated(context.Context, ProviderLocationUpdatedEvent) error {
	p.locationCount++
	return nil
}

func fixedClock(value string) func() time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return func() time.Time { return parsed }
}

func ceilMinutes(duration time.Duration) int {
	minutes := duration.Minutes()
	if minutes <= 0 {
		return int(minutes)
	}
	if minutes == float64(int(minutes)) {
		return int(minutes)
	}
	return int(minutes) + 1
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
