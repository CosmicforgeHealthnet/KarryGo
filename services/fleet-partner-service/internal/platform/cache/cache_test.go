package cache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// fakeClient records calls without any network access.
type fakeClient struct {
	store map[string]string
	keys  []string
}

func (f *fakeClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	if f.store == nil {
		f.store = map[string]string{}
	}
	f.store[key] = value.(string)
	f.keys = append(f.keys, key)
	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("OK")
	return cmd
}

func (f *fakeClient) Get(ctx context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx)
	if value, ok := f.store[key]; ok {
		cmd.SetVal(value)
	} else {
		cmd.SetErr(redis.Nil)
	}
	return cmd
}

func (f *fakeClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	for _, key := range keys {
		delete(f.store, key)
	}
	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(1)
	return cmd
}

func TestSetRequiresTTL(t *testing.T) {
	c := New(&fakeClient{})
	if err := c.Set(context.Background(), "k", "v", 0); !errors.Is(err, ErrTTLRequired) {
		t.Fatalf("zero TTL should be rejected, got %v", err)
	}
	if err := c.Set(context.Background(), "k", "v", -time.Second); !errors.Is(err, ErrTTLRequired) {
		t.Fatalf("negative TTL should be rejected, got %v", err)
	}
}

func TestEveryKeyIsNamespaced(t *testing.T) {
	fake := &fakeClient{}
	c := New(fake)
	if err := c.Set(context.Background(), "invite:abc", "v", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	_, _ = c.Get(context.Background(), "invite:abc")
	_ = c.Delete(context.Background(), "invite:abc")
	for _, key := range fake.keys {
		if !strings.HasPrefix(key, Namespace) {
			t.Fatalf("unnamespaced key reached redis: %s", key)
		}
	}
}

func TestRoundTripAndMiss(t *testing.T) {
	c := New(&fakeClient{})
	if err := c.Set(context.Background(), "k", "v", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	value, err := c.Get(context.Background(), "k")
	if err != nil || value != "v" {
		t.Fatalf("get = %q, %v", value, err)
	}
	if _, err := c.Get(context.Background(), "absent"); !errors.Is(err, ErrMiss) {
		t.Fatalf("absent key should miss, got %v", err)
	}
}

func TestDegradedWithoutRedis(t *testing.T) {
	// Redis outage or non-configuration never invents state and never panics.
	c := New(nil)
	if _, err := c.Get(context.Background(), "k"); !errors.Is(err, ErrMiss) {
		t.Fatalf("degraded get should miss, got %v", err)
	}
	if err := c.Set(context.Background(), "k", "v", time.Minute); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("degraded set should error explicitly, got %v", err)
	}
	if err := c.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("degraded delete should be a no-op, got %v", err)
	}
}
