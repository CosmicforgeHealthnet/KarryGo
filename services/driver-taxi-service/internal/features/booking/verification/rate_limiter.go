package verification

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/handover"
)

// RateLimiter caps verify-attempt RATE within a short window
// (taxi:{step}:verify:rate:{bookingID}, addendum §32 table), layered on top
// of -- never a substitute for -- Repository's own durable per-code attempt
// cap in Postgres: that one caps LIFETIME attempts against one generated
// code, this caps burst rate within a window regardless of which code the
// attempts are against. Same INCR+EXPIRE idiom already used by
// provider_auth's OTP rate limiter (redis_otp_repository.go).
type RateLimiter struct {
	client *redis.Client
}

func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// rateKey is per (step, booking): a burst of wrong PICKUP codes must not
// consume the delivery budget for the same booking, since the two handovers
// happen at different places to potentially different people.
func rateKey(bookingID string, step handover.Step) string {
	return fmt.Sprintf("taxi:%s:verify:rate:%s", string(step), bookingID)
}

// Allow increments the window counter and reports whether this attempt is
// within the configured max. The FIRST increment in a fresh window sets the
// key's expiry; subsequent increments within the window do not extend it,
// so the window is fixed, not a rolling/sliding one.
func (l *RateLimiter) Allow(ctx context.Context, bookingID string, step handover.Step, window time.Duration, max int) (bool, error) {
	key := rateKey(bookingID, step)
	count, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		_ = l.client.Expire(ctx, key, window).Err()
	}
	return count <= int64(max), nil
}
