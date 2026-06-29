package trip

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

// RedisEventPublisher publishes outbound trip lifecycle events over Redis pub/sub (Phase 7F–7H).
type RedisEventPublisher struct {
	client *redis.Client
}

func NewRedisEventPublisher(client *redis.Client) *RedisEventPublisher {
	return &RedisEventPublisher{client: client}
}

func (p *RedisEventPublisher) PublishTripCreated(ctx context.Context, event TripCreatedEvent) error {
	return p.publish(ctx, TopicTripCreated, event)
}

func (p *RedisEventPublisher) PublishProviderArrived(ctx context.Context, event TripProviderArrivedEvent) error {
	return p.publish(ctx, TopicTripProviderArrived, event)
}

func (p *RedisEventPublisher) PublishTripStarted(ctx context.Context, event TripStartedEvent) error {
	return p.publish(ctx, TopicTripStarted, event)
}

func (p *RedisEventPublisher) PublishProofSubmitted(ctx context.Context, event TripProofSubmittedEvent) error {
	return p.publish(ctx, TopicTripProofSubmitted, event)
}

func (p *RedisEventPublisher) PublishTripCompleted(ctx context.Context, event TripCompletedEvent) error {
	return p.publish(ctx, TopicTripCompleted, event)
}

func (p *RedisEventPublisher) PublishTripCancelled(ctx context.Context, event TripCancelledEvent) error {
	return p.publish(ctx, TopicTripCancelled, event)
}

func (p *RedisEventPublisher) PublishSuspensionFlag(ctx context.Context, event SuspensionFlagEvent) error {
	return p.publish(ctx, TopicVerificationSuspension, event)
}

func (p *RedisEventPublisher) PublishCustomerRated(ctx context.Context, event CustomerRatedEvent) error {
	return p.publish(ctx, TopicCustomerRated, event)
}

func (p *RedisEventPublisher) publish(ctx context.Context, topic string, payload any) error {
	if p == nil || p.client == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, topic, data).Err()
}
