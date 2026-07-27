// Package ratelimit implements bounded counters for authentication throttling.
//
// Counters live in Redis with a mandatory TTL when it is configured, and fall
// back to a bounded in-process limiter when it is not. Redis is never the
// source of truth for challenge status — that stays in PostgreSQL; these
// counters only decide whether to accept another attempt right now.
package ratelimit

import (
	"context"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cache"
)

// Rule bounds one class of attempts.
type Rule struct {
	Limit  int
	Window time.Duration
}

// Named rule keys.
const (
	ScopeChallengeCreate    = "challenge_create"
	ScopeVerify             = "verify"
	ScopeResend             = "resend"
	ScopeRefresh            = "refresh"
	ScopeOrganizationSwitch = "organization_switch"
	ScopeInvitationAccept   = "invitation_accept"
	ScopeStepUp             = "step_up"
)

// Limiter counts attempts per scope and key.
type Limiter interface {
	// Allow increments the counter and reports whether the attempt is within
	// the rule. It fails open only for infrastructure errors, never for a
	// confirmed over-limit result.
	Allow(ctx context.Context, scope string, key string, rule Rule) (bool, error)
}

// Counter is the minimal Redis surface the limiter needs.
type Counter interface {
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)
}

// RedisLimiter counts in Redis with a mandatory TTL.
type RedisLimiter struct {
	counter Counter
}

// NewRedisLimiter wraps a counter.
func NewRedisLimiter(counter Counter) *RedisLimiter {
	return &RedisLimiter{counter: counter}
}

// Allow increments and compares against the rule.
func (l *RedisLimiter) Allow(ctx context.Context, scope string, key string, rule Rule) (bool, error) {
	if rule.Limit <= 0 || rule.Window <= 0 {
		return true, nil
	}
	count, err := l.counter.Incr(ctx, cache.Key("ratelimit", scope, key), rule.Window)
	if err != nil {
		// Infrastructure failure must not lock everyone out; the durable
		// per-challenge attempt limit in PostgreSQL still applies.
		return true, err
	}
	return count <= int64(rule.Limit), nil
}

// MemoryLimiter is the bounded in-process fallback used when Redis is not
// configured. It keeps only live windows and prunes on write.
type MemoryLimiter struct {
	mu      sync.Mutex
	entries map[string]*window
	now     func() time.Time
}

type window struct {
	count     int
	expiresAt time.Time
}

// NewMemoryLimiter builds an in-process limiter.
func NewMemoryLimiter() *MemoryLimiter {
	return &MemoryLimiter{entries: map[string]*window{}, now: time.Now}
}

// Allow increments and compares against the rule.
func (l *MemoryLimiter) Allow(_ context.Context, scope string, key string, rule Rule) (bool, error) {
	if rule.Limit <= 0 || rule.Window <= 0 {
		return true, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.pruneLocked(now)

	fullKey := scope + "|" + key
	entry, ok := l.entries[fullKey]
	if !ok || now.After(entry.expiresAt) {
		entry = &window{expiresAt: now.Add(rule.Window)}
		l.entries[fullKey] = entry
	}
	entry.count++
	return entry.count <= rule.Limit, nil
}

func (l *MemoryLimiter) pruneLocked(now time.Time) {
	for key, entry := range l.entries {
		if now.After(entry.expiresAt) {
			delete(l.entries, key)
		}
	}
}
