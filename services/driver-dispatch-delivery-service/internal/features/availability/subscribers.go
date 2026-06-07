package availability

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"karrygo/services/driver-dispatch-delivery-service/internal/features/vehicle"
	"karrygo/services/driver-dispatch-delivery-service/internal/features/verification"
)

func StartSubscribers(ctx context.Context, client *redis.Client, repository Repository, live LiveStore) {
	if client == nil || repository == nil {
		log.Printf("availability subscribers: not started because redis client or repository is nil")
		return
	}
	service := NewService(repository, live)

	log.Printf("availability subscribers: starting %s subscriber", verification.TopicVerificationFullyApproved)
	subscribeAvailability(ctx, client, verification.TopicVerificationFullyApproved, func(ctx context.Context, payload []byte) error {
		return HandleVerificationFullyApprovedPayload(ctx, service, payload)
	})
	log.Printf("availability subscribers: starting %s subscriber", vehicle.TopicProviderVerificationSuspended)
	subscribeAvailability(ctx, client, vehicle.TopicProviderVerificationSuspended, func(ctx context.Context, payload []byte) error {
		return HandleProviderVerificationSuspendedPayload(ctx, service, payload)
	})
	log.Printf("availability subscribers: starting %s subscriber", vehicle.TopicVehicleSuspended)
	subscribeAvailability(ctx, client, vehicle.TopicVehicleSuspended, func(ctx context.Context, payload []byte) error {
		return HandleVehicleSuspendedPayload(ctx, service, payload)
	})
	log.Printf("availability subscribers: starting %s subscriber", vehicle.TopicVehicleRejected)
	subscribeAvailability(ctx, client, vehicle.TopicVehicleRejected, func(ctx context.Context, payload []byte) error {
		return HandleVehicleRejectedPayload(ctx, service, payload)
	})
	log.Printf("availability subscribers: starting %s subscriber", vehicle.TopicVehicleVerified)
	subscribeAvailability(ctx, client, vehicle.TopicVehicleVerified, func(ctx context.Context, payload []byte) error {
		return HandleVehicleVerifiedPayload(ctx, repository, payload)
	})

	// Trip event subscribers — published by Phase 6 trip-service.
	log.Printf("availability subscribers: starting %s subscriber", TopicTripStarted)
	subscribeAvailability(ctx, client, TopicTripStarted, func(ctx context.Context, payload []byte) error {
		return HandleTripStartedPayload(ctx, service, payload)
	})
	log.Printf("availability subscribers: starting %s subscriber", TopicTripCompleted)
	subscribeAvailability(ctx, client, TopicTripCompleted, func(ctx context.Context, payload []byte) error {
		return HandleTripCompletedPayload(ctx, service, payload)
	})
	log.Printf("availability subscribers: starting %s subscriber", TopicTripCancelled)
	subscribeAvailability(ctx, client, TopicTripCancelled, func(ctx context.Context, payload []byte) error {
		return HandleTripCancelledPayload(ctx, service, payload)
	})
}

func HandleVerificationFullyApprovedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event verification.VerificationFullyApprovedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("availability subscriber %s: invalid payload: %v - skipping", verification.TopicVerificationFullyApproved, err)
		return nil
	}
	providerID := strings.TrimSpace(event.ProviderID)
	if err := validateProviderID(providerID); err != nil {
		log.Printf("availability subscriber %s: invalid provider_id=%q - skipping", verification.TopicVerificationFullyApproved, event.ProviderID)
		return nil
	}
	if err := service.UnlockVerifiedToGoOnline(ctx, providerID); err != nil {
		return err
	}
	log.Printf("availability subscriber %s: unlocked provider_id=%s", verification.TopicVerificationFullyApproved, providerID)
	return nil
}

func HandleProviderVerificationSuspendedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event vehicle.ProviderVerificationSuspendedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("availability subscriber %s: invalid payload: %v - skipping", vehicle.TopicProviderVerificationSuspended, err)
		return nil
	}
	providerID := strings.TrimSpace(event.ProviderID)
	if err := validateProviderID(providerID); err != nil {
		log.Printf("availability subscriber %s: invalid provider_id=%q - skipping", vehicle.TopicProviderVerificationSuspended, event.ProviderID)
		return nil
	}
	if err := service.ForceOffline(ctx, providerID, true, true); err != nil {
		return err
	}
	log.Printf("availability subscriber %s: forced offline provider_id=%s", vehicle.TopicProviderVerificationSuspended, providerID)
	return nil
}

func HandleVehicleSuspendedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event vehicle.VehicleSuspendedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("availability subscriber %s: invalid payload: %v - skipping", vehicle.TopicVehicleSuspended, err)
		return nil
	}
	providerID := strings.TrimSpace(event.ProviderID)
	if err := validateProviderID(providerID); err != nil {
		log.Printf("availability subscriber %s: invalid provider_id=%q - skipping", vehicle.TopicVehicleSuspended, event.ProviderID)
		return nil
	}
	if err := service.ForceOfflineIfNoVerifiedActiveBike(ctx, providerID); err != nil {
		return err
	}
	log.Printf("availability subscriber %s: handled provider_id=%s bike_id=%s", vehicle.TopicVehicleSuspended, providerID, event.BikeID)
	return nil
}

func HandleVehicleRejectedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event vehicle.VehicleRejectedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("availability subscriber %s: invalid payload: %v - skipping", vehicle.TopicVehicleRejected, err)
		return nil
	}
	providerID := strings.TrimSpace(event.ProviderID)
	if err := validateProviderID(providerID); err != nil {
		log.Printf("availability subscriber %s: invalid provider_id=%q - skipping", vehicle.TopicVehicleRejected, event.ProviderID)
		return nil
	}
	if err := service.ForceOfflineIfNoVerifiedActiveBike(ctx, providerID); err != nil {
		return err
	}
	log.Printf("availability subscriber %s: handled provider_id=%s bike_id=%s", vehicle.TopicVehicleRejected, providerID, event.BikeID)
	return nil
}

func HandleVehicleVerifiedPayload(ctx context.Context, repository Repository, payload []byte) error {
	var event vehicle.VehicleVerifiedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("availability subscriber %s: invalid payload: %v - skipping", vehicle.TopicVehicleVerified, err)
		return nil
	}
	providerID := strings.TrimSpace(event.ProviderID)
	if err := validateProviderID(providerID); err != nil {
		log.Printf("availability subscriber %s: invalid provider_id=%q - skipping", vehicle.TopicVehicleVerified, event.ProviderID)
		return nil
	}
	if _, err := repository.EnsureAvailability(ctx, providerID); err != nil {
		return err
	}
	log.Printf("availability subscriber %s: handled provider_id=%s bike_id=%s", vehicle.TopicVehicleVerified, providerID, event.BikeID)
	return nil
}

func HandleTripStartedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event TripStartedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("availability subscriber %s: invalid payload: %v - skipping", TopicTripStarted, err)
		return nil
	}
	providerID := strings.TrimSpace(event.ProviderID)
	if err := validateProviderID(providerID); err != nil {
		log.Printf("availability subscriber %s: invalid provider_id=%q - skipping", TopicTripStarted, event.ProviderID)
		return nil
	}
	if err := service.SetBusy(ctx, providerID); err != nil {
		return err
	}
	log.Printf("availability subscriber %s: set busy provider_id=%s trip_id=%s", TopicTripStarted, providerID, event.TripID)
	return nil
}

func HandleTripCompletedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event TripCompletedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("availability subscriber %s: invalid payload: %v - skipping", TopicTripCompleted, err)
		return nil
	}
	providerID := strings.TrimSpace(event.ProviderID)
	if err := validateProviderID(providerID); err != nil {
		log.Printf("availability subscriber %s: invalid provider_id=%q - skipping", TopicTripCompleted, event.ProviderID)
		return nil
	}
	if err := service.ReturnFromTrip(ctx, providerID, true); err != nil {
		return err
	}
	log.Printf("availability subscriber %s: returned online provider_id=%s trip_id=%s trips_incremented=true", TopicTripCompleted, providerID, event.TripID)
	return nil
}

func HandleTripCancelledPayload(ctx context.Context, service *Service, payload []byte) error {
	var event TripCancelledEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("availability subscriber %s: invalid payload: %v - skipping", TopicTripCancelled, err)
		return nil
	}
	providerID := strings.TrimSpace(event.ProviderID)
	if err := validateProviderID(providerID); err != nil {
		log.Printf("availability subscriber %s: invalid provider_id=%q - skipping", TopicTripCancelled, event.ProviderID)
		return nil
	}
	if err := service.ReturnFromTrip(ctx, providerID, false); err != nil {
		return err
	}
	log.Printf("availability subscriber %s: returned online provider_id=%s trip_id=%s trips_incremented=false", TopicTripCancelled, providerID, event.TripID)
	return nil
}

func subscribeAvailability(ctx context.Context, client *redis.Client, topic string, handle func(context.Context, []byte) error) {
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
				log.Printf("availability subscriber %s: received payload=%s", topic, msg.Payload)
				msgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				if err := handle(msgCtx, []byte(msg.Payload)); err != nil {
					log.Printf("availability subscriber %s: handler error: %v", topic, err)
				}
				cancel()
			}
		}
	}()
}
