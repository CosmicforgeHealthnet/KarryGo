package availability

// Phase 5J — Trip event subscriber tests.
// Covers trip.started (set busy), trip.completed (return online + increment),
// trip.cancelled (return online, no increment), and bad-payload safety.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ── trip.started ─────────────────────────────────────────────────────────────

func TestTripStartedSetsBusyAndRemovesFromGeo(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	seedEligibleProvider(repo, providerID)
	// Provider is online with a location.
	live.statuses[providerID] = StatusOnline
	live.locations[providerID] = Location{ProviderID: providerID, Lat: 6, Lng: 3, UpdatedAt: time.Now()}
	live.discoverable[providerID] = true
	service := NewService(repo, live)

	payload := mustTripJSON(t, TripStartedEvent{
		Event:      TopicTripStarted,
		TripID:     uuid.NewString(),
		ProviderID: providerID,
		OccurredAt: time.Now().UTC(),
	})
	if err := HandleTripStartedPayload(ctx, service, payload); err != nil {
		t.Fatalf("HandleTripStartedPayload error = %v", err)
	}

	if live.statuses[providerID] != StatusBusy {
		t.Fatalf("live status = %s, want busy", live.statuses[providerID])
	}
	if live.discoverable[providerID] {
		t.Fatal("provider should have been removed from GEO on trip.started")
	}
	if repo.availability[providerID].Status != StatusBusy {
		t.Fatalf("DB status = %s, want busy", repo.availability[providerID].Status)
	}
}

func TestTripStartedBadPayloadDropsSafely(t *testing.T) {
	service := NewService(newFakeAvailabilityRepository(), newFakeLiveStore())
	if err := HandleTripStartedPayload(context.Background(), service, []byte("{bad json")); err != nil {
		t.Fatalf("bad payload returned error: %v", err)
	}
}

func TestTripStartedInvalidProviderIDDropsSafely(t *testing.T) {
	service := NewService(newFakeAvailabilityRepository(), newFakeLiveStore())
	payload := mustTripJSON(t, TripStartedEvent{
		Event:      TopicTripStarted,
		TripID:     uuid.NewString(),
		ProviderID: "not-a-uuid",
		OccurredAt: time.Now().UTC(),
	})
	if err := HandleTripStartedPayload(context.Background(), service, payload); err != nil {
		t.Fatalf("invalid provider_id returned error: %v", err)
	}
}

// ── trip.completed ────────────────────────────────────────────────────────────

func TestTripCompletedReturnsOnlineAndIncrementsTrips(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	seedEligibleProvider(repo, providerID)
	// Provider was busy during trip.
	live.statuses[providerID] = StatusBusy
	live.locations[providerID] = Location{ProviderID: providerID, Lat: 6, Lng: 3, UpdatedAt: time.Now()}
	live.discoverable[providerID] = false

	// Create an open session to increment.
	now := time.Now().UTC()
	repo.openSessions[providerID] = AvailabilitySession{
		ID:           uuid.NewString(),
		ProviderID:   providerID,
		WentOnlineAt: now.Add(-30 * time.Minute),
		CreatedAt:    now.Add(-30 * time.Minute),
	}
	service := NewService(repo, live, WithClock(func() time.Time { return now }))

	payload := mustTripJSON(t, TripCompletedEvent{
		Event:      TopicTripCompleted,
		TripID:     uuid.NewString(),
		ProviderID: providerID,
		OccurredAt: now,
	})
	if err := HandleTripCompletedPayload(ctx, service, payload); err != nil {
		t.Fatalf("HandleTripCompletedPayload error = %v", err)
	}

	if live.statuses[providerID] != StatusOnline {
		t.Fatalf("live status = %s, want online after trip.completed", live.statuses[providerID])
	}
	if !live.discoverable[providerID] {
		t.Fatal("provider should be restored to GEO after trip.completed (location exists)")
	}
	if repo.openSessions[providerID].TripsInSession != 1 {
		t.Fatalf("trips_in_session = %d, want 1", repo.openSessions[providerID].TripsInSession)
	}
}

func TestTripCompletedBadPayloadDropsSafely(t *testing.T) {
	service := NewService(newFakeAvailabilityRepository(), newFakeLiveStore())
	if err := HandleTripCompletedPayload(context.Background(), service, []byte("{bad}")); err != nil {
		t.Fatalf("bad payload returned error: %v", err)
	}
}

// ── trip.cancelled ────────────────────────────────────────────────────────────

func TestTripCancelledReturnsOnlineDoesNotIncrementTrips(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	seedEligibleProvider(repo, providerID)
	live.statuses[providerID] = StatusBusy
	live.locations[providerID] = Location{ProviderID: providerID, Lat: 6, Lng: 3, UpdatedAt: time.Now()}

	now := time.Now().UTC()
	repo.openSessions[providerID] = AvailabilitySession{
		ID:           uuid.NewString(),
		ProviderID:   providerID,
		WentOnlineAt: now.Add(-15 * time.Minute),
		CreatedAt:    now.Add(-15 * time.Minute),
	}
	service := NewService(repo, live, WithClock(func() time.Time { return now }))

	payload := mustTripJSON(t, TripCancelledEvent{
		Event:      TopicTripCancelled,
		TripID:     uuid.NewString(),
		ProviderID: providerID,
		OccurredAt: now,
	})
	if err := HandleTripCancelledPayload(ctx, service, payload); err != nil {
		t.Fatalf("HandleTripCancelledPayload error = %v", err)
	}

	if live.statuses[providerID] != StatusOnline {
		t.Fatalf("live status = %s, want online after trip.cancelled", live.statuses[providerID])
	}
	if repo.openSessions[providerID].TripsInSession != 0 {
		t.Fatalf("trips_in_session = %d, want 0 (cancelled trip must not count)", repo.openSessions[providerID].TripsInSession)
	}
}

func TestTripCancelledBadPayloadDropsSafely(t *testing.T) {
	service := NewService(newFakeAvailabilityRepository(), newFakeLiveStore())
	if err := HandleTripCancelledPayload(context.Background(), service, []byte(`{"provider_id":""}`)); err != nil {
		t.Fatalf("bad payload returned error: %v", err)
	}
}

// ── provider.location_updated event published on GPS ping ─────────────────────

func TestLocationUpdatedEventPublishedOnAcceptedPing(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	live.statuses[providerID] = StatusOnline
	events := &fakeEventPublisher{}
	service := NewService(repo, live, WithEventPublisher(events))

	_, err := service.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 6, Lng: 3})
	if err != nil {
		t.Fatalf("UpdateLocation error = %v", err)
	}
	if events.locationCount != 1 {
		t.Fatalf("location events = %d, want 1", events.locationCount)
	}
}

func TestLocationUpdatedEventNotPublishedWhenOffline(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	// No status key → treated as offline.
	events := &fakeEventPublisher{}
	service := NewService(repo, live, WithEventPublisher(events))

	_, _ = service.UpdateLocation(ctx, providerID, UpdateLocationRequest{Lat: 6, Lng: 3})
	if events.locationCount != 0 {
		t.Fatalf("location events = %d, want 0 (provider offline)", events.locationCount)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mustTripJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal trip payload: %v", err)
	}
	return b
}
