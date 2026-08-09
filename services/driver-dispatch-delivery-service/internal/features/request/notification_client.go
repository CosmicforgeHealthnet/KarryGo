package request

import (
	"context"

	"cosmicforge/logistics/shared/go/notifications"
)

const notificationSourceService = "driver-dispatch-delivery-service"

// NotificationServiceSender delivers new-request broadcasts to riders through
// the central notification-service (push + in-app). It replaces the
// LoggingNotificationSender stub when DISPATCH_NOTIFICATION_URL/SECRET are set.
//
// Hard send errors are returned so the asynq send_push task retries; device
// state (missing FCM token etc.) is owned by notification-service, which
// degrades to the in-app channel on its own.
type NotificationServiceSender struct {
	notifier notifications.Notifier
}

func NewNotificationServiceSender(baseURL string, secret []byte) *NotificationServiceSender {
	return &NotificationServiceSender{
		notifier: notifications.Client{
			BaseURL:     baseURL,
			ServiceName: notificationSourceService,
			Secret:      secret,
		},
	}
}

// NewNotificationServiceSenderWith wraps an explicit notifier. Useful in tests.
func NewNotificationServiceSenderWith(notifier notifications.Notifier) *NotificationServiceSender {
	return &NotificationServiceSender{notifier: notifier}
}

// SendRequestBroadcast notifies one rider about a new delivery request. The
// idempotency key is the inbox id — one inbox row exists per rider per
// broadcast attempt, so the key is inherently recipient-aware and re-offers on
// a wider radius (new inbox row) are not deduped away.
func (s *NotificationServiceSender) SendRequestBroadcast(ctx context.Context, providerID string, payload RequestPushPayload) error {
	data := map[string]interface{}{
		"type":            payload.Type,
		"inbox_id":        payload.InboxID,
		"broadcast_id":    payload.BroadcastID,
		"booking_id":      payload.BookingID,
		"fare_kobo":       payload.FareAmount,
		"pickup_address":  payload.PickupAddress,
		"dropoff_address": payload.DropoffAddress,
		"package_desc":    payload.PackageDesc,
		"receiver_name":   payload.ReceiverName,
		"expires_in":      payload.ExpiresIn,
		"service":         "dispatch",
	}
	_, err := s.notifier.Send(ctx, notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey(notificationSourceService, notifications.EventDeliveryMatched, payload.InboxID),
		SourceService:  notificationSourceService,
		EventType:      notifications.EventDeliveryMatched,
		Recipient: notifications.Recipient{
			Type: notifications.RecipientProvider,
			ID:   providerID,
		},
		TemplateKey:  notifications.EventDeliveryMatched,
		Priority:     notifications.PriorityHigh,
		Data:         data,
		TemplateData: data,
	})
	return err
}
