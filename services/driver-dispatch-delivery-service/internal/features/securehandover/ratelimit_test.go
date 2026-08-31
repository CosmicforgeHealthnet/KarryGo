package securehandover

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisRateLimiterIsSharedBoundedAndWindowed(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	limiter := NewRedisRateLimiter(client)
	ctx := context.Background()
	if err := limiter.Allow(ctx, "verify:provider-a", 2, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := limiter.Allow(ctx, "verify:provider-a", 2, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := limiter.Allow(ctx, "verify:provider-a", 2, time.Minute); !errors.Is(err, ErrLocked) {
		t.Fatalf("third request error = %v", err)
	}
	if err := limiter.Allow(ctx, "verify:provider-b", 2, time.Minute); err != nil {
		t.Fatalf("principal scopes were not isolated: %v", err)
	}
	server.FastForward(time.Minute)
	if err := limiter.Allow(ctx, "verify:provider-a", 2, time.Minute); err != nil {
		t.Fatalf("new window remained locked: %v", err)
	}
}
