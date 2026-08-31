package availability

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestRedisLiveStoreStatusTTLAndGeoRestore(t *testing.T) {
	ctx := context.Background()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer client.Close()

	store := NewRedisLiveStore(client)
	providerID := uuid.NewString()

	if err := store.SetStatus(ctx, providerID, StatusOnline); err != nil {
		t.Fatalf("SetStatus error = %v", err)
	}
	ttl, err := client.TTL(ctx, ProviderStatusKey(providerID)).Result()
	if err != nil {
		t.Fatalf("TTL error = %v", err)
	}
	if ttl != StatusTTL {
		t.Fatalf("ttl = %v, want %v", ttl, StatusTTL)
	}

	restored, err := store.RestoreGeoFromLocation(ctx, providerID)
	if err != nil {
		t.Fatalf("RestoreGeoFromLocation without location error = %v", err)
	}
	if restored {
		t.Fatal("restored GEO without last location")
	}

	// A live timestamp, not a fixed date: the spatial index scores entries by
	// fix time and prunes anything past LocationTTL, so a rider whose last
	// fix is old is correctly not discoverable.
	location := Location{
		ProviderID: providerID,
		Lat:        6.5244,
		Lng:        3.3792,
		UpdatedAt:  time.Now().UTC(),
	}
	if err := store.SetLocation(ctx, providerID, location, false); err != nil {
		t.Fatalf("SetLocation error = %v", err)
	}
	restored, err = store.RestoreGeoFromLocation(ctx, providerID)
	if err != nil {
		t.Fatalf("RestoreGeoFromLocation error = %v", err)
	}
	if !restored {
		t.Fatal("did not restore GEO from last location")
	}
	// Assert on discoverability rather than on the index's internal keys:
	// the spatial index is a geohash cell layout now, not a single GEO set.
	if !isDiscoverable(ctx, t, store, providerID, location) {
		t.Fatal("provider not discoverable after restoring from the last location")
	}
}

// isDiscoverable reports whether a proximity query at the provider's own
// position returns them.
func isDiscoverable(ctx context.Context, t *testing.T, store *RedisLiveStore, providerID string, at Location) bool {
	t.Helper()
	nearby, err := store.GetNearby(ctx, NearbyProvidersRequest{
		Latitude:  at.Lat,
		Longitude: at.Lng,
		RadiusKM:  1,
		Limit:     50,
	})
	if err != nil {
		t.Fatalf("GetNearby error = %v", err)
	}
	for _, provider := range nearby {
		if provider.ProviderID == providerID {
			return true
		}
	}
	return false
}

func TestRedisLiveStoreClearProviderSetsOfflineAndClearsLocation(t *testing.T) {
	ctx := context.Background()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer client.Close()

	store := NewRedisLiveStore(client)
	providerID := uuid.NewString()
	location := Location{ProviderID: providerID, Lat: 6.5244, Lng: 3.3792, UpdatedAt: time.Now().UTC()}
	if err := store.SetStatus(ctx, providerID, StatusOnline); err != nil {
		t.Fatalf("SetStatus error = %v", err)
	}
	if err := store.SetLocation(ctx, providerID, location, true); err != nil {
		t.Fatalf("SetLocation error = %v", err)
	}

	if err := store.ClearProvider(ctx, providerID); err != nil {
		t.Fatalf("ClearProvider error = %v", err)
	}
	status, ok, err := store.GetStatus(ctx, providerID)
	if err != nil {
		t.Fatalf("GetStatus error = %v", err)
	}
	if !ok || status != StatusOffline {
		t.Fatalf("status = %s ok=%v, want offline true", status, ok)
	}
	if _, ok, err := store.GetLocation(ctx, providerID); err != nil || ok {
		t.Fatalf("location ok=%v err=%v, want false nil", ok, err)
	}
	positions, err := client.GeoPos(ctx, OnlineProvidersGeoKey, providerID).Result()
	if err != nil {
		t.Fatalf("GeoPos error = %v", err)
	}
	if len(positions) != 1 || positions[0] != nil {
		t.Fatalf("provider remained in GEO: %#v", positions)
	}
}
