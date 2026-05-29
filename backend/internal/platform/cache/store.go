package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"karrygo/backend/internal/platform/apperrors"
)

type Store struct {
	client *redis.Client
	prefix string
}

func NewStore(client *redis.Client, prefix string) *Store {
	return &Store{client: client, prefix: strings.Trim(prefix, ":")}
}

func (s *Store) GetJSON(ctx context.Context, key string, target interface{}) (bool, error) {
	value, err := s.client.Get(ctx, s.key(key)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}

	if err != nil {
		return false, apperrors.Unavailable("Cache is temporarily unavailable.", err)
	}

	if err := json.Unmarshal([]byte(value), target); err != nil {
		return false, apperrors.Internal("Cached data could not be read.", err)
	}

	return true, nil
}

func (s *Store) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return apperrors.Internal("Cache payload could not be prepared.", err)
	}

	if err := s.client.Set(ctx, s.key(key), payload, ttl).Err(); err != nil {
		return apperrors.Unavailable("Cache is temporarily unavailable.", err)
	}

	return nil
}

func (s *Store) Delete(ctx context.Context, keys ...string) error {
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		normalized = append(normalized, s.key(key))
	}

	if len(normalized) == 0 {
		return nil
	}

	if err := s.client.Del(ctx, normalized...).Err(); err != nil {
		return apperrors.Unavailable("Cache is temporarily unavailable.", err)
	}

	return nil
}

func (s *Store) key(key string) string {
	key = strings.Trim(key, ":")
	if s.prefix == "" {
		return key
	}

	return fmt.Sprintf("%s:%s", s.prefix, key)
}
