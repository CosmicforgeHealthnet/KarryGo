package availabilityrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/livetrack"
)

// Redis keys. Every one is namespaced `taxi:` so this service can share a Redis
// instance with the hauling and dispatch verticals without colliding.
const (
	onlineSetKey  = "taxi:providers:online"
	statusKeyFmt  = "taxi:provider:status:%s"
	matchLockFmt  = "taxi:provider:matching:%s"
	rejectedFmt   = "taxi:booking:rejected:%s"
	defaultOnline = 90 * time.Second

	// trackingNamespacePrefix roots the per-market/class geo index namespaces
	// this service builds at runtime (see trackFor): addendum §4.2 asks for
	// taxi:providers:geo:{marketID}:{vehicleClass} specifically, rather than
	// one flat "taxi" namespace, so candidate search is scoped to a market
	// before eligibility filtering runs. It must differ from the other
	// verticals' namespaces for the same reason as the keys above.
	trackingNamespacePrefix = "taxi:providers:geo"
)

func statusKey(providerID string) string    { return fmt.Sprintf(statusKeyFmt, providerID) }
func matchLockKey(providerID string) string { return fmt.Sprintf(matchLockFmt, providerID) }
func rejectedKey(bookingID string) string   { return fmt.Sprintf(rejectedFmt, bookingID) }

// ProviderStatus is the heartbeat payload: where a driver is, which vehicle
// they are working with, and which market/class partition their position is
// indexed under right now.
type ProviderStatus struct {
	ProviderID   string  `json:"provider_id"`
	VehicleID    string  `json:"vehicle_id,omitempty"`
	MarketID     string  `json:"market_id,omitempty"`
	VehicleClass string  `json:"vehicle_class,omitempty"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	Heading      float64 `json:"heading,omitempty"`
	Speed        float64 `json:"speed,omitempty"`
	Accuracy     float64 `json:"accuracy,omitempty"`
	UpdatedAt    int64   `json:"updated_at"`
}

type AvailabilityStore interface {
	// SetOnline and Heartbeat require MarketID and VehicleClass on status: the
	// caller (the availability usecase) resolves the operating zone from the
	// driver's coordinates before calling in, because zone resolution needs
	// Postgres and this store is Redis-only by design.
	SetOnline(ctx context.Context, status ProviderStatus) error
	SetOffline(ctx context.Context, providerID string) error
	Heartbeat(ctx context.Context, status ProviderStatus) error
	GetStatus(ctx context.Context, providerID string) (ProviderStatus, bool, error)
	GetOnlineProviders(ctx context.Context) ([]ProviderStatus, error)
	// NearbyProviders queries the geo index for one (market, vehicleClass)
	// partition only. A booking never needs to search outside its own market
	// or across vehicle classes it did not request.
	NearbyProviders(ctx context.Context, marketID, vehicleClass string, center geo.Point, radiusKm float64, limit int) ([]ProviderStatus, error)
	// SetDiscoverable takes a driver out of match candidacy while keeping their
	// position readable — the state a driver is in once they are on a ride.
	// Needs marketID/vehicleClass for the same reason NearbyProviders does: the
	// index it mutates is partitioned by both.
	SetDiscoverable(ctx context.Context, providerID, marketID, vehicleClass string, discoverable bool) error

	AcquireMatchLock(ctx context.Context, providerID, bookingID string, ttl time.Duration) (bool, error)
	ReleaseMatchLock(ctx context.Context, providerID string) error
	MarkRejected(ctx context.Context, bookingID, providerID string, ttl time.Duration) error
	GetRejectedProviders(ctx context.Context, bookingID string) (map[string]bool, error)
}

type RedisAvailabilityStore struct {
	client *redis.Client
	ttl    time.Duration

	// tracks caches one *livetrack.RedisStore per (market, vehicleClass)
	// partition, built lazily. The set of partitions is small and bounded (a
	// handful of markets times three vehicle classes), so caching avoids
	// reallocating a store on every heartbeat without adding real complexity.
	tracksMu sync.Mutex
	tracks   map[string]*livetrack.RedisStore
}

func NewRedisAvailabilityStore(client *redis.Client) *RedisAvailabilityStore {
	return NewRedisAvailabilityStoreWithTTL(client, defaultOnline)
}

// NewRedisAvailabilityStoreWithTTL binds the tracking fix TTL to the same clock
// as the online status key, so a driver can never be matched from a position
// that has already aged out of their availability.
func NewRedisAvailabilityStoreWithTTL(client *redis.Client, ttl time.Duration) *RedisAvailabilityStore {
	if ttl <= 0 {
		ttl = defaultOnline
	}
	return &RedisAvailabilityStore{
		client: client,
		ttl:    ttl,
		tracks: map[string]*livetrack.RedisStore{},
	}
}

// trackFor returns the geo-index store for one (market, vehicleClass)
// partition, creating and caching it on first use. A blank marketID or
// vehicleClass still produces a valid (if degenerate) partition rather than
// panicking, so a caller that has not yet resolved a zone fails a later,
// clearer check instead of crashing here.
func (s *RedisAvailabilityStore) trackFor(marketID, vehicleClass string) *livetrack.RedisStore {
	key := marketID + ":" + vehicleClass
	s.tracksMu.Lock()
	defer s.tracksMu.Unlock()
	if t, ok := s.tracks[key]; ok {
		return t
	}
	t := livetrack.NewRedisStore(s.client, livetrack.Config{
		Namespace: trackingNamespacePrefix + ":" + key,
		FixTTL:    s.ttl,
	})
	s.tracks[key] = t
	return t
}

// Track exposes one partition's position store so the booking feature can
// read a driver's fix directly without going through the availability
// service.
func (s *RedisAvailabilityStore) Track(marketID, vehicleClass string) *livetrack.RedisStore {
	return s.trackFor(marketID, vehicleClass)
}

func (s *RedisAvailabilityStore) SetOnline(ctx context.Context, status ProviderStatus) error {
	return s.write(ctx, status, true)
}

func (s *RedisAvailabilityStore) Heartbeat(ctx context.Context, status ProviderStatus) error {
	return s.write(ctx, status, true)
}

func (s *RedisAvailabilityStore) write(ctx context.Context, status ProviderStatus, discoverable bool) error {
	status.UpdatedAt = time.Now().Unix()
	payload, err := json.Marshal(status)
	if err != nil {
		return err
	}

	pipe := s.client.Pipeline()
	pipe.SAdd(ctx, onlineSetKey, status.ProviderID)
	pipe.Set(ctx, statusKey(status.ProviderID), payload, s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	// Mirroring into the spatial index is best-effort: a tracking failure must
	// never stop a driver going online or receiving a heartbeat.
	s.putFix(ctx, status, discoverable)
	return nil
}

func (s *RedisAvailabilityStore) putFix(ctx context.Context, status ProviderStatus, discoverable bool) {
	if status.MarketID == "" || status.VehicleClass == "" {
		// No zone resolved for this position: there is nowhere correct to index
		// it. The provider is still online (the status key above is set) and
		// still readable by id, they simply will not surface in a proximity
		// query until a heartbeat carries a resolved zone.
		return
	}
	now := time.Now()
	_ = s.trackFor(status.MarketID, status.VehicleClass).PutFix(ctx, livetrack.Fix{
		EntityID:   status.ProviderID,
		Lat:        status.Lat,
		Lng:        status.Lng,
		Heading:    status.Heading,
		Speed:      status.Speed,
		Accuracy:   status.Accuracy,
		RecordedAt: now,
		ReceivedAt: now,
		Source:     "taxi-service",
	}, discoverable)
}

func (s *RedisAvailabilityStore) SetOffline(ctx context.Context, providerID string) error {
	// The caller may not know which partition this provider was last indexed
	// under (going offline does not require re-resolving a zone), so read the
	// status key first to find it, then remove from that specific partition.
	existing, ok, _ := s.GetStatus(ctx, providerID)

	pipe := s.client.Pipeline()
	pipe.SRem(ctx, onlineSetKey, providerID)
	pipe.Del(ctx, statusKey(providerID))
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	if ok && existing.MarketID != "" && existing.VehicleClass != "" {
		_ = s.trackFor(existing.MarketID, existing.VehicleClass).Remove(ctx, providerID)
	}
	return nil
}

func (s *RedisAvailabilityStore) SetDiscoverable(ctx context.Context, providerID, marketID, vehicleClass string, discoverable bool) error {
	if marketID == "" || vehicleClass == "" {
		return nil
	}
	return s.trackFor(marketID, vehicleClass).SetDiscoverable(ctx, providerID, discoverable)
}

func (s *RedisAvailabilityStore) GetStatus(ctx context.Context, providerID string) (ProviderStatus, bool, error) {
	value, err := s.client.Get(ctx, statusKey(providerID)).Result()
	if errors.Is(err, redis.Nil) {
		return ProviderStatus{}, false, nil
	}
	if err != nil {
		return ProviderStatus{}, false, err
	}
	var status ProviderStatus
	if err := json.Unmarshal([]byte(value), &status); err != nil {
		return ProviderStatus{}, false, err
	}
	return status, true, nil
}

// GetOnlineProviders returns every driver whose status key is still live,
// pruning the set as it goes.
//
// The online set has no TTL of its own, so SMEMBERS keeps returning drivers
// whose status key expired — a force-quit driver would otherwise stay
// dispatchable and burn match offers. Callers must use this rather than SCARD
// for the same reason.
func (s *RedisAvailabilityStore) GetOnlineProviders(ctx context.Context) ([]ProviderStatus, error) {
	ids, err := s.client.SMembers(ctx, onlineSetKey).Result()
	if err != nil {
		return nil, err
	}

	var out []ProviderStatus
	var stale []string
	for _, id := range ids {
		status, ok, err := s.GetStatus(ctx, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			stale = append(stale, id)
			continue
		}
		out = append(out, status)
	}
	if len(stale) > 0 {
		_ = s.client.SRem(ctx, onlineSetKey, toAny(stale)...).Err()
	}
	return out, nil
}

// NearbyProviders answers proximity from ONE (market, vehicleClass) geo
// index, then re-reads each candidate's status key.
//
// Being indexed is not the same as being available: the index knows positions,
// the status key knows who is actually on duty and in which vehicle. A hit with
// no live status is evicted from the index rather than returned.
func (s *RedisAvailabilityStore) NearbyProviders(ctx context.Context, marketID, vehicleClass string, center geo.Point, radiusKm float64, limit int) ([]ProviderStatus, error) {
	if marketID == "" || vehicleClass == "" {
		return nil, nil
	}
	track := s.trackFor(marketID, vehicleClass)
	entities, err := track.Nearby(ctx, livetrack.NearbyQuery{
		Center:   center,
		RadiusKm: radiusKm,
		Limit:    limit,
	})
	if err != nil {
		return nil, err
	}

	out := make([]ProviderStatus, 0, len(entities))
	for _, e := range entities {
		status, ok, err := s.GetStatus(ctx, e.Fix.EntityID)
		if err != nil {
			return nil, err
		}
		if !ok {
			_ = track.SetDiscoverable(ctx, e.Fix.EntityID, false)
			continue
		}
		// The tracking fix is the fresher position; prefer it over whatever the
		// status key was written with.
		status.Lat = e.Fix.Lat
		status.Lng = e.Fix.Lng
		if e.Fix.Heading != 0 {
			status.Heading = e.Fix.Heading
		}
		if e.Fix.Speed != 0 {
			status.Speed = e.Fix.Speed
		}
		if e.Fix.Accuracy != 0 {
			status.Accuracy = e.Fix.Accuracy
		}
		out = append(out, status)
	}
	return out, nil
}

// AcquireMatchLock reserves a driver for one booking offer. SETNX means the
// first matcher wins; a second booking cannot offer to the same driver until
// the lock is released or its TTL expires.
func (s *RedisAvailabilityStore) AcquireMatchLock(ctx context.Context, providerID, bookingID string, ttl time.Duration) (bool, error) {
	return s.client.SetNX(ctx, matchLockKey(providerID), bookingID, ttl).Result()
}

func (s *RedisAvailabilityStore) ReleaseMatchLock(ctx context.Context, providerID string) error {
	return s.client.Del(ctx, matchLockKey(providerID)).Err()
}

// MarkRejected records that a driver has already been offered this booking,
// whether they declined or simply let the offer time out.
//
// Without this the nearest silent driver is re-offered on every rescan and
// consumes the entire search window.
func (s *RedisAvailabilityStore) MarkRejected(ctx context.Context, bookingID, providerID string, ttl time.Duration) error {
	key := rejectedKey(bookingID)
	pipe := s.client.Pipeline()
	pipe.SAdd(ctx, key, providerID)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisAvailabilityStore) GetRejectedProviders(ctx context.Context, bookingID string) (map[string]bool, error) {
	ids, err := s.client.SMembers(ctx, rejectedKey(bookingID)).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

func toAny(values []string) []interface{} {
	out := make([]interface{}, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}
