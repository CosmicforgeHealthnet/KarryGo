package securehandover

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) error
}

type RedisRateLimiter struct{ client *redis.Client }

var allowScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return current
`)

func NewRedisRateLimiter(client *redis.Client) *RedisRateLimiter {
	return &RedisRateLimiter{client: client}
}

func (l *RedisRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) error {
	if l == nil || l.client == nil {
		return errors.New("handover rate limiter is unavailable")
	}
	if limit <= 0 || window <= 0 {
		return errors.New("handover rate limit policy is invalid")
	}
	value, err := allowScript.Run(ctx, l.client, []string{"dispatch:secure_handover:" + key}, window.Milliseconds()).Int64()
	if err != nil {
		return err
	}
	if value > int64(limit) {
		return ErrLocked
	}
	return nil
}
