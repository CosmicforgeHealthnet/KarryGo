package verification

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TAXI_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TAXI_TEST_REDIS_ADDR is required for pickup verify rate limiter tests")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestRateLimiter_AllowsUpToMaxThenBlocks(t *testing.T) {
	client := testRedis(t)
	limiter := NewRateLimiter(client)
	bookingID := uuid.NewString()
	t.Cleanup(func() { _ = client.Del(context.Background(), rateKey(bookingID)).Err() })

	for i := 1; i <= 3; i++ {
		allowed, err := limiter.Allow(context.Background(), bookingID, time.Minute, 3)
		if err != nil {
			t.Fatalf("Allow[%d]: %v", i, err)
		}
		if !allowed {
			t.Fatalf("attempt %d should be allowed (max is 3)", i)
		}
	}

	allowed, err := limiter.Allow(context.Background(), bookingID, time.Minute, 3)
	if err != nil {
		t.Fatalf("Allow[4]: %v", err)
	}
	if allowed {
		t.Fatal("the 4th attempt should be blocked once the max of 3 is reached")
	}
}

func TestRateLimiter_DifferentBookingsHaveIndependentWindows(t *testing.T) {
	client := testRedis(t)
	limiter := NewRateLimiter(client)
	bookingA := uuid.NewString()
	bookingB := uuid.NewString()
	t.Cleanup(func() {
		_ = client.Del(context.Background(), rateKey(bookingA), rateKey(bookingB)).Err()
	})

	for i := 0; i < 5; i++ {
		if _, err := limiter.Allow(context.Background(), bookingA, time.Minute, 1); err != nil {
			t.Fatalf("Allow(A): %v", err)
		}
	}

	allowed, err := limiter.Allow(context.Background(), bookingB, time.Minute, 1)
	if err != nil {
		t.Fatalf("Allow(B): %v", err)
	}
	if !allowed {
		t.Fatal("booking B's first attempt should be allowed regardless of booking A's exhausted window")
	}
}

func TestRateLimiter_KeyExpiresAfterWindow(t *testing.T) {
	client := testRedis(t)
	limiter := NewRateLimiter(client)
	bookingID := uuid.NewString()
	t.Cleanup(func() { _ = client.Del(context.Background(), rateKey(bookingID)).Err() })

	if _, err := limiter.Allow(context.Background(), bookingID, 2*time.Second, 1); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	ttl, err := client.TTL(context.Background(), rateKey(bookingID)).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > 2*time.Second {
		t.Errorf("TTL = %v, want a positive value at or under the 2s window", ttl)
	}
}
