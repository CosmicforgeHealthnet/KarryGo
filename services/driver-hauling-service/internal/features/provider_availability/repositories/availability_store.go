package availabilityrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/livetrack"
)

const (
	onlineSetKey   = "hauling:providers:online"
	statusKeyFmt   = "hauling:provider:status:%s"
	matchLockFmt   = "hauling:provider:matching:%s"
	rejectedSetFmt = "hauling:booking:rejected:%s"
)

// TrackingNamespace prefixes this service's keys in the shared live-tracking
// index, so hauling and dispatch can share a Redis without colliding.
const TrackingNamespace = "hauling"

type ProviderStatus struct {
	ProviderID string  `json:"provider_id"`
	TruckID    string  `json:"truck_id"`
	Lat        float64 `json:"lat"`
	Lng        float64 `json:"lng"`
	// Heading (degrees), Speed (m/s) and Accuracy (metres) come from the
	// provider device's GPS stream. They are optional: a heartbeat that
	// omits them still keeps the provider online, it just cannot rotate the
	// customer's map marker.
	Heading   float64 `json:"heading,omitempty"`
	Speed     float64 `json:"speed,omitempty"`
	Accuracy  float64 `json:"accuracy,omitempty"`
	UpdatedAt int64   `json:"updated_at"`
}

// Point exposes the status position to the shared geo helpers.
func (s ProviderStatus) Point() geo.Point { return geo.Point{Lat: s.Lat, Lng: s.Lng} }

// Telemetry is one position report from a provider device.
type Telemetry struct {
	Lat      float64
	Lng      float64
	Heading  float64
	Speed    float64
	Accuracy float64
}

type AvailabilityStore interface {
	SetOnline(ctx context.Context, status ProviderStatus, ttl time.Duration) error
	SetOffline(ctx context.Context, providerID string) error
	Heartbeat(ctx context.Context, providerID string, fix Telemetry, ttl time.Duration) error
	CountOnline(ctx context.Context) (int64, error)
	GetOnlineProviders(ctx context.Context) ([]ProviderStatus, error)
	// NearbyProviders returns online providers within radiusKm of center,
	// nearest first, reading the spatial index rather than scanning every
	// online provider.
	NearbyProviders(ctx context.Context, center geo.Point, radiusKm float64, limit int) ([]ProviderStatus, error)
	GetProviderStatus(ctx context.Context, providerID string) (ProviderStatus, bool, error)
	// SetDiscoverable removes a provider from the matching index while
	// keeping their position readable, so a provider on a job still shows on
	// the customer's map.
	SetDiscoverable(ctx context.Context, providerID string, discoverable bool) error
	AcquireMatchLock(ctx context.Context, providerID, bookingID string, ttl time.Duration) (bool, error)
	ReleaseMatchLock(ctx context.Context, providerID string) error
	MarkRejected(ctx context.Context, bookingID, providerID string, ttl time.Duration) error
	GetRejectedProviders(ctx context.Context, bookingID string) (map[string]bool, error)
}

type RedisAvailabilityStore struct {
	client *redis.Client
	// track is the shared geohash-cell index. The legacy online set and
	// per-provider status keys are still written so nothing that reads them
	// changes behaviour; proximity queries read the index instead.
	track livetrack.Store
}

func NewRedisAvailabilityStore(client *redis.Client) *RedisAvailabilityStore {
	return NewRedisAvailabilityStoreWithTTL(client, livetrack.DefaultFixTTL)
}

// NewRedisAvailabilityStoreWithTTL builds a store whose tracking index expires
// fixes after the same window the availability keys use.
func NewRedisAvailabilityStoreWithTTL(client *redis.Client, fixTTL time.Duration) *RedisAvailabilityStore {
	return &RedisAvailabilityStore{
		client: client,
		track: livetrack.NewRedisStore(client, livetrack.Config{
			Namespace: TrackingNamespace,
			FixTTL:    fixTTL,
		}),
	}
}

// Track exposes the tracking store so the booking feature can read a single
// provider's live fix and subscribe to their update channel.
func (s *RedisAvailabilityStore) Track() livetrack.Store { return s.track }

// putFix mirrors a provider status into the shared tracking index. Tracking is
// best-effort relative to availability: a tracking write failure must not stop
// a provider going online or being matched.
func (s *RedisAvailabilityStore) putFix(ctx context.Context, st ProviderStatus, discoverable bool) {
	if s.track == nil || !st.Point().HasFix() {
		return
	}
	recorded := time.Unix(st.UpdatedAt, 0).UTC()
	if st.UpdatedAt == 0 {
		recorded = time.Now().UTC()
	}
	_ = s.track.PutFix(ctx, livetrack.Fix{
		EntityID:   st.ProviderID,
		Lat:        st.Lat,
		Lng:        st.Lng,
		Heading:    st.Heading,
		Speed:      st.Speed,
		Accuracy:   st.Accuracy,
		RecordedAt: recorded,
		ReceivedAt: time.Now().UTC(),
		Source:     "driver-hauling-service",
	}, discoverable)
}

func (s *RedisAvailabilityStore) SetOnline(ctx context.Context, status ProviderStatus, ttl time.Duration) error {
	payload, err := json.Marshal(status)
	if err != nil {
		return apperrors.Internal("Availability data could not be prepared.", err)
	}

	pipe := s.client.Pipeline()
	pipe.SAdd(ctx, onlineSetKey, status.ProviderID)
	pipe.Set(ctx, statusKey(status.ProviderID), payload, ttl)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return apperrors.Unavailable("Availability service is temporarily unavailable.", err)
	}
	s.putFix(ctx, status, true)
	return nil
}

func (s *RedisAvailabilityStore) SetOffline(ctx context.Context, providerID string) error {
	pipe := s.client.Pipeline()
	pipe.SRem(ctx, onlineSetKey, providerID)
	pipe.Del(ctx, statusKey(providerID))
	_, err := pipe.Exec(ctx)
	if err != nil {
		return apperrors.Unavailable("Availability service is temporarily unavailable.", err)
	}
	if s.track != nil {
		_ = s.track.Remove(ctx, providerID)
	}
	return nil
}

func (s *RedisAvailabilityStore) Heartbeat(ctx context.Context, providerID string, fix Telemetry, ttl time.Duration) error {
	existing, ok, err := s.GetProviderStatus(ctx, providerID)
	if err != nil {
		return err
	}
	if !ok {
		return apperrors.BadRequest("You are not currently online. Please go online first.", nil)
	}

	existing.Lat = fix.Lat
	existing.Lng = fix.Lng
	existing.Heading = fix.Heading
	existing.Speed = fix.Speed
	existing.Accuracy = fix.Accuracy
	existing.UpdatedAt = time.Now().Unix()

	payload, err := json.Marshal(existing)
	if err != nil {
		return apperrors.Internal("Heartbeat data could not be prepared.", err)
	}

	pipe := s.client.Pipeline()
	pipe.Set(ctx, statusKey(providerID), payload, ttl)
	pipe.SAdd(ctx, onlineSetKey, providerID)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	s.putFix(ctx, existing, true)
	return nil
}

// NearbyProviders reads the geohash cell index instead of scanning the whole
// online set, so matching cost scales with providers near the pickup rather
// than with providers on the platform.
func (s *RedisAvailabilityStore) NearbyProviders(ctx context.Context, center geo.Point, radiusKm float64, limit int) ([]ProviderStatus, error) {
	if s.track == nil {
		return nil, nil
	}
	found, err := s.track.Nearby(ctx, livetrack.NearbyQuery{Center: center, RadiusKm: radiusKm, Limit: limit})
	if err != nil {
		return nil, apperrors.Unavailable("Availability service is temporarily unavailable.", err)
	}

	statuses := make([]ProviderStatus, 0, len(found))
	for _, entity := range found {
		// The index knows positions; the status key knows which truck is
		// on duty, which matching needs. A provider indexed without a live
		// status key has gone offline since their last fix.
		status, ok, err := s.GetProviderStatus(ctx, entity.Fix.EntityID)
		if err != nil || !ok {
			continue
		}
		// Prefer the tracking fix: it is the fresher of the two.
		status.Lat = entity.Fix.Lat
		status.Lng = entity.Fix.Lng
		status.Heading = entity.Fix.Heading
		status.Speed = entity.Fix.Speed
		status.Accuracy = entity.Fix.Accuracy
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// SetDiscoverable keeps a provider's position readable while taking them out
// of the matching index.
func (s *RedisAvailabilityStore) SetDiscoverable(ctx context.Context, providerID string, discoverable bool) error {
	if s.track == nil {
		return nil
	}
	return s.track.SetDiscoverable(ctx, providerID, discoverable)
}

func (s *RedisAvailabilityStore) CountOnline(ctx context.Context) (int64, error) {
	count, err := s.client.SCard(ctx, onlineSetKey).Result()
	if err != nil {
		return 0, apperrors.Unavailable("Availability service is temporarily unavailable.", err)
	}
	return count, nil
}

func (s *RedisAvailabilityStore) GetOnlineProviders(ctx context.Context) ([]ProviderStatus, error) {
	ids, err := s.client.SMembers(ctx, onlineSetKey).Result()
	if err != nil {
		return nil, apperrors.Unavailable("Availability service is temporarily unavailable.", err)
	}

	var statuses []ProviderStatus
	for _, id := range ids {
		st, ok, err := s.GetProviderStatus(ctx, id)
		if err != nil || !ok {
			// Provider's key expired but set entry remains stale — skip and clean up
			_ = s.client.SRem(ctx, onlineSetKey, id)
			continue
		}
		statuses = append(statuses, st)
	}
	return statuses, nil
}

func (s *RedisAvailabilityStore) GetProviderStatus(ctx context.Context, providerID string) (ProviderStatus, bool, error) {
	value, err := s.client.Get(ctx, statusKey(providerID)).Result()
	if errors.Is(err, redis.Nil) {
		return ProviderStatus{}, false, nil
	}
	if err != nil {
		return ProviderStatus{}, false, apperrors.Unavailable("Availability service is temporarily unavailable.", err)
	}

	var st ProviderStatus
	if err := json.Unmarshal([]byte(value), &st); err != nil {
		return ProviderStatus{}, false, apperrors.Internal("Availability data could not be read.", err)
	}
	return st, true, nil
}

func (s *RedisAvailabilityStore) AcquireMatchLock(ctx context.Context, providerID, bookingID string, ttl time.Duration) (bool, error) {
	ok, err := s.client.SetNX(ctx, matchLockKey(providerID), bookingID, ttl).Result()
	if err != nil {
		return false, apperrors.Unavailable("Matching service is temporarily unavailable.", err)
	}
	return ok, nil
}

func (s *RedisAvailabilityStore) ReleaseMatchLock(ctx context.Context, providerID string) error {
	return s.client.Del(ctx, matchLockKey(providerID)).Err()
}

func (s *RedisAvailabilityStore) MarkRejected(ctx context.Context, bookingID, providerID string, ttl time.Duration) error {
	pipe := s.client.Pipeline()
	pipe.SAdd(ctx, rejectedKey(bookingID), providerID)
	pipe.Expire(ctx, rejectedKey(bookingID), ttl)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return apperrors.Unavailable("Matching service is temporarily unavailable.", err)
	}
	return nil
}

func (s *RedisAvailabilityStore) GetRejectedProviders(ctx context.Context, bookingID string) (map[string]bool, error) {
	ids, err := s.client.SMembers(ctx, rejectedKey(bookingID)).Result()
	if err != nil {
		return nil, apperrors.Unavailable("Matching service is temporarily unavailable.", err)
	}
	rejected := make(map[string]bool, len(ids))
	for _, id := range ids {
		rejected[id] = true
	}
	return rejected, nil
}

func statusKey(providerID string) string {
	return fmt.Sprintf(statusKeyFmt, providerID)
}

func matchLockKey(providerID string) string {
	return fmt.Sprintf(matchLockFmt, providerID)
}

func rejectedKey(bookingID string) string {
	return fmt.Sprintf(rejectedSetFmt, bookingID)
}
