package booking

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
)

// Topics consumed to keep dispatch_bookings in sync with the matching + trip
// pipeline. Partner-created bookings have no dispatch_bookings row, so their
// events no-op here.
const (
	topicRequestAccepted = requestfeature.TopicRequestAccepted
	topicNoProviderFound = requestfeature.TopicNoProviderFound
	topicTripCompleted   = "trip.completed"
	topicTripCancelled   = "trip.cancelled"
)

func StartSubscribers(ctx context.Context, client *redis.Client, service *Service) {
	if client == nil || service == nil {
		log.Printf("booking subscribers: not started because redis client or service is nil")
		return
	}
	subscribe := func(topic string, handle func(context.Context, []byte) error) {
		go func() {
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
						log.Printf("booking subscriber %s: dropped payload error=%v", topic, err)
					}
					cancel()
				}
			}
		}()
		log.Printf("booking subscribers: started %s subscriber", topic)
	}

	subscribe(topicRequestAccepted, func(ctx context.Context, payload []byte) error {
		return HandleRequestAcceptedPayload(ctx, service, payload)
	})
	subscribe(topicNoProviderFound, func(ctx context.Context, payload []byte) error {
		return HandleNoProviderFoundPayload(ctx, service, payload)
	})
	subscribe(topicTripCompleted, func(ctx context.Context, payload []byte) error {
		return HandleTripCompletedPayload(ctx, service, payload)
	})
	subscribe(topicTripCancelled, func(ctx context.Context, payload []byte) error {
		return HandleTripCancelledPayload(ctx, service, payload)
	})
}

func HandleRequestAcceptedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event requestfeature.RequestAcceptedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("booking subscriber %s: invalid payload: %v - skipping", topicRequestAccepted, err)
		return nil
	}
	if strings.TrimSpace(event.BookingID) == "" || strings.TrimSpace(event.ProviderID) == "" {
		return nil
	}
	return service.HandleRequestAccepted(ctx, event.BookingID, event.ProviderID)
}

func HandleNoProviderFoundPayload(ctx context.Context, service *Service, payload []byte) error {
	var event requestfeature.NoProviderFoundEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("booking subscriber %s: invalid payload: %v - skipping", topicNoProviderFound, err)
		return nil
	}
	if strings.TrimSpace(event.BookingID) == "" {
		return nil
	}
	return service.HandleNoProviderFound(ctx, event.BookingID)
}

type tripCompletedPayload struct {
	BookingID   string    `json:"booking_id"`
	CompletedAt time.Time `json:"completed_at"`
}

func HandleTripCompletedPayload(ctx context.Context, service *Service, payload []byte) error {
	var event tripCompletedPayload
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("booking subscriber %s: invalid payload: %v - skipping", topicTripCompleted, err)
		return nil
	}
	if strings.TrimSpace(event.BookingID) == "" {
		return nil
	}
	return service.HandleTripCompleted(ctx, event.BookingID, event.CompletedAt)
}

type tripCancelledPayload struct {
	BookingID                  string    `json:"booking_id"`
	TripID                     string    `json:"trip_id"`
	CancelledBy                string    `json:"cancelled_by"`
	ReasonCode                 string    `json:"reason_code"`
	RequiresAdminInvestigation bool      `json:"requires_admin_investigation"`
	CancelledAt                time.Time `json:"cancelled_at"`
}

func HandleTripCancelledPayload(ctx context.Context, service *Service, payload []byte) error {
	var event tripCancelledPayload
	if err := json.Unmarshal(payload, &event); err != nil {
		log.Printf("booking subscriber %s: invalid payload: %v - skipping", topicTripCancelled, err)
		return nil
	}
	if strings.TrimSpace(event.BookingID) == "" {
		return nil
	}
	return service.HandleTripCancelled(ctx, event.BookingID, event.TripID, event.CancelledBy, event.ReasonCode, event.RequiresAdminInvestigation, event.CancelledAt)
}
