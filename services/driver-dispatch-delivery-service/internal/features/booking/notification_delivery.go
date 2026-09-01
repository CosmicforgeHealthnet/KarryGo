package booking

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/notifications"
	"fmt"
)

// LifecycleNotificationDeliverer shares the canonical target worker with recovery.
type LifecycleNotificationDeliverer struct {
	Notifier notifications.Notifier
	Other    marketplace.EventDeliverer
}

func (d LifecycleNotificationDeliverer) Configured() bool { return d.Notifier != nil }
func (d LifecycleNotificationDeliverer) Deliver(ctx context.Context, e marketplace.Envelope) (marketplace.DeliveryResult, error) {
	if e.AggregateType != "booking_notification" {
		if d.Other == nil {
			return marketplace.DeliveryResult{Outcome: marketplace.DeliveryRejected, Code: "UNSUPPORTED_NOTIFICATION"}, nil
		}
		return d.Other.Deliver(ctx, e)
	}
	allowed := map[string]bool{notifications.EventDeliveryAccepted: true, notifications.EventDeliveryUnmatched: true, notifications.EventDeliveryEnRoutePickup: true, notifications.EventDeliveryArrivedPickup: true, notifications.EventPackagePickedUp: true, notifications.EventPackageDelivered: true, notifications.EventDeliveryCompleted: true, notifications.EventDeliveryCancelled: true, notifications.EventDeliveryCancelledByProvider: true, notifications.EventDeliveryPickupVerificationRequired: true}
	if !allowed[e.EventType] || e.SourceService != "driver-dispatch-delivery-service" {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryRejected, Code: "UNSUPPORTED_NOTIFICATION"}, fmt.Errorf("unsupported lifecycle notification")
	}
	bookingID, _ := e.Payload["booking_id"].(string)
	recipientID, _ := e.Payload["recipient_id"].(string)
	recipientType, _ := e.Payload["recipient_type"].(string)
	if bookingID == "" || recipientID == "" || e.AggregateID != bookingID+":"+recipientID || (recipientType != "customer" && recipientType != "provider") {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryRejected, Code: "INVALID_INTENT"}, fmt.Errorf("invalid notification intent")
	}
	data := map[string]interface{}{"booking_id": bookingID}
	ack, err := d.Notifier.Send(ctx, notifications.Request{IDempotencyKey: notifications.IdempotencyKey(e.SourceService, e.EventType, e.AggregateID), SourceService: e.SourceService, EventType: e.EventType, Recipient: notifications.Recipient{Type: recipientType, ID: recipientID}, TemplateKey: e.EventType, Data: data, TemplateData: data})
	if err != nil || ack.MessageID == "" {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryUnknown, Code: "NOTIFICATION_ACK_UNKNOWN"}, err
	}
	return marketplace.DeliveryResult{Outcome: marketplace.DeliveryAccepted}, nil
}
