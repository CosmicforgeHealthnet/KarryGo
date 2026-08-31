package availabilityusecases

import (
	"context"
	"testing"
	"time"

	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/geo"
)

// recordingStore captures what the availability service handed the store, so
// the tests can prove telemetry survives the usecase layer.
type recordingStore struct {
	lastHeartbeat availabilityrepositories.Telemetry
	heartbeats    int
	online        []availabilityrepositories.ProviderStatus
}

func (s *recordingStore) SetOnline(_ context.Context, status availabilityrepositories.ProviderStatus, _ time.Duration) error {
	s.online = append(s.online, status)
	return nil
}
func (s *recordingStore) SetOffline(context.Context, string) error { return nil }
func (s *recordingStore) Heartbeat(_ context.Context, _ string, fix availabilityrepositories.Telemetry, _ time.Duration) error {
	s.lastHeartbeat = fix
	s.heartbeats++
	return nil
}
func (s *recordingStore) SetDiscoverable(context.Context, string, bool) error { return nil }
func (s *recordingStore) CountOnline(context.Context) (int64, error) {
	return int64(len(s.online)), nil
}
func (s *recordingStore) GetOnlineProviders(context.Context) ([]availabilityrepositories.ProviderStatus, error) {
	return s.online, nil
}
func (s *recordingStore) NearbyProviders(context.Context, geo.Point, float64, int) ([]availabilityrepositories.ProviderStatus, error) {
	return s.online, nil
}
func (s *recordingStore) GetProviderStatus(context.Context, string) (availabilityrepositories.ProviderStatus, bool, error) {
	return availabilityrepositories.ProviderStatus{}, false, nil
}
func (s *recordingStore) AcquireMatchLock(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}
func (s *recordingStore) ReleaseMatchLock(context.Context, string) error { return nil }
func (s *recordingStore) MarkRejected(context.Context, string, string, time.Duration) error {
	return nil
}
func (s *recordingStore) GetRejectedProviders(context.Context, string) (map[string]bool, error) {
	return nil, nil
}

type noTrucks struct{}

func (noTrucks) CountActiveByProvider(context.Context, string) (int, error) { return 1, nil }

// Heading and speed are what let the customer's map rotate the truck and
// dead-reckon between fixes. The truck stack dropped them end to end before
// this change, so pin that they now reach the store.
func TestHeartbeatCarriesTelemetry(t *testing.T) {
	store := &recordingStore{}
	svc := &AvailabilityService{store: store, onlineTTL: 90 * time.Second}

	err := svc.Heartbeat(context.Background(), HeartbeatInput{
		ProviderID: "p1",
		Lat:        6.6018,
		Lng:        3.3515,
		Heading:    271.5,
		Speed:      12.25,
		Accuracy:   4,
	})
	if err != nil {
		t.Fatalf("Heartbeat returned error: %v", err)
	}

	got := store.lastHeartbeat
	want := availabilityrepositories.Telemetry{Lat: 6.6018, Lng: 3.3515, Heading: 271.5, Speed: 12.25, Accuracy: 4}
	if got != want {
		t.Fatalf("store received %+v, want %+v", got, want)
	}
}

// An older app build sends only lat/lng. It must still keep the provider
// online rather than being rejected for missing telemetry.
func TestHeartbeatWithoutTelemetryStillSucceeds(t *testing.T) {
	store := &recordingStore{}
	svc := &AvailabilityService{store: store, onlineTTL: 90 * time.Second}

	if err := svc.Heartbeat(context.Background(), HeartbeatInput{ProviderID: "p1", Lat: 6.6, Lng: 3.35}); err != nil {
		t.Fatalf("Heartbeat returned error: %v", err)
	}
	if store.heartbeats != 1 {
		t.Fatalf("heartbeats = %d, want 1", store.heartbeats)
	}
	if store.lastHeartbeat.Heading != 0 || store.lastHeartbeat.Speed != 0 {
		t.Fatalf("absent telemetry was not left zeroed: %+v", store.lastHeartbeat)
	}
}

// The reporting cadence is server-driven so it can be tuned without shipping
// an app build. A vehicle marker cannot be interpolated smoothly from the idle
// cadence alone.
func TestHeartbeatIntervalIsTighterOnTrip(t *testing.T) {
	svc := &AvailabilityService{}

	idle := svc.HeartbeatIntervalSeconds(false)
	onTrip := svc.HeartbeatIntervalSeconds(true)

	if idle != IdleHeartbeatSeconds {
		t.Fatalf("idle interval = %d, want %d", idle, IdleHeartbeatSeconds)
	}
	if onTrip != OnTripHeartbeatSeconds {
		t.Fatalf("on-trip interval = %d, want %d", onTrip, OnTripHeartbeatSeconds)
	}
	if onTrip >= idle {
		t.Fatalf("on-trip cadence %ds is not tighter than idle %ds", onTrip, idle)
	}
	if onTrip > 5 {
		t.Fatalf("on-trip cadence %ds is too slow to interpolate a moving vehicle from", onTrip)
	}
}
