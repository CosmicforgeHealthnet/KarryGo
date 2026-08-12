// Package cache is the bounded Redis wrapper for fleet-partner-service.
//
// Redis is never a source of truth (membership, permissions, assignments,
// trip state, balances, audit, outbox, jobs all live in PostgreSQL). This
// wrapper enforces the operational rules: every key is namespaced under
// "fleet-partner:" and every entry has an explicit TTL — writes without a
// positive TTL are rejected before any network call. A nil client degrades
// safely: reads miss, writes error, nothing panics.
package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Namespace prefixes every key this service stores in Redis.
const Namespace = "fleet-partner:"

// ErrTTLRequired is returned when a write carries no positive TTL.
var ErrTTLRequired = errors.New("cache: a positive TTL is required for every entry")

// ErrNotConfigured is returned by writes when Redis is not configured.
var ErrNotConfigured = errors.New("cache: redis is not configured")

// ErrMiss is returned by Get when the key is absent (or Redis is absent —
// a missing cache never invents state).
var ErrMiss = errors.New("cache: miss")

// Client is the subset of go-redis used by the wrapper (fake-able in tests).
type Client interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// Cache wraps an optional Redis client with namespacing and TTL enforcement.
type Cache struct {
	client Client
}

// New wraps client; client may be nil when Redis is not configured.
func New(client Client) *Cache {
	return &Cache{client: client}
}

// Key builds the namespaced key.
func Key(parts ...string) string {
	key := Namespace
	for i, part := range parts {
		if i > 0 {
			key += ":"
		}
		key += part
	}
	return key
}

// Set stores a value with a mandatory positive TTL.
func (c *Cache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrTTLRequired
	}
	if c == nil || c.client == nil {
		return ErrNotConfigured
	}
	if err := c.client.Set(ctx, Key(key), value, ttl).Err(); err != nil {
		return fmt.Errorf("cache: set: %w", err)
	}
	return nil
}

// Get returns the value or ErrMiss. A degraded (nil) client always misses.
func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	if c == nil || c.client == nil {
		return "", ErrMiss
	}
	value, err := c.client.Get(ctx, Key(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrMiss
	}
	if err != nil {
		// Redis failure degrades to a miss; callers fall back to PostgreSQL.
		return "", ErrMiss
	}
	return value, nil
}

// Delete removes a key; degraded clients are a no-op.
func (c *Cache) Delete(ctx context.Context, key string) error {
	if c == nil || c.client == nil {
		return nil
	}
	if err := c.client.Del(ctx, Key(key)).Err(); err != nil {
		return fmt.Errorf("cache: delete: %w", err)
	}
	return nil
}
