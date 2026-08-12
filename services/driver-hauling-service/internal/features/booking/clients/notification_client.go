package bookingclients

import (
	"context"
	"errors"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
	"github.com/redis/go-redis/v9"
)

const sourceService = "driver-hauling-service"

// BookingNotifier sends booking-lifecycle notifications to the central
// notification-service on behalf of the hauling service. It is intentionally
// fire-and-forget: callers should never fail a booking operation because a
// notification could not be sent. Send errors are logged, not returned.
type BookingNotifier struct {
	notifier notifications.Notifier
}

// NewBookingNotifier builds a BookingNotifier from the notification-service base
// URL and the HMAC secret shared with it. An incomplete configuration remains
// an error-producing real client; production must never report a fabricated
// notification success through a no-op implementation.
func NewBookingNotifier(baseURL string, secret []byte) *BookingNotifier {
	return &BookingNotifier{
		notifier: notifications.Client{
			BaseURL:     baseURL,
			ServiceName: sourceService,
			Secret:      secret,
		},
	}
}

// NewBookingNotifierWith wraps an explicit notifier. Useful in tests.
func NewBookingNotifierWith(notifier notifications.Notifier) *BookingNotifier {
	return &BookingNotifier{notifier: notifier}
}

// NewDurableBookingNotifier writes notification intents to the shared Redis
// stream consumed by notification-service. A downstream outage leaves the
// stream entry pending for retry; the booking transition is never rolled back
// and delivery is never fabricated.
func NewDurableBookingNotifier(client *redis.Client) *BookingNotifier {
	return NewBookingNotifierWith(redisQueueNotifier{publisher: notifications.RedisPublisher{Client: client}})
}

type redisQueueNotifier struct{ publisher notifications.RedisPublisher }

func (n redisQueueNotifier) Send(ctx context.Context, request notifications.Request) (notifications.SendResponse, error) {
	if n.publisher.Client == nil {
		return notifications.SendResponse{}, errors.New("notification queue is not configured")
	}
	if err := n.publisher.Publish(ctx, request); err != nil {
		return notifications.SendResponse{}, err
	}
	return notifications.SendResponse{MessageID: request.IDempotencyKey, Status: "queued"}, nil
}

// NotifyProviderMatched tells a provider they have been matched to a booking and
// must accept it.
func (n *BookingNotifier) NotifyProviderMatched(ctx context.Context, providerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventBookingMatched, notifications.RecipientProvider, providerID, bookingID, data)
}

// NotifyCustomerAccepted tells the customer a provider accepted their booking.
func (n *BookingNotifier) NotifyCustomerAccepted(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventBookingAccepted, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerUnmatched tells the customer no provider could be matched.
func (n *BookingNotifier) NotifyCustomerUnmatched(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventBookingUnmatched, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerEnRoutePickup tells the customer the driver is heading to pickup.
func (n *BookingNotifier) NotifyCustomerEnRoutePickup(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDriverEnRoutePickup, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerArrivedPickup tells the customer the driver arrived at pickup.
func (n *BookingNotifier) NotifyCustomerArrivedPickup(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDriverArrivedPickup, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerEnRouteDelivery tells the customer the cargo is on the way.
func (n *BookingNotifier) NotifyCustomerEnRouteDelivery(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDriverEnRouteDelivery, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerPickupVerificationRequired tells the customer their driver
// has arrived and must be verified before pickup can proceed. The driver's
// code is intentionally NOT included in this payload — only the driver's own
// app displays it.
func (n *BookingNotifier) NotifyCustomerPickupVerificationRequired(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventBookingPickupVerificationRequired, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerDeliveryCodeReady tells the customer their delivery
// verification code is ready to share with the driver. The code is included
// in data so it can be surfaced in the notification body.
func (n *BookingNotifier) NotifyCustomerDeliveryCodeReady(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventBookingDeliveryCodeReady, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyProviderPickupVerified tells the driver the customer has verified
// their pickup credential, so confirm-pickup is now unblocked.
func (n *BookingNotifier) NotifyProviderPickupVerified(ctx context.Context, providerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventBookingPickupVerified, notifications.RecipientProvider, providerID, bookingID, data)
}

// NotifyCustomerPickedUp tells the customer their cargo was picked up.
func (n *BookingNotifier) NotifyCustomerPickedUp(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventCargoPickedUp, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerDelivered tells the customer their cargo was delivered.
func (n *BookingNotifier) NotifyCustomerDelivered(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventCargoDelivered, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerCompleted tells the customer their booking is complete.
func (n *BookingNotifier) NotifyCustomerCompleted(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventBookingCompleted, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyProviderCancelled tells a provider the customer cancelled the booking.
func (n *BookingNotifier) NotifyProviderCancelled(ctx context.Context, providerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventBookingCancelled, notifications.RecipientProvider, providerID, bookingID, data)
}

// NotifyCustomerCancelledByProvider tells the customer a provider cancelled.
func (n *BookingNotifier) NotifyCustomerCancelledByProvider(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventBookingCancelledByProvider, notifications.RecipientCustomer, customerID, bookingID, data)
}

// send builds and dispatches a notification request. It is fire-and-forget:
// errors are logged and swallowed. The idempotency key is derived from the
// event, booking AND recipient: matching offers the same booking to several
// providers in turn, and a booking-only key would make notification-service
// silently drop every provider's booking.matched after the first (ON CONFLICT
// DO NOTHING on idempotency_key). Retries of the same logical event to the
// same recipient still dedupe. Title/body are resolved from a seeded template
// keyed on the event type; data is passed as both notification data and
// template variables.
func (n *BookingNotifier) send(ctx context.Context, eventType, recipientType, recipientID, bookingID string, data map[string]interface{}) {
	if recipientID == "" {
		return
	}
	_, err := n.notifier.Send(ctx, notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey(sourceService, eventType, bookingID+":"+recipientID),
		SourceService:  sourceService,
		EventType:      eventType,
		Recipient: notifications.Recipient{
			Type: recipientType,
			ID:   recipientID,
		},
		TemplateKey:  eventType,
		Data:         data,
		TemplateData: data,
	})
	if err != nil {
		logging.Error("notification", "send event=%s booking=%s recipient=%s: %v", eventType, bookingID, recipientID, err)
	}
}
