package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	TopicVerificationStepSubmitted = "verification.step.submitted"
	TopicVerificationFaceFailed    = "verification.face.failed"
	TopicVerificationStatusUpdated = "verification.status.updated"
	TopicVerificationFullyApproved = "verification.fully_approved"
	TopicVerificationRejected      = "verification.rejected"
	// TopicVerificationSuspended is reserved for a future admin suspension flow.
	TopicVerificationSuspended = "verification.suspended"
)

type StepSubmittedEvent struct {
	Event         string     `json:"event"`
	CorrelationID string     `json:"correlation_id"`
	ProviderID    string     `json:"provider_id"`
	Step          Step       `json:"step"`
	Status        StepStatus `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
}

type FaceFailedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	Step          Step      `json:"step"`
	Result        string    `json:"result"`
	MatchScore    float64   `json:"match_score"`
	CreatedAt     time.Time `json:"created_at"`
}

// VerificationStatusUpdatedEvent is published on every admin approve/reject action
// for step-level status changes.  It is also published with a provider-level
// VerificationStatus ("verified") when the provider becomes fully approved so that
// the profile mirror subscriber can update providers.verification_status.
type VerificationStatusUpdatedEvent struct {
	Event              string     `json:"event"`
	CorrelationID      string     `json:"correlation_id"`
	ProviderID         string     `json:"provider_id"`
	Step               Step       `json:"step,omitempty"`
	Status             StepStatus `json:"status,omitempty"`
	VerificationStatus string     `json:"verification_status,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

type VerificationFullyApprovedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	ApprovedAt    time.Time `json:"approved_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// VerificationSuspendedPayload is reserved for a future admin suspension flow.
// TODO: implement PublishVerificationSuspended when admin suspension endpoint is built.
type VerificationSuspendedPayload struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

type VerificationRejectedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	Step          Step      `json:"step"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

type EventPublisher interface {
	PublishStepSubmitted(ctx context.Context, event StepSubmittedEvent) error
	PublishFaceFailed(ctx context.Context, event FaceFailedEvent) error
	PublishVerificationStatusUpdated(ctx context.Context, event VerificationStatusUpdatedEvent) error
	PublishVerificationFullyApproved(ctx context.Context, event VerificationFullyApprovedEvent) error
	PublishVerificationRejected(ctx context.Context, event VerificationRejectedEvent) error
}

type RedisEventPublisher struct {
	client *redis.Client
}

func NewRedisEventPublisher(client *redis.Client) *RedisEventPublisher {
	return &RedisEventPublisher{client: client}
}

func (p *RedisEventPublisher) PublishStepSubmitted(ctx context.Context, event StepSubmittedEvent) error {
	return p.publish(ctx, TopicVerificationStepSubmitted, event)
}

func (p *RedisEventPublisher) PublishFaceFailed(ctx context.Context, event FaceFailedEvent) error {
	return p.publish(ctx, TopicVerificationFaceFailed, event)
}

func (p *RedisEventPublisher) PublishVerificationStatusUpdated(ctx context.Context, event VerificationStatusUpdatedEvent) error {
	return p.publish(ctx, TopicVerificationStatusUpdated, event)
}

func (p *RedisEventPublisher) PublishVerificationFullyApproved(ctx context.Context, event VerificationFullyApprovedEvent) error {
	return p.publish(ctx, TopicVerificationFullyApproved, event)
}

func (p *RedisEventPublisher) PublishVerificationRejected(ctx context.Context, event VerificationRejectedEvent) error {
	return p.publish(ctx, TopicVerificationRejected, event)
}

func (p *RedisEventPublisher) publish(ctx context.Context, topic string, event any) error {
	if p == nil || p.client == nil {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", topic, err)
	}
	cmd := p.client.Publish(ctx, topic, payload)
	if err := cmd.Err(); err != nil {
		log.Printf("verification publisher %s publish failed error=%v", topic, err)
		return err
	}
	log.Printf("verification publisher %s published subscribers=%d", topic, cmd.Val())
	return nil
}
