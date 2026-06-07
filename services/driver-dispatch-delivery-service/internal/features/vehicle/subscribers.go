package vehicle

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// StartSubscribers starts all vehicle event subscribers. Safe to call with nil
// client or repository — it will log and return without panicking.
func StartSubscribers(ctx context.Context, client *redis.Client, repository Repository) {
	if client == nil || repository == nil {
		log.Printf("vehicle subscribers: not started because redis client or repository is nil")
		return
	}
	log.Printf("vehicle subscribers: starting %s subscriber", TopicProviderVerificationSuspended)
	subscribeProviderVerificationSuspended(ctx, client, repository)
}

// subscribeProviderVerificationSuspended listens for provider-level account
// suspensions and deactivates all bikes owned by that provider.
func subscribeProviderVerificationSuspended(ctx context.Context, client *redis.Client, repository Repository) {
	vehicleSubscribe(ctx, client, TopicProviderVerificationSuspended, func(ctx context.Context, payload []byte) error {
		return HandleProviderVerificationSuspendedPayload(ctx, repository, payload)
	})
}

// HandleProviderVerificationSuspendedPayload processes a provider.verification.suspended
// event by suspending all bikes for that provider.
// A bad payload is logged and does NOT crash the service.
func HandleProviderVerificationSuspendedPayload(ctx context.Context, repository Repository, payload []byte) error {
	var event ProviderVerificationSuspendedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("vehicle subscriber %s: invalid payload: %v — skipping", TopicProviderVerificationSuspended, err)
		return nil // Do not crash on bad payload.
	}

	providerID := strings.TrimSpace(event.ProviderID)
	if providerID == "" {
		log.Printf("vehicle subscriber %s: empty provider_id — skipping", TopicProviderVerificationSuspended)
		return nil
	}

	reason := event.Reason
	if err := repository.SuspendAllBikesForProvider(ctx, providerID, reason); err != nil {
		log.Printf("vehicle subscriber %s: suspend bikes for provider_id=%s error=%v", TopicProviderVerificationSuspended, providerID, err)
		// Do not return the error — log it and continue.
		return nil
	}
	log.Printf("vehicle subscriber %s: suspended all bikes for provider_id=%s", TopicProviderVerificationSuspended, providerID)
	return nil
}

// vehicleSubscribe is the low-level subscription loop shared by all vehicle subscribers.
// A handler error is logged but does NOT terminate the loop.
func vehicleSubscribe(ctx context.Context, client *redis.Client, topic string, handle func(context.Context, []byte) error) {
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
				log.Printf("vehicle subscriber %s: received payload=%s", topic, msg.Payload)
				msgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				if err := handle(msgCtx, []byte(msg.Payload)); err != nil {
					log.Printf("vehicle subscriber %s: handler error: %v", topic, err)
				}
				cancel()
			}
		}
	}()
}
