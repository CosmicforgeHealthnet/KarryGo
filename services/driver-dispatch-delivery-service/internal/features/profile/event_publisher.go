package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const TopicProfileUpdated = "provider.profile.updated"
const TopicOnboardingCompleted = "provider.onboarding.completed"

type ProfileUpdatedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	ChangedFields []string  `json:"changed_fields"`
	CreatedAt     time.Time `json:"created_at"`
}

type OnboardingCompletedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	Phone         string    `json:"phone"`
	OperationType string    `json:"operation_type"`
	CreatedAt     time.Time `json:"created_at"`
}

type ProfileEventPublisher interface {
	PublishProfileUpdated(ctx context.Context, event ProfileUpdatedEvent) error
	PublishOnboardingCompleted(ctx context.Context, event OnboardingCompletedEvent) error
}

type RedisProfileEventPublisher struct {
	client *redis.Client
}

func NewRedisProfileEventPublisher(client *redis.Client) *RedisProfileEventPublisher {
	return &RedisProfileEventPublisher{client: client}
}

func (p *RedisProfileEventPublisher) PublishProfileUpdated(ctx context.Context, event ProfileUpdatedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal profile_updated event: %w", err)
	}
	return p.client.Publish(ctx, TopicProfileUpdated, payload).Err()
}

func (p *RedisProfileEventPublisher) PublishOnboardingCompleted(ctx context.Context, event OnboardingCompletedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal onboarding_completed event: %w", err)
	}
	cmd := p.client.Publish(ctx, TopicOnboardingCompleted, payload)
	if err := cmd.Err(); err != nil {
		log.Printf("profile publisher %s publish failed provider_id=%s correlation_id=%s error=%v", TopicOnboardingCompleted, event.ProviderID, event.CorrelationID, err)
		return err
	}
	log.Printf("profile publisher %s published provider_id=%s correlation_id=%s subscribers=%d", TopicOnboardingCompleted, event.ProviderID, event.CorrelationID, cmd.Val())
	return nil
}
