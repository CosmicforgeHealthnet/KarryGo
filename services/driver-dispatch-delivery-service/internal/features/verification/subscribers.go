package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/profile"
)

type OnboardingCompletedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	Phone         string    `json:"phone"`
	OperationType string    `json:"operation_type"`
	CreatedAt     time.Time `json:"created_at"`
}

const (
	TopicVehicleRegistered = "vehicle.registered"
	TopicVehicleVerified   = "vehicle.verified"
	TopicVehicleRejected   = "vehicle.rejected"
)

type VehicleRegisteredEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	BikeID        string    `json:"bike_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type VehicleVerifiedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id"`
	ProviderID    string    `json:"provider_id"`
	BikeID        string    `json:"bike_id"`
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

func StartSubscribers(ctx context.Context, client *redis.Client, repository Repository) {
	if client == nil || repository == nil {
		log.Printf("verification subscribers: not started because redis client or repository is nil")
		return
	}

	log.Printf("verification subscribers: starting %s subscriber", profile.TopicOnboardingCompleted)
	subscribeOnboardingCompleted(ctx, client, repository)
	log.Printf("verification subscribers: starting %s subscriber", TopicVehicleRegistered)
	subscribeVehicleRegistered(ctx, client, repository)
	log.Printf("verification subscribers: starting %s subscriber", TopicVehicleVerified)
	subscribeVehicleVerified(ctx, client, repository)
	log.Printf("verification subscribers: starting %s subscriber", TopicVehicleRejected)
	subscribeVehicleRejected(ctx, client, repository)
}

func subscribeVehicleRegistered(ctx context.Context, client *redis.Client, repository Repository) {
	subscribe(ctx, client, TopicVehicleRegistered, func(ctx context.Context, payload []byte) error {
		return HandleVehicleRegisteredPayload(ctx, repository, payload)
	})
}

func HandleVehicleRegisteredPayload(ctx context.Context, repository Repository, payload []byte) error {
	var event VehicleRegisteredEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshal vehicle_registered: %w", err)
	}
	service := NewService(repository, NewStubSmileIdentityClient())
	if err := service.ApplyVehicleRegistered(ctx, event); err != nil {
		return fmt.Errorf("handle vehicle registered: %w", err)
	}
	log.Printf("verification subscriber %s: handled provider_id=%s bike_id=%s", TopicVehicleRegistered, event.ProviderID, event.BikeID)
	return nil
}

func subscribeOnboardingCompleted(ctx context.Context, client *redis.Client, repository Repository) {
	subscribe(ctx, client, profile.TopicOnboardingCompleted, func(ctx context.Context, payload []byte) error {
		return HandleOnboardingCompletedPayload(ctx, repository, payload)
	})
}

func subscribeVehicleVerified(ctx context.Context, client *redis.Client, repository Repository) {
	subscribe(ctx, client, TopicVehicleVerified, func(ctx context.Context, payload []byte) error {
		return HandleVehicleVerifiedPayload(ctx, repository, payload)
	})
}

func subscribeVehicleRejected(ctx context.Context, client *redis.Client, repository Repository) {
	subscribe(ctx, client, TopicVehicleRejected, func(ctx context.Context, payload []byte) error {
		return HandleVehicleRejectedPayload(ctx, repository, payload)
	})
}

func subscribe(ctx context.Context, client *redis.Client, topic string, handle func(context.Context, []byte) error) {
	go func() {
		sub := client.Subscribe(ctx, topic)
		defer sub.Close()

		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				log.Printf("verification subscriber %s: received payload=%s", topic, msg.Payload)
				msgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				if err := handle(msgCtx, []byte(msg.Payload)); err != nil {
					log.Printf("verification subscriber %s: %v", topic, err)
				}
				cancel()
			}
		}
	}()
}

func HandleOnboardingCompletedPayload(ctx context.Context, repository Repository, payload []byte) error {
	var event OnboardingCompletedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshal onboarding_completed: %w", err)
	}
	if err := validateProviderID(event.ProviderID); err != nil {
		return err
	}

	service := NewService(repository, NewStubSmileIdentityClient())
	result, err := service.InitializeForCompletedOnboarding(ctx, event.ProviderID)
	if err != nil {
		return fmt.Errorf("initialize verification steps: %w", err)
	}
	log.Printf("verification subscriber %s: initialized provider_id=%s inserted_steps=%d auto_confirmed_audit_rows=%d", profile.TopicOnboardingCompleted, event.ProviderID, result.InsertedSteps, result.AutoConfirmedAuditRows)
	return nil
}

func HandleVehicleVerifiedPayload(ctx context.Context, repository Repository, payload []byte) error {
	var event VehicleVerifiedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshal vehicle_verified: %w", err)
	}
	service := NewService(repository, NewStubSmileIdentityClient())
	if err := service.ApplyVehicleVerified(ctx, event); err != nil {
		return fmt.Errorf("handle vehicle verified: %w", err)
	}
	log.Printf("verification subscriber %s: handled provider_id=%s bike_id=%s", TopicVehicleVerified, event.ProviderID, event.BikeID)
	return nil
}

func HandleVehicleRejectedPayload(ctx context.Context, repository Repository, payload []byte) error {
	var event VehicleRejectedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshal vehicle_rejected: %w", err)
	}
	service := NewService(repository, NewStubSmileIdentityClient())
	if err := service.ApplyVehicleRejected(ctx, event); err != nil {
		return fmt.Errorf("handle vehicle rejected: %w", err)
	}
	log.Printf("verification subscriber %s: handled provider_id=%s bike_id=%s", TopicVehicleRejected, event.ProviderID, event.BikeID)
	return nil
}
