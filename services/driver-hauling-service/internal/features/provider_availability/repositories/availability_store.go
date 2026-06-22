package availabilityrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	onlineSetKey  = "hauling:providers:online"
	statusKeyFmt  = "hauling:provider:status:%s"
	matchLockFmt  = "hauling:provider:matching:%s"
)

type ProviderStatus struct {
	ProviderID string  `json:"provider_id"`
	TruckID    string  `json:"truck_id"`
	Lat        float64 `json:"lat"`
	Lng        float64 `json:"lng"`
	UpdatedAt  int64   `json:"updated_at"`
}

type AvailabilityStore interface {
	SetOnline(ctx context.Context, status ProviderStatus, ttl time.Duration) error
	SetOffline(ctx context.Context, providerID string) error
	Heartbeat(ctx context.Context, providerID string, lat, lng float64, ttl time.Duration) error
	CountOnline(ctx context.Context) (int64, error)
	GetOnlineProviders(ctx context.Context) ([]ProviderStatus, error)
	GetProviderStatus(ctx context.Context, providerID string) (ProviderStatus, bool, error)
	AcquireMatchLock(ctx context.Context, providerID, bookingID string, ttl time.Duration) (bool, error)
	ReleaseMatchLock(ctx context.Context, providerID string) error
}

type RedisAvailabilityStore struct {
	client *redis.Client
}

func NewRedisAvailabilityStore(client *redis.Client) *RedisAvailabilityStore {
	return &RedisAvailabilityStore{client: client}
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
	return nil
}

func (s *RedisAvailabilityStore) Heartbeat(ctx context.Context, providerID string, lat, lng float64, ttl time.Duration) error {
	existing, ok, err := s.GetProviderStatus(ctx, providerID)
	if err != nil {
		return err
	}
	if !ok {
		return apperrors.BadRequest("You are not currently online. Please go online first.", nil)
	}

	existing.Lat = lat
	existing.Lng = lng
	existing.UpdatedAt = time.Now().Unix()

	payload, err := json.Marshal(existing)
	if err != nil {
		return apperrors.Internal("Heartbeat data could not be prepared.", err)
	}

	pipe := s.client.Pipeline()
	pipe.Set(ctx, statusKey(providerID), payload, ttl)
	pipe.SAdd(ctx, onlineSetKey, providerID)
	_, err = pipe.Exec(ctx)
	return err
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

func statusKey(providerID string) string {
	return fmt.Sprintf(statusKeyFmt, providerID)
}

func matchLockKey(providerID string) string {
	return fmt.Sprintf(matchLockFmt, providerID)
}
