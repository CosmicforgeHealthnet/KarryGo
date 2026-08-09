package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
)

// redisCounterForTest mirrors internal/app/wiring.go's unexported
// redisCounter adapter exactly (Incr + Expire in one pipeline, mandatory
// TTL on every key) -- duplicated here rather than exported from wiring.go
// solely to avoid widening that package's surface for a five-line test
// helper.
type redisCounterForTest struct {
	client *redis.Client
}

func (r redisCounterForTest) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// TestRateLimit_RedisBackedLimiterSharesStateAcrossTwoInstances proves
// Section 28/47's "distributed behavior across two instances" requirement
// against a real Redis: two SEPARATE ratelimit.RedisLimiter values (standing
// in for two separate fleet-partner-service process instances behind a load
// balancer) sharing the same Redis backend see each other's counts, so a
// caller cannot evade a limit merely by having consecutive requests land on
// different instances -- something ratelimit.MemoryLimiter (in-process,
// per-instance state) structurally cannot provide, which is exactly why
// Section 12 requires the distributed backend for a multi-instance
// deployment.
func TestRateLimit_RedisBackedLimiterSharesStateAcrossTwoInstances(t *testing.T) {
	addr := os.Getenv("FLEET_PARTNER_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("FLEET_PARTNER_TEST_REDIS_ADDR not set; skipping real-Redis rate-limit test")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}

	instanceA := ratelimit.NewRedisLimiter(redisCounterForTest{client: client})
	instanceB := ratelimit.NewRedisLimiter(redisCounterForTest{client: client})

	rule := ratelimit.Rule{Limit: 3, Window: time.Minute}
	scope := "risk:" + ratelimit.RiskHighRisk
	key := "org:redis-distributed-test-" + time.Now().Format("150405.000000000")

	ctx := context.Background()
	// Alternate between the two "instances" -- a real deployment would have
	// a load balancer doing this per-request, unpredictably.
	sequence := []*ratelimit.RedisLimiter{instanceA, instanceB, instanceA, instanceB}
	var results []bool
	for _, instance := range sequence {
		allowed, err := instance.Allow(ctx, scope, key, rule)
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		results = append(results, allowed)
	}

	want := []bool{true, true, true, false}
	for i, got := range results {
		if got != want[i] {
			t.Fatalf("request %d (instance %s) allowed=%v, want %v -- counters are not actually shared across instances: %v",
				i+1, map[int]string{0: "A", 1: "B", 2: "A", 3: "B"}[i], got, want[i], results)
		}
	}
}
