package request

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func StartSubscribers(ctx context.Context, client *redis.Client, service *Service) {
	if client == nil || service == nil {
		log.Printf("request subscribers: not started because redis client or service is nil")
		return
	}
	go startCreatedSubscriber(ctx, client, service)
	go startCancelledSubscriber(ctx, client, service)
	log.Printf("request subscribers: started %s subscriber", TopicBookingDispatchCreated)
	log.Printf("request subscribers: started %s subscriber", TopicBookingDispatchCancelled)
}

func startCreatedSubscriber(ctx context.Context, client *redis.Client, service *Service) {
	sub := client.Subscribe(ctx, TopicBookingDispatchCreated)
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
			if err := HandleBookingDispatchCreatedPayload(msgCtx, service, []byte(msg.Payload)); err != nil {
				log.Printf("request subscriber %s: dropped payload error=%v", TopicBookingDispatchCreated, err)
			}
			cancel()
		}
	}
}

func startCancelledSubscriber(ctx context.Context, client *redis.Client, service *Service) {
	sub := client.Subscribe(ctx, TopicBookingDispatchCancelled)
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
			if err := HandleBookingDispatchCancelledPayload(msgCtx, service, []byte(msg.Payload)); err != nil {
				log.Printf("request subscriber %s: dropped payload error=%v", TopicBookingDispatchCancelled, err)
			}
			cancel()
		}
	}
}

func HandleBookingDispatchCreatedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event BookingDispatchCreatedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("request subscriber %s: invalid payload: %v - skipping", TopicBookingDispatchCreated, err)
		return nil
	}
	_, err := service.StartBroadcast(ctx, event)
	return err
}

// HandleBookingDispatchCancelledPayload processes a booking.dispatch.cancelled message (Phase 6I).
// Invalid JSON and missing booking_id are logged and dropped without crashing.
func HandleBookingDispatchCancelledPayload(ctx context.Context, service *Service, payload []byte) error {
	var event BookingDispatchCancelledEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("request subscriber %s: invalid payload: %v - skipping", TopicBookingDispatchCancelled, err)
		return nil
	}
	if strings.TrimSpace(event.BookingID) == "" {
		log.Printf("request subscriber %s: missing booking_id - skipping", TopicBookingDispatchCancelled)
		return nil
	}
	return service.CancelBroadcastForBooking(ctx, event.BookingID)
}
