package securehandover

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestRealRedisHandoverLimitAcrossClients(t *testing.T) {
	addr := os.Getenv("DISPATCH_ADMIN_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("DISPATCH_ADMIN_TEST_REDIS_ADDR required for real Redis certification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a := redis.NewClient(&redis.Options{Addr: addr})
	b := redis.NewClient(&redis.Options{Addr: addr})
	defer a.Close()
	defer b.Close()
	if err := a.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	key := "verify:admin-certification:" + uuid.NewString()
	limiters := []*RedisRateLimiter{NewRedisRateLimiter(a), NewRedisRateLimiter(b)}
	outcomes := make(chan error, 40)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(n int) { defer wg.Done(); outcomes <- limiters[n%2].Allow(ctx, key, 5, time.Minute) }(i)
	}
	wg.Wait()
	close(outcomes)
	allowed, locked := 0, 0
	for err := range outcomes {
		if err == nil {
			allowed++
		} else if errors.Is(err, ErrLocked) {
			locked++
		} else {
			t.Fatal(err)
		}
	}
	if allowed != 5 || locked != 35 {
		t.Fatalf("shared allowance=%d locked=%d", allowed, locked)
	}
	// Closing one replica's connection must not reset the shared allowance.
	a.Close()
	if err := limiters[1].Allow(ctx, key, 5, time.Minute); !errors.Is(err, ErrLocked) {
		t.Fatalf("replica restart reset allowance: %v", err)
	}
}
