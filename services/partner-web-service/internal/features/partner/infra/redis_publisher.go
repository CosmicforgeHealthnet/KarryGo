package infra

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// RedisPublisher implements delivery.Publisher over Redis pub/sub, matching the
// dispatch service's client.Subscribe consumers.
type RedisPublisher struct {
	client *redis.Client
}

// NewRedisPublisher constructs a RedisPublisher.
func NewRedisPublisher(client *redis.Client) *RedisPublisher { return &RedisPublisher{client: client} }

func (p *RedisPublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	return p.client.Publish(ctx, topic, payload).Err()
}
