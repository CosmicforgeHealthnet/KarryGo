package availability

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func newIndexTestStore(t *testing.T) (*RedisLiveStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisLiveStore(client), mr
}

func onlineAt(t *testing.T, store *RedisLiveStore, lat, lng float64) string {
	t.Helper()
	ctx := context.Background()
	providerID := uuid.NewString()
	if err := store.SetStatus(ctx, providerID, StatusOnline); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	loc := Location{ProviderID: providerID, Lat: lat, Lng: lng, UpdatedAt: time.Now().UTC()}
	if err := store.SetLocation(ctx, providerID, loc, true); err != nil {
		t.Fatalf("SetLocation: %v", err)
	}
	return providerID
}

func nearbyIDs(t *testing.T, store *RedisLiveStore, lat, lng, radiusKM float64) []string {
	t.Helper()
	found, err := store.GetNearby(context.Background(), NearbyProvidersRequest{
		Latitude: lat, Longitude: lng, RadiusKM: radiusKM, Limit: 50,
	})
	if err != nil {
		t.Fatalf("GetNearby: %v", err)
	}
	ids := make([]string, len(found))
	for i, provider := range found {
		ids[i] = provider.ProviderID
	}
	return ids
}

// The defect this index replaces: avail:geo:online had no TTL and no sweeper,
// so a rider who force-quit stayed matchable indefinitely. The cell index
// scores entries by fix time and prunes them on read, so a rider whose fix has
// aged past LocationTTL must disappear on the very next query.
func TestNearbyDropsRidersWhoseFixHasExpired(t *testing.T) {
	store, mr := newIndexTestStore(t)
	providerID := onlineAt(t, store, 6.5244, 3.3792)

	if got := nearbyIDs(t, store, 6.5244, 3.3792, 5); len(got) != 1 || got[0] != providerID {
		t.Fatalf("rider not discoverable right after a ping: %v", got)
	}

	// Advance past the location TTL without the rider reporting again.
	mr.FastForward(LocationTTL + time.Second)

	if got := nearbyIDs(t, store, 6.5244, 3.3792, 5); len(got) != 0 {
		t.Fatalf("a rider whose last fix expired is still matchable: %v", got)
	}
}

// Proximity must be a true radius filter, not just cell membership: the cell
// cover is deliberately a superset of the circle.
func TestNearbyFiltersByTrueDistance(t *testing.T) {
	store, _ := newIndexTestStore(t)

	const lat, lng = 6.5244, 3.3792
	near := onlineAt(t, store, lat, lng)
	// Roughly 20 km away.
	far := onlineAt(t, store, 6.4281, 3.4219)

	tight := nearbyIDs(t, store, lat, lng, 5)
	if len(tight) != 1 || tight[0] != near {
		t.Fatalf("5 km query returned %v, want only the nearby rider", tight)
	}

	wide := nearbyIDs(t, store, lat, lng, 30)
	if len(wide) != 2 {
		t.Fatalf("30 km query returned %v, want both riders", wide)
	}
	if wide[0] != near || wide[1] != far {
		t.Fatalf("results %v are not ordered nearest first", wide)
	}
}

// A rider on a job keeps a readable position for the customer's live map but
// must not be offered new work.
func TestBusyRiderKeepsPositionButLeavesTheIndex(t *testing.T) {
	store, _ := newIndexTestStore(t)
	ctx := context.Background()

	const lat, lng = 6.5244, 3.3792
	providerID := onlineAt(t, store, lat, lng)

	if err := store.RemoveFromGeo(ctx, providerID); err != nil {
		t.Fatalf("RemoveFromGeo: %v", err)
	}

	if got := nearbyIDs(t, store, lat, lng, 5); len(got) != 0 {
		t.Fatalf("a rider taken out of matching is still discoverable: %v", got)
	}
	location, ok, err := store.GetLocation(ctx, providerID)
	if err != nil || !ok {
		t.Fatalf("GetLocation = (ok %v, err %v), want the position still readable", ok, err)
	}
	if location.Lat != lat || location.Lng != lng {
		t.Fatalf("position changed when leaving the index: %+v", location)
	}

	// And they come back when the job ends.
	restored, err := store.RestoreGeoFromLocation(ctx, providerID)
	if err != nil || !restored {
		t.Fatalf("RestoreGeoFromLocation = (%v, %v), want a restore", restored, err)
	}
	if got := nearbyIDs(t, store, lat, lng, 5); len(got) != 1 {
		t.Fatalf("rider not rediscoverable after returning from a trip: %v", got)
	}
}

// A rider who is indexed but whose status key has lapsed or flipped to busy
// must not be offered work, and must be dropped from the index as a side
// effect so the next query does not re-consider them.
func TestNearbySkipsRidersWhoAreNoLongerOnline(t *testing.T) {
	store, _ := newIndexTestStore(t)
	ctx := context.Background()

	const lat, lng = 6.5244, 3.3792
	providerID := onlineAt(t, store, lat, lng)

	if err := store.SetStatus(ctx, providerID, StatusBusy); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	if got := nearbyIDs(t, store, lat, lng, 5); len(got) != 0 {
		t.Fatalf("a busy rider was returned as available: %v", got)
	}
}

// Moving must not leave the rider discoverable at their old position.
func TestMovingRiderIsOnlyDiscoverableAtTheNewPosition(t *testing.T) {
	store, _ := newIndexTestStore(t)
	ctx := context.Background()

	const oldLat, oldLng = 6.5244, 3.3792
	const newLat, newLng = 6.4281, 3.4219

	providerID := onlineAt(t, store, oldLat, oldLng)
	moved := Location{ProviderID: providerID, Lat: newLat, Lng: newLng, UpdatedAt: time.Now().UTC()}
	if err := store.SetLocation(ctx, providerID, moved, true); err != nil {
		t.Fatalf("SetLocation: %v", err)
	}

	if got := nearbyIDs(t, store, oldLat, oldLng, 2); len(got) != 0 {
		t.Fatalf("rider still discoverable at their old position: %v", got)
	}
	if got := nearbyIDs(t, store, newLat, newLng, 2); len(got) != 1 {
		t.Fatalf("rider not discoverable at their new position: %v", got)
	}
}
