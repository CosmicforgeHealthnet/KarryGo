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

	location := Location{
		ProviderID: providerID,
		Lat:        6.5244,
		Lng:        3.3792,
		UpdatedAt:  time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC),
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
	positions, err := client.GeoPos(ctx, OnlineProvidersGeoKey, providerID).Result()
	if err != nil {
		t.Fatalf("GeoPos error = %v", err)
	}
	if len(positions) != 1 || positions[0] == nil {
		t.Fatalf("provider not in GEO: %#v", positions)
	}
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
