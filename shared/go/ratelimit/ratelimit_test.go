package ratelimit

import (
	"strings"
	"testing"
)

func TestRedisKeyIsNamespacedDeterministicAndDoesNotExposePrincipal(t *testing.T) {
	limiter := New(nil, "karrygo:abuse")
	first := limiter.redisKey("quote", "customer@example.test")
	second := limiter.redisKey("quote", "customer@example.test")
	other := limiter.redisKey("search", "customer@example.test")
	if first != second || first == other {
		t.Fatalf("unexpected key stability/isolation: %q %q %q", first, second, other)
	}
	if strings.Contains(first, "customer@example.test") {
		t.Fatalf("rate-limit key exposes principal: %q", first)
	}
}
