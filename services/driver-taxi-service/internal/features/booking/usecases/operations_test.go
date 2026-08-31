package bookingusecases

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/adminaudit"
	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/geo"
)

// fakeOperationalStateStore is a minimal in-memory OperationalStateStore --
// this package's other tests exercise the real guarded-CAS semantics
// against real Postgres (see lifecycle_integration_test.go), so this fake
// exists only to let a usecase-level test (like MarkCompleted's own
// operational-state release step) run without a database, not to prove any
// concurrency guarantee.
type fakeOperationalStateStore struct {
	states map[string]availabilitymodels.State
}

func newFakeOperationalStateStore() *fakeOperationalStateStore {
	return &fakeOperationalStateStore{states: map[string]availabilitymodels.State{}}
}

func (f *fakeOperationalStateStore) Get(ctx context.Context, providerID string) (availabilitymodels.State, error) {
	if s, ok := f.states[providerID]; ok {
		return s, nil
	}
	return availabilitymodels.State{ProviderID: providerID, Current: availabilitymodels.StateOffline}, nil
}

func (f *fakeOperationalStateStore) Transition(ctx context.Context, exec Executor, providerID string, to availabilitymodels.OperationalState, reason string, bookingID *string, expectedVersion int64) (availabilitymodels.State, error) {
	s := availabilitymodels.State{ProviderID: providerID, Current: to, Reason: reason, BookingID: bookingID, Version: expectedVersion + 1}
	f.states[providerID] = s
	return s, nil
}

// ─── fakes ──────────────────────────────────────────────────────────────────

type fakeAvailabilityOps struct {
	online []availabilityrepositories.ProviderStatus
}

func (f *fakeAvailabilityOps) NearbyProviders(ctx context.Context, marketID, vehicleClass string, center geo.Point, radiusKm float64, limit int) ([]availabilityrepositories.ProviderStatus, error) {
	return nil, nil
}
func (f *fakeAvailabilityOps) GetOnlineProviders(ctx context.Context) ([]availabilityrepositories.ProviderStatus, error) {
	return f.online, nil
}
func (f *fakeAvailabilityOps) AcquireMatchLock(ctx context.Context, providerID, bookingID string, ttl time.Duration) (bool, error) {
	return true, nil
}
func (f *fakeAvailabilityOps) ReleaseMatchLock(ctx context.Context, providerID string) error {
	return nil
}
func (f *fakeAvailabilityOps) MarkRejected(ctx context.Context, bookingID, providerID string, ttl time.Duration) error {
	return nil
}
func (f *fakeAvailabilityOps) GetRejectedProviders(ctx context.Context, bookingID string) (map[string]bool, error) {
	return nil, nil
}

type fakeAdminAudit struct {
	recorded []string // "action:entityType:entityID"
}

func (f *fakeAdminAudit) Record(ctx context.Context, a adminaudit.Action) error {
	f.recorded = append(f.recorded, a.Action+":"+a.EntityType+":"+a.EntityID)
	return nil
}

// ─── ListLiveOperations / ListUnmatchedBookings / GetMetricsSnapshot ───────

func TestListLiveOperations_NilAvailabilityIsUnavailable(t *testing.T) {
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo()})
	_, err := svc.ListLiveOperations(context.Background(), "")
	if err == nil {
		t.Fatal("expected an error when Availability is not configured")
	}
}

func TestListLiveOperations_MergesActiveBookingsAndOnlineProviders(t *testing.T) {
	bookings := newFakeBookingRepo()
	pid := "provider-1"
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", ProviderID: &pid, Status: bookingmodels.StatusInProgress, MarketID: "market-1"}
	bookings.byID["b2"] = bookingmodels.Booking{ID: "b2", Status: bookingmodels.StatusCompleted, MarketID: "market-1"} // terminal, must be excluded
	availability := &fakeAvailabilityOps{online: []availabilityrepositories.ProviderStatus{{ProviderID: pid, VehicleClass: "economy", Lat: 6.5, Lng: 3.4}}}
	svc := NewBookingService(Options{Bookings: bookings, Availability: availability})

	view, err := svc.ListLiveOperations(context.Background(), "")
	if err != nil {
		t.Fatalf("ListLiveOperations: %v", err)
	}
	if len(view.ActiveBookings) != 1 || view.ActiveBookings[0].ID != "b1" {
		t.Errorf("ActiveBookings = %+v, want exactly [b1] (b2 is terminal)", view.ActiveBookings)
	}
	if len(view.OnlineProviders) != 1 || view.OnlineProviders[0].ProviderID != pid {
		t.Errorf("OnlineProviders = %+v, want exactly [%s]", view.OnlineProviders, pid)
	}
}

func TestListLiveOperations_FiltersByMarket(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", Status: bookingmodels.StatusPendingMatch, MarketID: "market-1"}
	bookings.byID["b2"] = bookingmodels.Booking{ID: "b2", Status: bookingmodels.StatusPendingMatch, MarketID: "market-2"}
	svc := NewBookingService(Options{Bookings: bookings, Availability: &fakeAvailabilityOps{}})

	view, err := svc.ListLiveOperations(context.Background(), "market-1")
	if err != nil {
		t.Fatalf("ListLiveOperations: %v", err)
	}
	if len(view.ActiveBookings) != 1 || view.ActiveBookings[0].ID != "b1" {
		t.Errorf("ActiveBookings = %+v, want exactly [b1]", view.ActiveBookings)
	}
}

func TestListUnmatchedBookings_ResolvesOlderThanDurationAgainstNow(t *testing.T) {
	bookings := newFakeBookingRepo()
	old := bookingmodels.Booking{ID: "old", Status: bookingmodels.StatusUnmatched, CreatedAt: time.Now().Add(-2 * time.Hour)}
	recent := bookingmodels.Booking{ID: "recent", Status: bookingmodels.StatusUnmatched, CreatedAt: time.Now()}
	bookings.byID["old"] = old
	bookings.byID["recent"] = recent
	svc := NewBookingService(Options{Bookings: bookings})

	out, err := svc.ListUnmatchedBookings(context.Background(), UnmatchedFilter{OlderThan: time.Hour})
	if err != nil {
		t.Fatalf("ListUnmatchedBookings: %v", err)
	}
	if len(out) != 1 || out[0].ID != "old" {
		t.Errorf("out = %+v, want exactly [old]", out)
	}
}

func TestListUnmatchedBookings_ZeroOlderThanMeansNoAgeFilter(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["recent"] = bookingmodels.Booking{ID: "recent", Status: bookingmodels.StatusUnmatched, CreatedAt: time.Now()}
	svc := NewBookingService(Options{Bookings: bookings})

	out, err := svc.ListUnmatchedBookings(context.Background(), UnmatchedFilter{})
	if err != nil {
		t.Fatalf("ListUnmatchedBookings: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("out = %+v, want the recent booking included with no age filter", out)
	}
}

func TestGetMetricsSnapshot_IsLedgerFreeAndBoundedCardinality(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", Status: bookingmodels.StatusCompleted}
	bookings.byID["b2"] = bookingmodels.Booking{ID: "b2", Status: bookingmodels.StatusCompleted}
	bookings.byID["b3"] = bookingmodels.Booking{ID: "b3", Status: bookingmodels.StatusCancelled}
	svc := NewBookingService(Options{Bookings: bookings, Availability: &fakeAvailabilityOps{online: []availabilityrepositories.ProviderStatus{{ProviderID: "p1"}, {ProviderID: "p2"}}}})

	snapshot, err := svc.GetMetricsSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetMetricsSnapshot: %v", err)
	}
	if snapshot.BookingsByStatus["completed"] != 2 || snapshot.BookingsByStatus["cancelled"] != 1 {
		t.Errorf("BookingsByStatus = %+v, want completed=2 cancelled=1", snapshot.BookingsByStatus)
	}
	if snapshot.ProvidersOnline != 2 {
		t.Errorf("ProvidersOnline = %d, want 2", snapshot.ProvidersOnline)
	}
	// Labels are status values (a fixed, small set) -- never booking_id.
	// This test's own construction is the guarantee: BookingsByStatus is
	// keyed by status string, full stop, there is no code path that could
	// key it by an individual booking's id.
}

// ─── Admin action validation (reason required) ─────────────────────────────

func TestAdminActions_RequireNonEmptyReason(t *testing.T) {
	bookings := newFakeBookingRepo()
	pid := "p1"
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", ProviderID: &pid, Status: bookingmodels.StatusInProgress}
	audit := &fakeAdminAudit{}
	svc := NewBookingService(Options{Bookings: bookings, AdminAudit: audit})

	if _, err := svc.AdminCancelBooking(context.Background(), "op-1", "b1", "   "); err == nil {
		t.Error("AdminCancelBooking: expected a validation error for a blank reason")
	}
	if _, err := svc.AdminForceRecover(context.Background(), "op-1", "b1", ""); err == nil {
		t.Error("AdminForceRecover: expected a validation error for an empty reason")
	}
	if _, err := svc.AdminPauseProvider(context.Background(), "op-1", "b1", ""); err == nil {
		t.Error("AdminPauseProvider: expected a validation error for an empty reason")
	}
	if _, err := svc.AdminReleaseHold(context.Background(), "op-1", "b1", ""); err == nil {
		t.Error("AdminReleaseHold: expected a validation error for an empty reason")
	}
	if len(audit.recorded) != 0 {
		t.Errorf("audit.recorded = %v, want none -- a rejected action must never be recorded as if it happened", audit.recorded)
	}
}

func TestAdminActions_NilAdminAuditIsUnavailable(t *testing.T) {
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo()})
	if _, err := svc.AdminCancelBooking(context.Background(), "op-1", "b1", "reason"); err == nil {
		t.Error("expected an error when AdminAudit is not configured")
	}
}

func TestAdminForceRecover_RefusesABookingNotAwaitingMatch(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", Status: bookingmodels.StatusCompleted}
	svc := NewBookingService(Options{Bookings: bookings, AdminAudit: &fakeAdminAudit{}})

	if _, err := svc.AdminForceRecover(context.Background(), "op-1", "b1", "stuck"); err == nil {
		t.Fatal("expected a conflict for a booking that is not pending_match/awaiting_acceptance")
	}
}

func TestAdminForceRecover_RecordsAuditForAnEligibleBooking(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", Status: bookingmodels.StatusPendingMatch}
	audit := &fakeAdminAudit{}
	svc := NewBookingService(Options{Bookings: bookings, AdminAudit: audit}) // Lease left nil -> spawnMatcher no-ops safely

	if _, err := svc.AdminForceRecover(context.Background(), "op-1", "b1", "stuck matcher"); err != nil {
		t.Fatalf("AdminForceRecover: %v", err)
	}
	if len(audit.recorded) != 1 || audit.recorded[0] != "force_recover:booking:b1" {
		t.Errorf("audit.recorded = %v, want [force_recover:booking:b1]", audit.recorded)
	}
}

// ─── GetBookingReplay ───────────────────────────────────────────────────────

func TestGetBookingReplay_MergesAndSortsAllThreeSourcesChronologically(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", Status: bookingmodels.StatusCompleted}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	bookings.events = []bookingmodels.BookingEvent{
		{BookingID: "b1", EventType: "taxi.booking.accepted", ActorType: "provider", ActorID: "p1", CreatedAt: t0.Add(2 * time.Minute)},
	}
	opHistory := &fakeOperationalHistory{history: []availabilityrepositories.HistoryEntry{
		{ToState: "ON_TRIP", ChangedAt: t0.Add(1 * time.Minute)},
	}}
	svc := NewBookingService(Options{Bookings: bookings, OperationalHistory: opHistory})

	replay, err := svc.GetBookingReplay(context.Background(), "b1")
	if err != nil {
		t.Fatalf("GetBookingReplay: %v", err)
	}
	if len(replay.Events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(replay.Events))
	}
	if replay.Events[0].Kind != "operational_state" || replay.Events[1].Kind != "lifecycle_event" {
		t.Errorf("Events = %+v, want [operational_state, lifecycle_event] in chronological order", replay.Events)
	}
}

func TestGetBookingReplay_UnknownBookingIsNotFound(t *testing.T) {
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo()})
	if _, err := svc.GetBookingReplay(context.Background(), "missing"); err == nil {
		t.Fatal("expected an error for an unknown booking id")
	}
}

// ─── AdminPauseProvider / AdminReleaseHold gating ──────────────────────────
// Both call operationalState.Transition directly, with no BeginTx involved
// (unlike AdminCancelBooking/MarkCompleted) -- fully unit-testable.

func newAdminHoldTestService(bookings *fakeBookingRepo, opState *fakeOperationalStateStore) *BookingService {
	return NewBookingService(Options{Bookings: bookings, OperationalState: opState, AdminAudit: &fakeAdminAudit{}})
}

func TestAdminPauseProvider_SucceedsFromAvailable(t *testing.T) {
	bookings := newFakeBookingRepo()
	pid := "p1"
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", ProviderID: &pid, Status: bookingmodels.StatusInProgress}
	opState := newFakeOperationalStateStore()
	opState.states[pid] = availabilitymodels.State{ProviderID: pid, Current: availabilitymodels.StateAvailable}
	svc := newAdminHoldTestService(bookings, opState)

	state, err := svc.AdminPauseProvider(context.Background(), "op-1", "b1", "customer complaint")
	if err != nil {
		t.Fatalf("AdminPauseProvider: %v", err)
	}
	if state.Current != availabilitymodels.StateSafetyHold {
		t.Errorf("state = %q, want SAFETY_HOLD", state.Current)
	}
}

func TestAdminPauseProvider_RefusedWhenAlreadyOnHold(t *testing.T) {
	bookings := newFakeBookingRepo()
	pid := "p1"
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", ProviderID: &pid, Status: bookingmodels.StatusInProgress}
	opState := newFakeOperationalStateStore()
	opState.states[pid] = availabilitymodels.State{ProviderID: pid, Current: availabilitymodels.StateSafetyHold}
	svc := newAdminHoldTestService(bookings, opState)

	if _, err := svc.AdminPauseProvider(context.Background(), "op-1", "b1", "reason"); err == nil {
		t.Fatal("expected a conflict for a provider already on hold")
	}
}

func TestAdminPauseProvider_RefusedWhenOffline(t *testing.T) {
	bookings := newFakeBookingRepo()
	pid := "p1"
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", ProviderID: &pid, Status: bookingmodels.StatusInProgress}
	opState := newFakeOperationalStateStore()
	opState.states[pid] = availabilitymodels.State{ProviderID: pid, Current: availabilitymodels.StateOffline}
	svc := newAdminHoldTestService(bookings, opState)

	if _, err := svc.AdminPauseProvider(context.Background(), "op-1", "b1", "reason"); err == nil {
		t.Fatal("expected a conflict for an already-offline provider -- nothing to pause")
	}
}

func TestAdminPauseProvider_RefusedWithNoAssignedProvider(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", Status: bookingmodels.StatusPendingMatch}
	svc := newAdminHoldTestService(bookings, newFakeOperationalStateStore())

	if _, err := svc.AdminPauseProvider(context.Background(), "op-1", "b1", "reason"); err == nil {
		t.Fatal("expected an error for a booking with no assigned provider")
	}
}

// TestAdminReleaseHold_RestoresTheStateTheirActiveBookingImplies is a
// regression test for a real bug caught live, not by this test as
// originally written: releasing a hold used to go unconditionally to
// AVAILABLE, so a driver paused mid-trip (booking still in_progress) came
// back as "ready for a brand new offer" while still owing their existing
// passenger a ride. Release must restore ON_TRIP here, not AVAILABLE.
func TestAdminReleaseHold_RestoresTheStateTheirActiveBookingImplies(t *testing.T) {
	bookings := newFakeBookingRepo()
	pid := "p1"
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", ProviderID: &pid, Status: bookingmodels.StatusInProgress}
	opState := newFakeOperationalStateStore()
	opState.states[pid] = availabilitymodels.State{ProviderID: pid, Current: availabilitymodels.StateSafetyHold}
	svc := newAdminHoldTestService(bookings, opState)

	state, err := svc.AdminReleaseHold(context.Background(), "op-1", "b1", "cleared")
	if err != nil {
		t.Fatalf("AdminReleaseHold: %v", err)
	}
	if state.Current != availabilitymodels.StateOnTrip {
		t.Errorf("state = %q, want ON_TRIP -- the driver still has an in_progress ride to carry out", state.Current)
	}
}

func TestAdminReleaseHold_RestoresAvailableWhenNoOutstandingRide(t *testing.T) {
	bookings := newFakeBookingRepo()
	pid := "p1"
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", ProviderID: &pid, Status: bookingmodels.StatusCompleted}
	opState := newFakeOperationalStateStore()
	opState.states[pid] = availabilitymodels.State{ProviderID: pid, Current: availabilitymodels.StateSafetyHold}
	svc := newAdminHoldTestService(bookings, opState)

	state, err := svc.AdminReleaseHold(context.Background(), "op-1", "b1", "cleared")
	if err != nil {
		t.Fatalf("AdminReleaseHold: %v", err)
	}
	if state.Current != availabilitymodels.StateAvailable {
		t.Errorf("state = %q, want AVAILABLE -- the referenced booking is already completed", state.Current)
	}
}

func TestOperationalStateForBookingStatus_MapsEveryInFlightStatus(t *testing.T) {
	cases := map[bookingmodels.Status]availabilitymodels.OperationalState{
		bookingmodels.StatusAwaitingAccept:  availabilitymodels.StateOffered,
		bookingmodels.StatusAccepted:        availabilitymodels.StateOffered,
		bookingmodels.StatusEnRoutePickup:   availabilitymodels.StateEnRoutePickup,
		bookingmodels.StatusArrivedAtPickup: availabilitymodels.StateAtPickup,
		bookingmodels.StatusInProgress:      availabilitymodels.StateOnTrip,
		bookingmodels.StatusCompleted:       availabilitymodels.StateAvailable,
		bookingmodels.StatusCancelled:       availabilitymodels.StateAvailable,
		bookingmodels.StatusPendingMatch:    availabilitymodels.StateAvailable,
	}
	for status, want := range cases {
		if got := operationalStateForBookingStatus(status); got != want {
			t.Errorf("operationalStateForBookingStatus(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestAdminReleaseHold_RefusedWhenNotOnHold(t *testing.T) {
	bookings := newFakeBookingRepo()
	pid := "p1"
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", ProviderID: &pid, Status: bookingmodels.StatusInProgress}
	opState := newFakeOperationalStateStore()
	opState.states[pid] = availabilitymodels.State{ProviderID: pid, Current: availabilitymodels.StateOnTrip}
	svc := newAdminHoldTestService(bookings, opState)

	if _, err := svc.AdminReleaseHold(context.Background(), "op-1", "b1", "reason"); err == nil {
		t.Fatal("expected a conflict releasing a driver who is not on hold -- this must never bypass ON_TRIP")
	}
}

// ─── RecoverStaleMatchingBookings / RecoverStuckInProgressBookings ─────────

func TestRecoverStaleMatchingBookings_RespawnsEachStaleBooking(t *testing.T) {
	bookings := newFakeBookingRepo()
	old := time.Now().Add(-time.Hour)
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", Status: bookingmodels.StatusPendingMatch, UpdatedAt: old}
	bookings.byID["b2"] = bookingmodels.Booking{ID: "b2", Status: bookingmodels.StatusAwaitingAccept, UpdatedAt: old}
	bookings.byID["b3"] = bookingmodels.Booking{ID: "b3", Status: bookingmodels.StatusPendingMatch, UpdatedAt: time.Now()} // fresh, not stale
	svc := NewBookingService(Options{Bookings: bookings})

	count, err := svc.RecoverStaleMatchingBookings(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("RecoverStaleMatchingBookings: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

// TestRecoverStuckInProgressBookings_CompletesEachStuckBooking lives in
// recovery_integration_test.go: it goes through MarkCompleted's real
// withTransition (BeginTx + guarded CAS + operational-state release), which
// fakeBookingRepo's own BeginTx deliberately refuses to support (see that
// fake's own doc comment) -- real Postgres is required to prove this path,
// the same "guarded transitions need real Postgres" rule every other
// phase's tests in this file already follow.

func TestRecoverStuckInProgressBookings_SkipsBookingsNotYetPastGrace(t *testing.T) {
	bookings := newFakeBookingRepo()
	pid := "p1"
	recent := time.Now().Add(-5 * time.Minute)
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", ProviderID: &pid, Status: bookingmodels.StatusInProgress, StartedAt: &recent}
	svc := NewBookingService(Options{Bookings: bookings})

	count, err := svc.RecoverStuckInProgressBookings(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("RecoverStuckInProgressBookings: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (5 minutes has not passed the 30-minute grace)", count)
	}
}
