package deliveryrecovery

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/notifications"
)

type notificationCustomer struct{}

func (notificationCustomer) RecoveryNotificationCustomer(context.Context, string, string) (string, error) {
	return "actual-customer", nil
}

type recoveryNotificationRecorder struct {
	requests []notifications.Request
	fail     bool
}

func (r *recoveryNotificationRecorder) Send(_ context.Context, request notifications.Request) (notifications.SendResponse, error) {
	r.requests = append(r.requests, request)
	if r.fail {
		return notifications.SendResponse{}, errors.New("ack lost")
	}
	return notifications.SendResponse{MessageID: "durable-message", Status: "accepted"}, nil
}
func TestRecoveryNotificationRetryBindsRecipientAndEvent(t *testing.T) {
	recorder := &recoveryNotificationRecorder{fail: true}
	d := NewRecoveryNotificationDeliverer(notificationCustomer{}, recorder)
	e := marketplace.Envelope{EventID: "event", EventType: "dispatch.delivery_recovery.activated.v1", AggregateType: "recovery_action", AggregateID: "action", AggregateVersion: 1, SourceService: "driver-dispatch-delivery-service", Payload: map[string]any{"customer_id": "forged", "otp": "never-forward"}}
	result, err := d.Deliver(context.Background(), e)
	if err == nil || result.Outcome != marketplace.DeliveryUnknown {
		t.Fatal("unknown acknowledgement was treated as delivery")
	}
	recorder.fail = false
	result, err = d.Deliver(context.Background(), e)
	if err != nil || result.Outcome != marketplace.DeliveryAccepted {
		t.Fatal(result, err)
	}
	a, b := recorder.requests[0], recorder.requests[1]
	if a.IDempotencyKey != b.IDempotencyKey || b.Recipient.ID != "actual-customer" {
		t.Fatal("recipient or idempotency authority lost")
	}
	if _, found := b.Data["otp"]; found {
		t.Fatal("secret forwarded")
	}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	e.EventType = "unknown"
	result, _ = d.Deliver(context.Background(), e)
	if result.Outcome != marketplace.DeliveryRejected || len(recorder.requests) != 2 {
		t.Fatal("unsupported event sent")
	}
}
