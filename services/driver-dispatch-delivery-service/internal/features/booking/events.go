package booking

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
)

// EventPublisher publishes booking.dispatch.cancelled so the request and trip
// subscribers (and any external observers) react to customer cancellations the
// same way they do for partner bookings.
type EventPublisher interface {
	PublishBookingCancelled(ctx context.Context, bookingID, reason string, cancelledAt time.Time) error
}

type RedisEventPublisher struct {
	client *redis.Client
}

func NewRedisEventPublisher(client *redis.Client) *RedisEventPublisher {
	return &RedisEventPublisher{client: client}
}

func (p *RedisEventPublisher) PublishBookingCancelled(ctx context.Context, bookingID, reason string, cancelledAt time.Time) error {
	if p == nil || p.client == nil {
		return nil
	}
	payload, err := json.Marshal(requestfeature.BookingDispatchCancelledEvent{
		Event:       requestfeature.TopicBookingDispatchCancelled,
		BookingID:   bookingID,
		Reason:      reason,
		CancelledAt: cancelledAt,
		OccurredAt:  cancelledAt,
	})
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, requestfeature.TopicBookingDispatchCancelled, payload).Err()
}
