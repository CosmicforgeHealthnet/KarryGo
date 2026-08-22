package searchprojection

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisSearchCache struct {
	client redis.UniversalClient
}

func NewRedisSearchCache(
	client redis.UniversalClient,
) *RedisSearchCache {
	return &RedisSearchCache{
		client: client,
	}
}

func (
	cache *RedisSearchCache,
) Get(
	ctx context.Context,
	key string,
) (
	[]byte,
	bool,
	error,
) {
	if cache == nil ||
		cache.client == nil ||
		key == "" {
		return nil,
			false,
			ErrInvalidArgument
	}

	value, err :=
		cache.client.Get(
			ctx,
			key,
		).Bytes()

	if err == redis.Nil {
		return nil,
			false,
			nil
	}

	if err != nil {
		return nil,
			false,
			err
	}

	return value,
		true,
		nil
}

func (
	cache *RedisSearchCache,
) Set(
	ctx context.Context,
	key string,
	value []byte,
	ttl time.Duration,
) error {
	if cache == nil ||
		cache.client == nil ||
		key == "" ||
		ttl < time.Second ||
		ttl > 15*time.Minute {
		return ErrInvalidArgument
	}

	return cache.client.Set(
		ctx,
		key,
		value,
		ttl,
	).Err()
}

func (
	cache *RedisSearchCache,
) Delete(
	ctx context.Context,
	keys ...string,
) error {
	if cache == nil ||
		cache.client == nil {
		return ErrInvalidArgument
	}

	if len(keys) == 0 {
		return nil
	}

	if len(keys) >
		maxTargetedCacheInvalidationKeys {
		return ErrInvalidArgument
	}

	return cache.client.Del(
		ctx,
		keys...,
	).Err()
}

var _ SearchCache = (*RedisSearchCache)(nil)
