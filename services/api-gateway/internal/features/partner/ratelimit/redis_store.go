package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// incrScript atomically increments a counter and sets the TTL only on creation,
// so a fixed window is never extended by later requests within it.
var incrScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return current
`)

// RedisStore is the production rate-limit Store.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore builds a RedisStore.
func NewRedisStore(client *redis.Client) *RedisStore { return &RedisStore{client: client} }

// Incr increments the counter for key and returns the new value.
func (s *RedisStore) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	return incrScript.Run(ctx, s.client, []string{key}, int(window.Seconds())).Int64()
}
