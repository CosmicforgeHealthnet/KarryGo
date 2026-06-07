package authclients

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// TopicOTPRequested is the Redis pub/sub channel for OTP-requested events.
// Consumed by notification-service to deliver SMS to the dispatch rider.
const TopicOTPRequested = "provider.auth.otp_requested"

// TopicSessionCreated is published after a dispatch rider session is successfully created.
const TopicSessionCreated = "provider.auth.session_created"

// TopicLoggedOut is published after a dispatch rider session is revoked.
const TopicLoggedOut = "provider.auth.logged_out"

// TopicProfileSuspended is the future event published by profile/admin service when
// a provider is suspended. The auth feature will subscribe and update identity status.
// TODO(Phase-2): subscribe to this topic and call IdentityRepository.UpdateStatus to
// set status = suspended when received. Not implemented until profile suspension flow is live.
const TopicProfileSuspended = "provider.profile.suspended"

// OTPRequestedEvent is the payload published after a dispatch rider OTP is created.
//
// otp_code is included so that notification-service can embed it in the SMS body.
// This value must NEVER be logged in production and must NEVER appear in HTTP responses.
type OTPRequestedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	PhoneNumber   string    `json:"phone_number"`
	OTPCode       string    `json:"otp_code"`
	Purpose       string    `json:"purpose"`
	ExpiresIn     int       `json:"expires_in_seconds"`
	CreatedAt     time.Time `json:"created_at"`
}

// SessionCreatedEvent is published after a dispatch provider session is created.
// role is always "dispatch_provider". Does not include tokens.
type SessionCreatedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	PhoneNumber   string    `json:"phone_number"`
	Role          string    `json:"role"`
	SessionID     string    `json:"session_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// LoggedOutEvent is published after a dispatch rider session is revoked.
// It does not include access or refresh token values.
type LoggedOutEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	SessionID     string    `json:"session_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// EventPublisher publishes auth domain events to an external message bus.
type EventPublisher interface {
	PublishOTPRequested(ctx context.Context, event OTPRequestedEvent) error
	PublishSessionCreated(ctx context.Context, event SessionCreatedEvent) error
	PublishLoggedOut(ctx context.Context, event LoggedOutEvent) error
}

// RedisEventPublisher publishes events to Redis Pub/Sub.
//
// If notification-service is not subscribed when an event is published,
// Redis silently drops it. This is acceptable during Phase 1D/1E when
// notification-service may not yet be deployed.
type RedisEventPublisher struct {
	client *redis.Client
}

// NewRedisEventPublisher creates a RedisEventPublisher backed by the given client.
func NewRedisEventPublisher(client *redis.Client) *RedisEventPublisher {
	return &RedisEventPublisher{client: client}
}

// PublishOTPRequested serialises the event and publishes it to TopicOTPRequested.
func (p *RedisEventPublisher) PublishOTPRequested(ctx context.Context, event OTPRequestedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal otp_requested event: %w", err)
	}
	return p.client.Publish(ctx, TopicOTPRequested, payload).Err()
}

// PublishSessionCreated serialises the event and publishes it to TopicSessionCreated.
func (p *RedisEventPublisher) PublishSessionCreated(ctx context.Context, event SessionCreatedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal session_created event: %w", err)
	}
	return p.client.Publish(ctx, TopicSessionCreated, payload).Err()
}

// PublishLoggedOut serialises the event and publishes it to TopicLoggedOut.
func (p *RedisEventPublisher) PublishLoggedOut(ctx context.Context, event LoggedOutEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal logged_out event: %w", err)
	}
	return p.client.Publish(ctx, TopicLoggedOut, payload).Err()
}

func SubscribeProfileSuspended(ctx context.Context, client *redis.Client) {
	// TODO: parse provider.profile.suspended payload and update identity status to suspended.
	// Not implemented yet because profile/admin suspension flow is not live.
	_ = ctx
	_ = client
}
