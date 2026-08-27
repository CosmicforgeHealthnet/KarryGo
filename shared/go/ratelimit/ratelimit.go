// Package ratelimit provides the platform Redis-backed fixed-window abuse
// limiter. Redis is the production authority; callers may supply small fakes
// only in tests of HTTP/use-case behavior.
package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/apperrors"
)

type Limiter struct {
	client    redis.Scripter
	keyPrefix string
}

func New(client redis.Scripter, keyPrefix string) *Limiter {
	return &Limiter{client: client, keyPrefix: strings.Trim(strings.TrimSpace(keyPrefix), ":")}
}

var incrementWithExpiry = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return current
`)

func (l *Limiter) Allow(ctx context.Context, bucket, key string, max int, window time.Duration) error {
	if max <= 0 {
		return nil
	}
	if l == nil || l.client == nil || strings.TrimSpace(key) == "" || window <= 0 {
		return apperrors.Unavailable("Rate limiting is temporarily unavailable.", nil)
	}
	count, err := incrementWithExpiry.Run(ctx, l.client, []string{l.redisKey(bucket, key)}, window.Milliseconds()).Int64()
	if err != nil {
		return apperrors.Unavailable("Rate limiting is temporarily unavailable.", err)
	}
	if count > int64(max) {
		return apperrors.RateLimited("Too many requests. Please wait before trying again.", nil)
	}
	return nil
}

func (l *Limiter) redisKey(bucket, key string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return fmt.Sprintf("%s:%s:%s", l.keyPrefix, strings.Trim(strings.TrimSpace(bucket), ":"), hex.EncodeToString(digest[:]))
}
