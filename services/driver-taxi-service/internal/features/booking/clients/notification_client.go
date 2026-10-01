// Package bookingclients holds the taxi booking feature's outbound adapters.
package bookingclients

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
)

const sourceService = "driver-taxi-service"

// RideNotifier sends ride-lifecycle notifications to notification-service on
// behalf of the taxi service.
//
// Deliberately fire-and-forget: a ride transition must never fail because a
// notification could not be delivered. Errors are logged, never returned.
type RideNotifier struct {
	notifier notifications.Notifier
}

// NewRideNotifier builds a notifier over the notification-service HTTP API.
// An incomplete configuration stays a real, error-producing client rather
// than degrading to a no-op: a fabricated "sent" is worse than a logged
// failure.
func NewRideNotifier(baseURL string, secret []byte) *RideNotifier {
	return &RideNotifier{
		notifier: notifications.Client{
			BaseURL:     baseURL,
			ServiceName: sourceService,
			Secret:      secret,
		},
	}
}

// NewRideNotifierWith wraps an explicit notifier. Used by tests.
func NewRideNotifierWith(notifier notifications.Notifier) *RideNotifier {
	return &RideNotifier{notifier: notifier}
}

// NewDurableRideNotifier writes intents to the Redis stream that
// notification-service consumes. A downstream outage leaves the entry pending
// for retry rather than dropping the notification.
func NewDurableRideNotifier(client *redis.Client) *RideNotifier {
	return NewRideNotifierWith(redisQueueNotifier{
		publisher: notifications.RedisPublisher{Client: client},
	})
}

type redisQueueNotifier struct{ publisher notifications.RedisPublisher }

func (n redisQueueNotifier) Send(ctx context.Context, request notifications.Request) (notifications.SendResponse, error) {
	if n.publisher.Client == nil {
		return notifications.SendResponse{}, errors.New("notification queue is not configured")
	}
	if err := n.publisher.Publish(ctx, request); err != nil {
		return notifications.SendResponse{}, err
	}
	return notifications.SendResponse{
		MessageID: request.IDempotencyKey,
		Status:    "queued",
	}, nil
}

// ─── Driver-facing ─────────────────────────────────────────────────────────

// NotifyDriverMatched tells a driver a ride has been offered to them.
func (n *RideNotifier) NotifyDriverMatched(ctx context.Context, providerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventRideMatched, notifications.RecipientProvider, providerID, bookingID, data)
}

// NotifyDriverCancelled tells the assigned driver the passenger called it off.
func (n *RideNotifier) NotifyDriverCancelled(ctx context.Context, providerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventRideCancelled, notifications.RecipientProvider, providerID, bookingID, data)
}

// ─── Passenger-facing ──────────────────────────────────────────────────────

func (n *RideNotifier) NotifyCustomerAccepted(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventRideAccepted, notifications.RecipientCustomer, customerID, bookingID, data)
}

func (n *RideNotifier) NotifyCustomerUnmatched(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventRideUnmatched, notifications.RecipientCustomer, customerID, bookingID, data)
}

func (n *RideNotifier) NotifyCustomerEnRoutePickup(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventRideEnRoutePickup, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerArrivedPickup doubles as the "your code is ready" nudge: the
// service generates the pickup credential in the very same transition.
func (n *RideNotifier) NotifyCustomerArrivedPickup(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventRideArrivedPickup, notifications.RecipientCustomer, customerID, bookingID, data)
}

func (n *RideNotifier) NotifyCustomerStarted(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventRideStarted, notifications.RecipientCustomer, customerID, bookingID, data)
}

func (n *RideNotifier) NotifyCustomerCompleted(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventRideCompleted, notifications.RecipientCustomer, customerID, bookingID, data)
}

func (n *RideNotifier) NotifyCustomerCancelledByProvider(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventRideCancelledByProvider, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerDeliveryCodeReady is parcel-only: the receiver's drop-off code
// has become available.
func (n *RideNotifier) NotifyCustomerDeliveryCodeReady(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventRideDeliveryCodeReady, notifications.RecipientCustomer, customerID, bookingID, data)
}

// send keys idempotency on booking AND recipient. Keying on the booking alone
// was a real bug in this platform: when a booking is offered to driver A and
// then driver B, B's "matched" notification was silently swallowed by
// notification-service's ON CONFLICT DO NOTHING against A's key.
func (n *RideNotifier) send(ctx context.Context, eventType, recipientType, recipientID, bookingID string, data map[string]interface{}) {
	if recipientID == "" {
		return
	}
	_, err := n.notifier.Send(ctx, notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey(
			sourceService, eventType, bookingID+":"+recipientID,
		),
		SourceService: sourceService,
		EventType:     eventType,
		Recipient: notifications.Recipient{
			Type: recipientType,
			ID:   recipientID,
		},
		TemplateKey:  eventType,
		Data:         data,
		TemplateData: data,
	})
	if err != nil {
		logging.Error(
			"notification",
			"send event=%s booking=%s recipient=%s: %v",
			eventType, bookingID, recipientID, err,
		)
	}
}
