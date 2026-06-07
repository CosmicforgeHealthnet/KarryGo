package trip

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	TopicRequestAccepted          = "request.accepted"
	TopicBookingDispatchCancelled = "booking.dispatch.cancelled"
	TopicProviderLocationUpdated  = "provider.location_updated"
)

func StartSubscribers(ctx context.Context, client *redis.Client, service *Service) {
	if client == nil || service == nil {
		log.Printf("trip subscribers: not started because redis client or service is nil")
		return
	}
	go startSubscriber(ctx, client, TopicRequestAccepted, func(ctx context.Context, payload []byte) error {
		return HandleRequestAcceptedPayload(ctx, service, payload)
	})
	go startSubscriber(ctx, client, TopicBookingDispatchCancelled, func(ctx context.Context, payload []byte) error {
		return HandleBookingDispatchCancelledPayload(ctx, service, payload)
	})
	go startSubscriber(ctx, client, TopicProviderLocationUpdated, func(ctx context.Context, payload []byte) error {
		return HandleProviderLocationUpdatedPayload(ctx, service, payload)
	})
	log.Printf("trip subscribers: started %s subscriber", TopicRequestAccepted)
	log.Printf("trip subscribers: started %s subscriber", TopicBookingDispatchCancelled)
	log.Printf("trip subscribers: started %s subscriber", TopicProviderLocationUpdated)
}

func HandleProviderLocationUpdatedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event ProviderLocationUpdatedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("trip subscriber %s: invalid payload: %v - skipping", TopicProviderLocationUpdated, err)
		return nil
	}
	if strings.TrimSpace(event.ProviderID) == "" {
		log.Printf("trip subscriber %s: missing provider_id - skipping", TopicProviderLocationUpdated)
		return nil
	}
	if err := service.HandleProviderLocationUpdated(ctx, event); err != nil {
		log.Printf("trip subscriber %s: payload rejected error=%v", TopicProviderLocationUpdated, err)
	}
	return nil
}

func startSubscriber(ctx context.Context, client *redis.Client, topic string, handle func(context.Context, []byte) error) {
	sub := client.Subscribe(ctx, topic)
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sub.Channel():
			if !ok {
				return
			}
			msgCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := handle(msgCtx, []byte(msg.Payload)); err != nil {
				log.Printf("trip subscriber %s: dropped payload error=%v", topic, err)
			}
			cancel()
		}
	}
}

func HandleRequestAcceptedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event RequestAcceptedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("trip subscriber %s: invalid payload: %v - skipping", TopicRequestAccepted, err)
		return nil
	}
	if strings.TrimSpace(event.Event) != "" && event.Event != TopicRequestAccepted {
		log.Printf("trip subscriber %s: unexpected event=%s - skipping", TopicRequestAccepted, event.Event)
		return nil
	}
	if _, err := service.HandleRequestAccepted(ctx, event); err != nil {
		log.Printf("trip subscriber %s: payload rejected error=%v", TopicRequestAccepted, err)
		return nil
	}
	log.Printf("trip subscriber %s: trip ensured booking_id=%s provider_id=%s", TopicRequestAccepted, event.BookingID, event.ProviderID)
	return nil
}

func HandleBookingDispatchCancelledPayload(ctx context.Context, service *Service, payload []byte) error {
	var event BookingDispatchCancelledEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("trip subscriber %s: invalid payload: %v - skipping", TopicBookingDispatchCancelled, err)
		return nil
	}
	if strings.TrimSpace(event.BookingID) == "" {
		log.Printf("trip subscriber %s: missing booking_id - skipping", TopicBookingDispatchCancelled)
		return nil
	}
	if err := service.HandleBookingDispatchCancelled(ctx, event); err != nil {
		log.Printf("trip subscriber %s: payload rejected error=%v", TopicBookingDispatchCancelled, err)
	}
	return nil
}
