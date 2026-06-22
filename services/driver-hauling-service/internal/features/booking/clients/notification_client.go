package bookingclients

import (
	"context"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
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
// URL and the HMAC secret shared with it. When either is empty (e.g. local dev
// without the notification-service running) it returns a no-op notifier so the
// booking flow keeps working.
func NewBookingNotifier(baseURL string, secret []byte) *BookingNotifier {
	if baseURL == "" || len(secret) == 0 {
		return &BookingNotifier{notifier: noopNotifier{}}
	}
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
// event and booking so retries of the same logical event dedupe. Title/body are
// resolved from a seeded template keyed on the event type; data is passed as
// both notification data and template variables.
func (n *BookingNotifier) send(ctx context.Context, eventType, recipientType, recipientID, bookingID string, data map[string]interface{}) {
	if recipientID == "" {
		return
	}
	_, err := n.notifier.Send(ctx, notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey(sourceService, eventType, bookingID),
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

// noopNotifier is used when the notification-service is not configured.
type noopNotifier struct{}

func (noopNotifier) Send(context.Context, notifications.Request) (notifications.SendResponse, error) {
	return notifications.SendResponse{}, nil
}
