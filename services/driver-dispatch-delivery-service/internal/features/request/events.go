package request

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

type RedisEventPublisher struct {
	client *redis.Client
}

func NewRedisEventPublisher(client *redis.Client) *RedisEventPublisher {
	return &RedisEventPublisher{client: client}
}

func (p *RedisEventPublisher) PublishRequestAccepted(ctx context.Context, event RequestAcceptedEvent) error {
	return p.publish(ctx, TopicRequestAccepted, event)
}

func (p *RedisEventPublisher) PublishRequestRejected(ctx context.Context, event RequestRejectedEvent) error {
	return p.publish(ctx, TopicRequestRejected, event)
}

func (p *RedisEventPublisher) PublishNoProviderFound(ctx context.Context, event NoProviderFoundEvent) error {
	return p.publish(ctx, TopicNoProviderFound, event)
}

func (p *RedisEventPublisher) publish(ctx context.Context, topic string, event any) error {
	if p == nil || p.client == nil {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, topic, payload).Err()
}
