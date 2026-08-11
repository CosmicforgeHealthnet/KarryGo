// Package ratelimit implements the platform's established Redis
// INCR+EXPIRE rate-limit idiom (the same pattern
// driver-hauling-service's provider_auth OTP repository already uses inline)
// as a small reusable component, so every Section 5 flow (OTP start/resend/
// verify, refresh, step-up start/verify) shares one implementation instead of
// five copies.
//
// Fails closed for authorization-relevant limits by design: if Redis is
// unavailable, Allow returns an error rather than silently permitting the
// request, per the architecture's degraded-state policy (critical
// authorization must not fail open because Redis is unavailable).
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Limiter enforces a max-count-per-window limit per key, namespaced so
// different flows (otp_start, otp_verify, refresh, step_up, ...) never share
// a counter even for the same identity.
type Limiter struct {
	client    *redis.Client
	keyPrefix string
}

func New(client *redis.Client, keyPrefix string) *Limiter {
	return &Limiter{client: client, keyPrefix: keyPrefix}
}

// Allow increments the counter for (bucket, key) and returns an
// apperrors.RateLimited error once count exceeds max within window. Returns
// apperrors.Unavailable (never a silent pass) if Redis cannot be reached.
func (l *Limiter) Allow(ctx context.Context, bucket, key string, max int, window time.Duration) error {
	if max <= 0 {
		return nil // an unset/zero limit means "not enforced" for this bucket
	}
	redisKey := fmt.Sprintf("%s:%s:%s", l.keyPrefix, bucket, key)
	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return apperrors.Unavailable("Rate limiting is temporarily unavailable.", err)
	}
	if count == 1 {
		_ = l.client.Expire(ctx, redisKey, window)
	}
	if count > int64(max) {
		return apperrors.RateLimited("Too many requests. Please wait before trying again.", nil)
	}
	return nil
}
