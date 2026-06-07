package availability

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"karrygo/services/driver-dispatch-delivery-service/internal/features/vehicle"
	"karrygo/services/driver-dispatch-delivery-service/internal/features/verification"
)

func TestVerificationFullyApprovedUnlocksAvailabilityWithoutAutoOnline(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	repo.providers[providerID] = ProviderGateState{
		ProviderID:         providerID,
		IsActive:           true,
		VerificationStatus: providerVerifiedStatus,
	}
	service := NewService(repo, newFakeLiveStore())

	payload := mustJSON(t, verification.VerificationFullyApprovedEvent{
		Event:      verification.TopicVerificationFullyApproved,
		ProviderID: providerID,
		ApprovedAt: time.Now().UTC(),
		CreatedAt:  time.Now().UTC(),
	})
	if err := HandleVerificationFullyApprovedPayload(ctx, service, payload); err != nil {
		t.Fatalf("HandleVerificationFullyApprovedPayload error = %v", err)
	}
	availability := repo.availability[providerID]
	if !availability.VerifiedToGoOnline {
		t.Fatal("verified_to_go_online was not set")
	}
	if availability.Status != StatusOffline {
		t.Fatalf("status = %s, want offline", availability.Status)
	}
}

func TestProviderVerificationSuspendedForcesOfflineAndClearsEligibility(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	seedEligibleProvider(repo, providerID)
	service := NewService(repo, live, WithClock(fixedClock("2026-06-06T11:00:00Z")))
	if _, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOnline}); err != nil {
		t.Fatalf("online error = %v", err)
	}

	payload := mustJSON(t, vehicle.ProviderVerificationSuspendedEvent{
		Event:      vehicle.TopicProviderVerificationSuspended,
		ProviderID: providerID,
		Reason:     "admin suspension",
		CreatedAt:  time.Now().UTC(),
	})
	if err := HandleProviderVerificationSuspendedPayload(ctx, service, payload); err != nil {
		t.Fatalf("HandleProviderVerificationSuspendedPayload error = %v", err)
	}
	availability := repo.availability[providerID]
	if availability.VerifiedToGoOnline {
		t.Fatal("verified_to_go_online was not cleared")
	}
	if availability.Status != StatusOffline {
		t.Fatalf("status = %s, want offline", availability.Status)
	}
	if live.statuses[providerID] != StatusOffline {
		t.Fatalf("live status = %s, want offline", live.statuses[providerID])
	}
	if !repo.lastForcedOffline {
		t.Fatal("session was not marked forced_offline")
	}
}

func TestVehicleRejectedForcesOfflineOnlyWhenNoVerifiedActiveBikeRemains(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()
	live := newFakeLiveStore()
	seedEligibleProvider(repo, providerID)
	service := NewService(repo, live, WithClock(fixedClock("2026-06-06T11:00:00Z")))
	if _, err := service.SetStatus(ctx, providerID, SetAvailabilityRequest{Status: StatusOnline}); err != nil {
		t.Fatalf("online error = %v", err)
	}

	repo.hasBike[providerID] = true
	payload := mustJSON(t, vehicle.VehicleRejectedEvent{
		Event:      vehicle.TopicVehicleRejected,
		ProviderID: providerID,
		BikeID:     uuid.NewString(),
		Reason:     "documents invalid",
		CreatedAt:  time.Now().UTC(),
	})
	if err := HandleVehicleRejectedPayload(ctx, service, payload); err != nil {
		t.Fatalf("HandleVehicleRejectedPayload with bike error = %v", err)
	}
	if repo.availability[providerID].Status != StatusOnline {
		t.Fatalf("status = %s, want online while another verified bike remains", repo.availability[providerID].Status)
	}

	repo.hasBike[providerID] = false
	if err := HandleVehicleRejectedPayload(ctx, service, payload); err != nil {
		t.Fatalf("HandleVehicleRejectedPayload no bike error = %v", err)
	}
	if repo.availability[providerID].Status != StatusOffline {
		t.Fatalf("status = %s, want offline", repo.availability[providerID].Status)
	}
	if live.discoverable[providerID] {
		t.Fatal("provider remained discoverable")
	}
}

func TestVehicleVerifiedDoesNotAutoOnline(t *testing.T) {
	ctx := context.Background()
	providerID := uuid.NewString()
	repo := newFakeAvailabilityRepository()

	payload := mustJSON(t, vehicle.VehicleVerifiedEvent{
		Event:      vehicle.TopicVehicleVerified,
		ProviderID: providerID,
		BikeID:     uuid.NewString(),
		CreatedAt:  time.Now().UTC(),
	})
	if err := HandleVehicleVerifiedPayload(ctx, repo, payload); err != nil {
		t.Fatalf("HandleVehicleVerifiedPayload error = %v", err)
	}
	if repo.availability[providerID].Status != StatusOffline {
		t.Fatalf("status = %s, want offline", repo.availability[providerID].Status)
	}
}

func TestSubscriberBadPayloadsAreDropped(t *testing.T) {
	ctx := context.Background()
	repo := newFakeAvailabilityRepository()
	service := NewService(repo, newFakeLiveStore())

	if err := HandleVerificationFullyApprovedPayload(ctx, service, []byte("{bad json")); err != nil {
		t.Fatalf("bad fully approved payload returned error: %v", err)
	}
	if err := HandleProviderVerificationSuspendedPayload(ctx, service, []byte(`{"provider_id":""}`)); err != nil {
		t.Fatalf("empty suspended provider_id returned error: %v", err)
	}
	if err := HandleVehicleSuspendedPayload(ctx, service, []byte(`{"provider_id":"not-a-uuid"}`)); err != nil {
		t.Fatalf("invalid vehicle suspended provider_id returned error: %v", err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}
