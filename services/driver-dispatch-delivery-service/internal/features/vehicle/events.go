package vehicle

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// ── Topics ─────────────────────────────────────────────────────────────────────

// TopicVehicleRegistered is published when a new bike record is created.
const TopicVehicleRegistered = "vehicle.registered"

// TopicVehicleDocsSubmitted is published when a provider uploads a bike document.
const TopicVehicleDocsSubmitted = "vehicle.documents.submitted"

// TopicVehicleVerified is consumed by the verification feature to mark the vehicle
// verification step as approved.
const TopicVehicleVerified = "vehicle.verified"

// TopicVehicleRejected is consumed by the verification feature to mark the vehicle
// verification step as rejected.
const TopicVehicleRejected = "vehicle.rejected"

// TopicVehicleSuspended is published when an admin suspends a bike.
// Consumed by the availability service to force the provider offline.
const TopicVehicleSuspended = "vehicle.suspended"

// TopicProviderVerificationSuspended is published by the verification feature when
// an entire provider account is suspended. This service subscribes to deactivate
// all bikes for that provider.
const TopicProviderVerificationSuspended = "provider.verification.suspended"

// ── Event payloads ─────────────────────────────────────────────────────────────

type VehicleRegisteredEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	BikeID        string    `json:"bike_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type VehicleDocsSubmittedEvent struct {
	Event         string       `json:"event"`
	CorrelationID string       `json:"correlation_id"`
	ProviderID    string       `json:"provider_id"`
	BikeID        string       `json:"bike_id"`
	DocumentType  DocumentType `json:"document_type"`
	CreatedAt     time.Time    `json:"created_at"`
}

type VehicleVerifiedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	BikeID        string    `json:"bike_id"`
	VerifiedAt    time.Time `json:"verified_at"`
	CreatedAt     time.Time `json:"created_at"`
}

type VehicleRejectedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	BikeID        string    `json:"bike_id"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

type VehicleSuspendedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	BikeID        string    `json:"bike_id"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

// ProviderVerificationSuspendedEvent is published by the verification feature
// when an entire provider account is suspended.
type ProviderVerificationSuspendedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

// ── Publisher interface ────────────────────────────────────────────────────────

// EventPublisher publishes vehicle domain events.
type EventPublisher interface {
	PublishVehicleRegistered(ctx context.Context, event VehicleRegisteredEvent) error
	PublishVehicleDocsSubmitted(ctx context.Context, event VehicleDocsSubmittedEvent) error
	PublishVehicleVerified(ctx context.Context, event VehicleVerifiedEvent) error
	PublishVehicleRejected(ctx context.Context, event VehicleRejectedEvent) error
	PublishVehicleSuspended(ctx context.Context, event VehicleSuspendedEvent) error
}

// ── Redis publisher ────────────────────────────────────────────────────────────

// RedisEventPublisher publishes vehicle events to Redis Pub/Sub.
type RedisEventPublisher struct {
	client *redis.Client
}

func NewRedisEventPublisher(client *redis.Client) *RedisEventPublisher {
	return &RedisEventPublisher{client: client}
}

func (p *RedisEventPublisher) PublishVehicleRegistered(ctx context.Context, event VehicleRegisteredEvent) error {
	return p.publish(ctx, TopicVehicleRegistered, event)
}

func (p *RedisEventPublisher) PublishVehicleDocsSubmitted(ctx context.Context, event VehicleDocsSubmittedEvent) error {
	return p.publish(ctx, TopicVehicleDocsSubmitted, event)
}

func (p *RedisEventPublisher) PublishVehicleVerified(ctx context.Context, event VehicleVerifiedEvent) error {
	return p.publish(ctx, TopicVehicleVerified, event)
}

func (p *RedisEventPublisher) PublishVehicleRejected(ctx context.Context, event VehicleRejectedEvent) error {
	return p.publish(ctx, TopicVehicleRejected, event)
}

func (p *RedisEventPublisher) PublishVehicleSuspended(ctx context.Context, event VehicleSuspendedEvent) error {
	return p.publish(ctx, TopicVehicleSuspended, event)
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
		log.Printf("vehicle publisher %s publish failed error=%v", topic, err)
		return err
	}
	log.Printf("vehicle publisher %s published subscribers=%d", topic, cmd.Val())
	return nil
}
