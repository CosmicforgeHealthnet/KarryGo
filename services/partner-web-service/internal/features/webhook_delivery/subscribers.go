package webhookdelivery

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// dispatchStatusMap maps a dispatch topic to the partner booking status.
var dispatchStatusMap = map[string]string{
	"request.accepted":          "accepted",
	"request.no_provider_found": "no_provider",
	"trip.started":              "in_progress",
	"trip.completed":            "completed",
	"trip.cancelled":            "cancelled",
	"trip.failed":               "failed",
}

// topicToWebhookEvent maps a dispatch topic to the partner-facing webhook event.
func topicToWebhookEvent(topic string) string {
	m := map[string]string{
		"request.accepted":          "delivery.accepted",
		"request.no_provider_found": "delivery.no_provider",
		"trip.started":              "delivery.in_progress",
		"trip.completed":            "delivery.completed",
		"trip.cancelled":            "delivery.cancelled",
		"trip.failed":               "delivery.failed",
	}
	return m[topic]
}

// DispatchSubscriber consumes dispatch lifecycle events, updates the partner
// booking status, and enqueues partner webhook deliveries.
type DispatchSubscriber struct {
	repo   Repository
	worker WebhookWorkerInterface
	redis  *redis.Client
}

// NewDispatchSubscriber builds a DispatchSubscriber.
func NewDispatchSubscriber(redis *redis.Client, repo Repository, worker WebhookWorkerInterface) *DispatchSubscriber {
	return &DispatchSubscriber{repo: repo, worker: worker, redis: redis}
}

// Start subscribes to all 6 dispatch topics, one goroutine per topic. It returns
// immediately; goroutines run until ctx is cancelled.
func (s *DispatchSubscriber) Start(ctx context.Context) {
	topics := dispatchStatusMap
	for topic, status := range topics {
		go s.subscribe(ctx, topic, status)
		log.Printf("webhookdelivery: subscribed to %s", topic)
	}
}

func (s *DispatchSubscriber) subscribe(ctx context.Context, topic, newStatus string) {
	if s.redis == nil {
		return
	}
	sub := s.redis.Subscribe(ctx, topic)
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
			msgCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			if err := s.handleEvent(msgCtx, topic, newStatus, []byte(msg.Payload)); err != nil {
				log.Printf("webhookdelivery: %s handler error: %v", topic, err)
			}
			cancel()
		}
	}
}

// handleEvent parses the dispatch payload, updates the matched partner booking's
// status, and enqueues a webhook delivery. Malformed payloads, missing booking
// ids, and non-partner bookings are silently skipped (return nil).
func (s *DispatchSubscriber) handleEvent(ctx context.Context, topic, newStatus string, payload []byte) error {
	var event struct {
		BookingID string `json:"booking_id"`
	}
	if err := json.Unmarshal(payload, &event); err != nil || event.BookingID == "" {
		return nil
	}

	booking, err := s.repo.FindByDispatchBookingID(ctx, event.BookingID)
	if err != nil || booking == nil {
		// Not a partner booking (or lookup error): skip silently.
		return nil
	}

	if err := s.repo.UpdateBookingStatus(ctx, booking.ID, newStatus); err != nil {
		return err
	}

	if webhookEvent := topicToWebhookEvent(topic); webhookEvent != "" {
		return s.worker.EnqueueForBooking(ctx, booking.ID, booking.PartnerID, webhookEvent, payload)
	}
	return nil
}
