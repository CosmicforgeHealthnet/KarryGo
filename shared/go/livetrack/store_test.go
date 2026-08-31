package livetrack

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/geo"
)

// Both implementations must satisfy the interface services depend on.
func TestStoreImplementations(t *testing.T) {
	var _ Store = (*RedisStore)(nil)
	var _ Store = NoopStore{}
}

const testNamespace = "hauling"

var (
	ikeja       = geo.Point{Lat: 6.6018, Lng: 3.3515}
	victoriaIsl = geo.Point{Lat: 6.4281, Lng: 3.4219}
)

func newTestStore(t *testing.T) (*RedisStore, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisStore(client, Config{Namespace: testNamespace}), mr, client
}

func fixAt(entityID string, p geo.Point) Fix {
	now := time.Now().UTC()
	return Fix{
		EntityID:   entityID,
		Lat:        p.Lat,
		Lng:        p.Lng,
		Heading:    90,
		Speed:      8.5,
		Accuracy:   5,
		RecordedAt: now,
		ReceivedAt: now,
		Source:     "driver-hauling-service",
	}
}

func TestPutFixAndGetFixRoundTrip(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	want := fixAt("provider-1", ikeja)
	if err := store.PutFix(ctx, want, true); err != nil {
		t.Fatalf("PutFix returned error: %v", err)
	}

	got, ok, err := store.GetFix(ctx, "provider-1")
	if err != nil {
		t.Fatalf("GetFix returned error: %v", err)
	}
	if !ok {
		t.Fatal("GetFix reported no fix for a provider that just reported one")
	}
	// Heading and speed are the fields the truck stack drops today; assert
	// they survive the round trip.
	if got.Lat != want.Lat || got.Lng != want.Lng || got.Heading != want.Heading ||
		got.Speed != want.Speed || got.Accuracy != want.Accuracy || got.Source != want.Source {
		t.Fatalf("GetFix = %+v, want %+v", got, want)
	}

	if _, ok, err := store.GetFix(ctx, "nobody"); err != nil || ok {
		t.Fatalf("GetFix for an unknown entity = (ok %v, err %v), want (false, nil)", ok, err)
	}
}

func TestPutFixRejectsBadInput(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.PutFix(ctx, Fix{Lat: 6, Lng: 3}, true); err == nil {
		t.Fatal("PutFix without an entity id should error")
	}
	if err := store.PutFix(ctx, Fix{EntityID: "x", Lat: 991, Lng: 3}, true); err == nil {
		t.Fatal("PutFix with an out-of-range coordinate should error")
	}
}

func TestPutFixDefaultsTimestamps(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	// A caller that only has coordinates must still produce an indexable,
	// non-stale fix rather than one dated to the zero time.
	if err := store.PutFix(ctx, Fix{EntityID: "p", Lat: ikeja.Lat, Lng: ikeja.Lng}, true); err != nil {
		t.Fatalf("PutFix returned error: %v", err)
	}
	got, ok, err := store.GetFix(ctx, "p")
	if err != nil || !ok {
		t.Fatalf("GetFix = (ok %v, err %v)", ok, err)
	}
	if got.RecordedAt.IsZero() || got.ReceivedAt.IsZero() {
		t.Fatalf("timestamps not defaulted: %+v", got)
	}

	found, err := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 5})
	if err != nil || len(found) != 1 {
		t.Fatalf("Nearby = %d results (err %v), want 1", len(found), err)
	}
}

func TestNearbyFiltersByTrueDistance(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	// Two providers ~20 km apart.
	if err := store.PutFix(ctx, fixAt("near", ikeja), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}
	if err := store.PutFix(ctx, fixAt("far", victoriaIsl), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}

	cases := []struct {
		name     string
		radiusKm float64
		want     []string
	}{
		// The cell cover is always a superset of the circle, so a tight
		// radius proves the true-distance filter runs.
		{"tight radius keeps only the close one", 5, []string{"near"}},
		{"wide radius keeps both", 30, []string{"near", "far"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: tc.radiusKm})
			if err != nil {
				t.Fatalf("Nearby returned error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("Nearby returned %d results %v, want %d %v", len(got), ids(got), len(tc.want), tc.want)
			}
			for i, want := range tc.want {
				if got[i].Fix.EntityID != want {
					t.Fatalf("result %d = %q, want %q (nearest first)", i, got[i].Fix.EntityID, want)
				}
			}
		})
	}
}

// Both configured precisions must be usable, otherwise a wide sweep would
// silently return nothing once PickPrecision drops to the coarse index.
func TestNearbyWorksAtBothIndexedPrecisions(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.PutFix(ctx, fixAt("provider-1", victoriaIsl), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}

	cfg := store.Config()
	for _, radiusKm := range []float64{5, 25, 50} {
		precision := geo.PickPrecision(ikeja, radiusKm, cfg.CellPrecisions, cfg.MaxCoverCells)
		got, err := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: radiusKm})
		if err != nil {
			t.Fatalf("Nearby(%v km): %v", radiusKm, err)
		}
		// Victoria Island is ~20.8 km from Ikeja.
		wantFound := radiusKm >= 25
		if found := len(got) == 1; found != wantFound {
			t.Fatalf("Nearby(%v km) at precision %d returned %d results, want found=%v",
				radiusKm, precision, len(got), wantFound)
		}
	}
}

func TestNearbySortsNearestFirstAndHonoursLimit(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	// Three providers, progressively further east of Ikeja.
	for i, km := range []float64{3, 1, 2} {
		id := []string{"three", "one", "two"}[i]
		if err := store.PutFix(ctx, fixAt(id, geo.Destination(ikeja, 90, km)), true); err != nil {
			t.Fatalf("PutFix: %v", err)
		}
	}

	got, err := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 10})
	if err != nil {
		t.Fatalf("Nearby: %v", err)
	}
	want := []string{"one", "two", "three"}
	if len(got) != 3 {
		t.Fatalf("Nearby returned %v, want %v", ids(got), want)
	}
	for i := range want {
		if got[i].Fix.EntityID != want[i] {
			t.Fatalf("Nearby returned %v, want %v", ids(got), want)
		}
		if i > 0 && got[i].DistanceKm < got[i-1].DistanceKm {
			t.Fatalf("distances not ascending: %v", got)
		}
	}

	limited, err := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 10, Limit: 2})
	if err != nil {
		t.Fatalf("Nearby: %v", err)
	}
	if len(limited) != 2 || limited[0].Fix.EntityID != "one" {
		t.Fatalf("limited Nearby returned %v, want the two nearest", ids(limited))
	}
}

// Moving must retract the old cell entry, or a provider would be discoverable
// from two places at once until the score aged out.
func TestPutFixRetractsThePreviousCell(t *testing.T) {
	store, _, client := newTestStore(t)
	ctx := context.Background()

	if err := store.PutFix(ctx, fixAt("mover", ikeja), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}
	oldCell := geo.Encode(ikeja, store.Config().IndexPrecision())

	if err := store.PutFix(ctx, fixAt("mover", victoriaIsl), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}

	for _, precision := range store.Config().CellPrecisions {
		key := IndexKey(testNamespace, precision, oldCell.Parent(precision))
		members, err := client.ZRange(ctx, key, 0, -1).Result()
		if err != nil {
			t.Fatalf("ZRange(%s): %v", key, err)
		}
		newCell := geo.Encode(victoriaIsl, store.Config().IndexPrecision())
		if oldCell.Parent(precision) == newCell.Parent(precision) {
			continue // same cell at this precision, nothing to retract
		}
		for _, m := range members {
			if m == "mover" {
				t.Fatalf("provider still indexed in its old cell %s at precision %d", oldCell.Parent(precision), precision)
			}
		}
	}

	// And it must be findable at the new position only.
	atOld, _ := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 2})
	if len(atOld) != 0 {
		t.Fatalf("still discoverable at the old position: %v", ids(atOld))
	}
	atNew, _ := store.Nearby(ctx, NearbyQuery{Center: victoriaIsl, RadiusKm: 2})
	if len(atNew) != 1 {
		t.Fatalf("not discoverable at the new position: %v", ids(atNew))
	}
}

// A provider on a job keeps a readable position for the customer's map but
// must not be offered new work. This is the behaviour dispatch encodes today
// by removing busy riders from the GEO set.
func TestNonDiscoverableFixStaysReadableButUnmatched(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.PutFix(ctx, fixAt("busy", ikeja), false); err != nil {
		t.Fatalf("PutFix: %v", err)
	}

	if _, ok, err := store.GetFix(ctx, "busy"); err != nil || !ok {
		t.Fatalf("GetFix = (ok %v, err %v), want a readable fix", ok, err)
	}
	got, err := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 5})
	if err != nil {
		t.Fatalf("Nearby: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a non-discoverable provider was returned by Nearby: %v", ids(got))
	}
}

func TestSetDiscoverableTogglesIndexMembership(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.PutFix(ctx, fixAt("toggler", ikeja), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}

	if err := store.SetDiscoverable(ctx, "toggler", false); err != nil {
		t.Fatalf("SetDiscoverable(false): %v", err)
	}
	if got, _ := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 5}); len(got) != 0 {
		t.Fatalf("still discoverable after SetDiscoverable(false): %v", ids(got))
	}
	if _, ok, _ := store.GetFix(ctx, "toggler"); !ok {
		t.Fatal("SetDiscoverable(false) must not delete the position")
	}

	if err := store.SetDiscoverable(ctx, "toggler", true); err != nil {
		t.Fatalf("SetDiscoverable(true): %v", err)
	}
	if got, _ := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 5}); len(got) != 1 {
		t.Fatalf("not rediscoverable after SetDiscoverable(true): %v", ids(got))
	}

	// Re-enabling an entity with no stored position is a no-op, not an error.
	if err := store.SetDiscoverable(ctx, "ghost", true); err != nil {
		t.Fatalf("SetDiscoverable for an unknown entity: %v", err)
	}
}

func TestRemoveClearsPositionAndIndex(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.PutFix(ctx, fixAt("leaver", ikeja), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}
	if err := store.Remove(ctx, "leaver"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, ok, _ := store.GetFix(ctx, "leaver"); ok {
		t.Fatal("Remove left the position readable")
	}
	if got, _ := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 5}); len(got) != 0 {
		t.Fatalf("Remove left the entity discoverable: %v", ids(got))
	}
}

// The defect this index replaces: dispatch's GEO set has no TTL and no
// sweeper, so a provider who force-quits stays matchable indefinitely. Here a
// fix older than the TTL must be pruned on the very next read.
func TestNearbyPrunesFixesOlderThanTheTTL(t *testing.T) {
	store, _, client := newTestStore(t)
	ctx := context.Background()

	stale := fixAt("stale", ikeja)
	stale.RecordedAt = time.Now().UTC().Add(-10 * time.Minute) // well past DefaultFixTTL
	if err := store.PutFix(ctx, stale, true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}

	got, err := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 5})
	if err != nil {
		t.Fatalf("Nearby: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a stale fix was returned: %v", ids(got))
	}

	// And the read must have physically pruned it, not just filtered it.
	cell := geo.Encode(ikeja, store.Config().IndexPrecision())
	for _, precision := range store.Config().CellPrecisions {
		key := IndexKey(testNamespace, precision, cell.Parent(precision))
		n, err := client.ZCard(ctx, key).Result()
		if err != nil {
			t.Fatalf("ZCard(%s): %v", key, err)
		}
		if n != 0 {
			t.Fatalf("cell %s still holds %d stale members", key, n)
		}
	}
}

// An index entry whose fix key expired is a ghost. It must be evicted rather
// than returned or repeatedly re-read.
func TestNearbyEvictsIndexEntriesWithNoFix(t *testing.T) {
	store, mr, client := newTestStore(t)
	ctx := context.Background()

	if err := store.PutFix(ctx, fixAt("ghost", ikeja), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}
	// Expire only the fix key, leaving the index entry behind.
	mr.Del(FixKey(testNamespace, "ghost"))

	got, err := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 5})
	if err != nil {
		t.Fatalf("Nearby: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a ghost entry was returned: %v", ids(got))
	}

	cell := geo.Encode(ikeja, store.Config().IndexPrecision())
	precision := geo.PickPrecision(ikeja, 5, store.Config().CellPrecisions, store.Config().MaxCoverCells)
	key := IndexKey(testNamespace, precision, cell.Parent(precision))
	if n, _ := client.ZCard(ctx, key).Result(); n != 0 {
		t.Fatalf("ghost still indexed in %s (%d members)", key, n)
	}
}

func TestFixKeyExpiresAfterTTL(t *testing.T) {
	store, mr, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.PutFix(ctx, fixAt("ttl", ikeja), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}
	mr.FastForward(DefaultFixTTL + time.Second)

	if _, ok, err := store.GetFix(ctx, "ttl"); err != nil || ok {
		t.Fatalf("GetFix after the TTL = (ok %v, err %v), want (false, nil)", ok, err)
	}
}

func TestPutFixPublishesLocationUpdate(t *testing.T) {
	store, _, client := newTestStore(t)
	ctx := context.Background()

	sub := Subscribe(ctx, client, testNamespace, "watched")
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil { // wait for the subscription to land
		t.Fatalf("Receive (subscribe confirmation): %v", err)
	}
	messages := sub.Channel()

	fix := fixAt("watched", ikeja)
	if err := store.PutFix(ctx, fix, true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}

	var update Update
	select {
	case msg := <-messages:
		if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
			t.Fatalf("unmarshal published update: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a published location update")
	}

	if update.Type != UpdateTypeLocation {
		t.Fatalf("update type = %q, want %q", update.Type, UpdateTypeLocation)
	}
	// provider_id is the compatibility alias the shipped rider app reads.
	if update.EntityID != "watched" || update.ProviderID != "watched" {
		t.Fatalf("update ids = (%q, %q), want both %q", update.EntityID, update.ProviderID, "watched")
	}
	if update.Heading != fix.Heading || update.Speed != fix.Speed || update.Accuracy != fix.Accuracy {
		t.Fatalf("telemetry lost in the published update: %+v", update)
	}
}

func TestRemovePublishesOffline(t *testing.T) {
	store, _, client := newTestStore(t)
	ctx := context.Background()

	if err := store.PutFix(ctx, fixAt("goner", ikeja), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}

	sub := Subscribe(ctx, client, testNamespace, "goner")
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("Receive (subscribe confirmation): %v", err)
	}
	messages := sub.Channel()

	if err := store.Remove(ctx, "goner"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	select {
	case msg := <-messages:
		var update Update
		if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
			t.Fatalf("unmarshal published update: %v", err)
		}
		if update.Type != UpdateTypeOffline {
			t.Fatalf("update type = %q, want %q", update.Type, UpdateTypeOffline)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for an offline update")
	}
}

func TestNearbyBoundsCandidateWork(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	store := NewRedisStore(client, Config{Namespace: testNamespace, MaxCandidates: 3})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		if err := store.PutFix(ctx, fixAt(id, geo.Destination(ikeja, float64(i)*36, 0.5)), true); err != nil {
			t.Fatalf("PutFix: %v", err)
		}
	}

	got, err := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 5, Limit: 50})
	if err != nil {
		t.Fatalf("Nearby: %v", err)
	}
	if len(got) > 3 {
		t.Fatalf("Nearby hydrated %d candidates, want at most MaxCandidates (3)", len(got))
	}
}

// A service booting without Redis must degrade to no-ops, never panic.
func TestNilClientDegradesToNoop(t *testing.T) {
	store := NewRedisStore(nil, Config{Namespace: testNamespace})
	ctx := context.Background()

	if err := store.PutFix(ctx, fixAt("x", ikeja), true); err != nil {
		t.Fatalf("PutFix on a nil client: %v", err)
	}
	if _, ok, err := store.GetFix(ctx, "x"); err != nil || ok {
		t.Fatalf("GetFix on a nil client = (ok %v, err %v)", ok, err)
	}
	if err := store.SetDiscoverable(ctx, "x", true); err != nil {
		t.Fatalf("SetDiscoverable on a nil client: %v", err)
	}
	if err := store.Remove(ctx, "x"); err != nil {
		t.Fatalf("Remove on a nil client: %v", err)
	}
	if got, err := store.Nearby(ctx, NearbyQuery{Center: ikeja, RadiusKm: 5}); err != nil || len(got) != 0 {
		t.Fatalf("Nearby on a nil client = (%v, %v)", got, err)
	}

	var nilStore *RedisStore
	if err := nilStore.PutFix(ctx, fixAt("x", ikeja), true); err != nil {
		t.Fatalf("PutFix on a nil store: %v", err)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{Namespace: "ns"}.WithDefaults()

	if cfg.FixTTL != DefaultFixTTL {
		t.Fatalf("FixTTL = %v, want %v", cfg.FixTTL, DefaultFixTTL)
	}
	if len(cfg.CellPrecisions) != 2 {
		t.Fatalf("CellPrecisions = %v, want two precisions", cfg.CellPrecisions)
	}
	// The finest precision is the one entity cells are recorded at; every
	// coarser one must be reachable from it by prefix.
	if got, want := cfg.IndexPrecision(), geo.FinePrecision; got != want {
		t.Fatalf("IndexPrecision() = %d, want %d", got, want)
	}
	for _, p := range cfg.CellPrecisions {
		if p > cfg.IndexPrecision() {
			t.Fatalf("precision %d is finer than IndexPrecision %d, so its cells are not prefixes", p, cfg.IndexPrecision())
		}
	}
}

// A service that already shipped a channel name to its apps must be able to
// keep publishing there while still using this index.
func TestChannelKeyOverride(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	legacy := func(entityID string) string { return "avail:loc:chan:" + entityID }
	store := NewRedisStore(client, Config{Namespace: "dispatch", ChannelKeyFunc: legacy})
	ctx := context.Background()

	if got, want := store.ChannelFor("r1"), legacy("r1"); got != want {
		t.Fatalf("ChannelFor = %q, want %q", got, want)
	}

	sub := client.Subscribe(ctx, legacy("r1"))
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("Receive (subscribe confirmation): %v", err)
	}
	messages := sub.Channel()

	if err := store.PutFix(ctx, fixAt("r1", ikeja), true); err != nil {
		t.Fatalf("PutFix: %v", err)
	}

	select {
	case msg := <-messages:
		var update Update
		if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if update.ProviderID != "r1" {
			t.Fatalf("provider_id = %q, want %q", update.ProviderID, "r1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nothing published on the overridden channel")
	}

	// Without an override the namespaced default still applies.
	plain := NewRedisStore(client, Config{Namespace: "dispatch"})
	if got, want := plain.ChannelFor("r1"), ChannelKey("dispatch", "r1"); got != want {
		t.Fatalf("default ChannelFor = %q, want %q", got, want)
	}
}

func TestKeysAreNamespaced(t *testing.T) {
	// Two services sharing one Redis must never collide.
	if FixKey("hauling", "p1") == FixKey("dispatch", "p1") {
		t.Fatal("fix keys collide across namespaces")
	}
	if IndexKey("hauling", 5, "u4pru") == IndexKey("dispatch", 5, "u4pru") {
		t.Fatal("index keys collide across namespaces")
	}
	if ChannelKey("hauling", "p1") == ChannelKey("dispatch", "p1") {
		t.Fatal("channels collide across namespaces")
	}
	if IndexKey("hauling", 4, "u4pr") == IndexKey("hauling", 5, "u4pr") {
		t.Fatal("index keys collide across precisions")
	}
}

func TestFixHelpers(t *testing.T) {
	now := time.Now().UTC()
	fix := Fix{EntityID: "p", Lat: ikeja.Lat, Lng: ikeja.Lng, RecordedAt: now.Add(-30 * time.Second)}

	if !fix.HasFix() {
		t.Fatal("HasFix() = false for a real coordinate")
	}
	if (Fix{EntityID: "p"}).HasFix() {
		t.Fatal("HasFix() = true for null island")
	}
	if got := fix.Age(now); got < 29*time.Second || got > 31*time.Second {
		t.Fatalf("Age() = %v, want about 30s", got)
	}
	if fix.IsStale(now, time.Minute) {
		t.Fatal("a 30s-old fix is not stale against a 60s TTL")
	}
	if !fix.IsStale(now, 10*time.Second) {
		t.Fatal("a 30s-old fix is stale against a 10s TTL")
	}
}

func ids(entities []NearbyEntity) []string {
	out := make([]string, len(entities))
	for i, e := range entities {
		out[i] = e.Fix.EntityID
	}
	return out
}
