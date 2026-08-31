package deliveryrecovery

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/notifications"
)

type NotificationRecipientResolver interface {
	RecoveryNotificationCustomer(context.Context, string, string) (string, error)
}

// RecoveryNotificationDeliverer adapts existing durable domain events to the
// Notification Service's accepted send contract. It does not forward arbitrary
// domain payloads, recipient fields or secret-bearing material.
type RecoveryNotificationDeliverer struct {
	recipients NotificationRecipientResolver
	notifier   notifications.Notifier
}

func NewRecoveryNotificationDeliverer(repo NotificationRecipientResolver, client notifications.Notifier) *RecoveryNotificationDeliverer {
	return &RecoveryNotificationDeliverer{recipients: repo, notifier: client}
}
func (d *RecoveryNotificationDeliverer) Configured() bool {
	return d != nil && d.recipients != nil && d.notifier != nil
}

var recoveryNotificationEvents = map[string]bool{
	"dispatch.delivery_attempt.started.v1": true, "dispatch.receiver_unavailable.recorded.v1": true,
	"dispatch.delivery_recovery.selected.v1": true, "dispatch.delivery_recovery.activated.v1": true,
	"dispatch.delivery.retry_scheduled.v1": true, "dispatch.delivery.rescheduled.v1": true,
	"dispatch.delivery.alternative_receiver_selected.v1": true, "dispatch.return.started.v1": true,
	"dispatch.hub_hold.started.v1": true, "dispatch.delivery.operations_review_requested.v1": true,
	"dispatch.delivery_incident.reported.v1": true, "dispatch.provider_replacement.started.v1": true,
	"dispatch.hub_hold.checkpoint_recorded.v1": true, "dispatch.hub_hold.released.v1": true,
	"dispatch.emergency_recovery.controlled.v1": true,
}

func (d *RecoveryNotificationDeliverer) Deliver(ctx context.Context, e marketplace.Envelope) (marketplace.DeliveryResult, error) {
	if !d.Configured() {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryRetryable, Code: "NOTIFICATION_NOT_CONFIGURED"}, nil
	}
	if e.EventID == "" || !recoveryNotificationEvents[e.EventType] || e.SourceService != "driver-dispatch-delivery-service" {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryRejected, Code: "UNSUPPORTED_RECOVERY_EVENT"}, nil
	}
	customer, err := d.recipients.RecoveryNotificationCustomer(ctx, e.AggregateType, e.AggregateID)
	if err != nil || customer == "" {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryRetryable, Code: "RECIPIENT_READBACK_UNAVAILABLE"}, err
	}
	request := notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey(e.SourceService, e.EventType, e.EventID),
		SourceService:  e.SourceService, SourceEventID: e.EventID, SourceAggregateType: e.AggregateType, SourceAggregateID: e.AggregateID, SourceVersion: e.AggregateVersion,
		EventType: e.EventType, Recipient: notifications.Recipient{Type: notifications.RecipientCustomer, ID: customer},
		Channels: []string{notifications.ChannelInApp, notifications.ChannelPush},
		Title:    "Delivery update", Body: "Your delivery recovery status has changed. Open your delivery for the current status.",
		Data:     map[string]any{"aggregate_type": e.AggregateType, "aggregate_id": e.AggregateID, "event_type": e.EventType},
		Priority: notifications.PriorityNormal,
	}
	response, err := d.notifier.Send(ctx, request)
	if err != nil {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryUnknown, Code: "NOTIFICATION_ACKNOWLEDGEMENT_UNKNOWN"}, err
	}
	if response.MessageID == "" {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryUnknown, Code: "NOTIFICATION_ACKNOWLEDGEMENT_MISSING"}, nil
	}
	return marketplace.DeliveryResult{Outcome: marketplace.DeliveryAccepted}, nil
}

func (r *Repository) RecoveryNotificationCustomer(ctx context.Context, kind, id string) (string, error) {
	var customer string
	switch kind {
	case "trip":
		err := r.db.QueryRow(ctx, `SELECT customer_id::text FROM trips WHERE id=$1::uuid`, id).Scan(&customer)
		return customer, err
	case "recovery_action":
		err := r.db.QueryRow(ctx, `SELECT t.customer_id::text FROM dispatch_delivery_recovery_actions a JOIN trips t ON t.id=a.trip_id WHERE a.id=$1::uuid`, id).Scan(&customer)
		return customer, err
	default:
		return "", errors.New("unsupported recovery aggregate")
	}
}
