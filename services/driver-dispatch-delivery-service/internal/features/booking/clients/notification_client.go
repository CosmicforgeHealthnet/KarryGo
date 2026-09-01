package bookingclients

import (
	"context"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
)

const sourceService = "driver-dispatch-delivery-service"

// BookingNotifier sends delivery-lifecycle notifications to the central
// notification-service on behalf of the dispatch service. It is intentionally
// fire-and-forget: callers should never fail a booking or trip operation
// because a notification could not be sent. Send errors are logged, not
// returned.
type BookingNotifier struct {
	notifier         notifications.Notifier
	durableLifecycle bool
}

// Production Booking/Trip transitions persist lifecycle intent atomically.
// Only the existing tightly scoped delivery-code transport stays direct.
func (n *BookingNotifier) WithDurableLifecycle() *BookingNotifier {
	n.durableLifecycle = true
	return n
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

// NotifyCustomerAccepted tells the customer a rider accepted their delivery.
func (n *BookingNotifier) NotifyCustomerAccepted(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDeliveryAccepted, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerUnmatched tells the customer no rider could be found.
func (n *BookingNotifier) NotifyCustomerUnmatched(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDeliveryUnmatched, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerEnRoutePickup tells the customer the rider is heading to pickup.
func (n *BookingNotifier) NotifyCustomerEnRoutePickup(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDeliveryEnRoutePickup, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerArrivedPickup tells the customer the rider arrived at pickup.
func (n *BookingNotifier) NotifyCustomerArrivedPickup(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDeliveryArrivedPickup, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerPickedUp tells the customer their package is in transit.
func (n *BookingNotifier) NotifyCustomerPickedUp(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventPackagePickedUp, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerPickupVerificationRequired tells the customer their rider
// has arrived and must be verified before pickup can proceed. The rider's
// code is intentionally NOT included in this payload — only the rider's own
// app displays it.
func (n *BookingNotifier) NotifyCustomerPickupVerificationRequired(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDeliveryPickupVerificationRequired, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerDeliveryCodeReady tells the customer their delivery
// verification code is ready. Generic events do not carry the credential;
// the customer reads it through the ownership-scoped Booking endpoint.
func (n *BookingNotifier) NotifyCustomerDeliveryCodeReady(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventPackageDeliveryCodeReady, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerDelivered tells the customer their package was delivered.
func (n *BookingNotifier) NotifyCustomerDelivered(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventPackageDelivered, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyCustomerCompleted tells the customer their delivery is complete.
func (n *BookingNotifier) NotifyCustomerCompleted(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDeliveryCompleted, notifications.RecipientCustomer, customerID, bookingID, data)
}

// NotifyProviderCancelled tells a rider the customer cancelled the delivery.
func (n *BookingNotifier) NotifyProviderCancelled(ctx context.Context, providerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDeliveryCancelled, notifications.RecipientProvider, providerID, bookingID, data)
}

// NotifyCustomerCancelledByProvider tells the customer the rider cancelled.
func (n *BookingNotifier) NotifyCustomerCancelledByProvider(ctx context.Context, customerID, bookingID string, data map[string]interface{}) {
	n.send(ctx, notifications.EventDeliveryCancelledByProvider, notifications.RecipientCustomer, customerID, bookingID, data)
}

// send builds and dispatches a notification request. It is fire-and-forget:
// errors are logged and swallowed. The idempotency key is derived from the
// event, booking AND recipient (hauling pattern): a booking-only key would let
// notification-service silently drop the same logical event to a second
// recipient (ON CONFLICT DO NOTHING on idempotency_key). Retries of the same
// logical event to the same recipient still dedupe. Title/body resolve from
// the seeded template keyed on the event type.
func (n *BookingNotifier) send(ctx context.Context, eventType, recipientType, recipientID, bookingID string, data map[string]interface{}) {
	if n.durableLifecycle && eventType != notifications.EventPackageDeliveryCodeReady {
		return
	}
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

// noopNotifier is used when the notification-service is not configured.
type noopNotifier struct{}

func (noopNotifier) Send(context.Context, notifications.Request) (notifications.SendResponse, error) {
	return notifications.SendResponse{}, nil
}
