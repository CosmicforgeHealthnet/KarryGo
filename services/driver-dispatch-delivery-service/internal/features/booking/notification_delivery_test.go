package booking

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/notifications"
	"errors"
	"testing"
)

type lifecycleSender struct {
	sent []notifications.Request
	fail bool
}

func (n *lifecycleSender) Send(_ context.Context, r notifications.Request) (notifications.SendResponse, error) {
	n.sent = append(n.sent, r)
	if err := r.Validate(); err != nil {
		return notifications.SendResponse{}, err
	}
	if n.fail {
		return notifications.SendResponse{}, errors.New("response lost")
	}
	return notifications.SendResponse{MessageID: "durable-message"}, nil
}
func TestLifecycleNotificationTimeoutRetriesSameIdentityAndMinimizes(t *testing.T) {
	n := &lifecycleSender{fail: true}
	d := LifecycleNotificationDeliverer{Notifier: n}
	e := marketplace.Envelope{EventID: "event", EventType: notifications.EventDeliveryAccepted, SourceService: "driver-dispatch-delivery-service", AggregateType: "booking_notification", AggregateID: "booking:customer", Payload: map[string]any{"booking_id": "booking", "recipient_id": "customer", "recipient_type": "customer", "otp": "do-not-forward"}}
	first, _ := d.Deliver(context.Background(), e)
	if first.Outcome != marketplace.DeliveryUnknown {
		t.Fatal(first)
	}
	n.fail = false
	second, err := d.Deliver(context.Background(), e)
	if err != nil || second.Outcome != marketplace.DeliveryAccepted {
		t.Fatal(second, err)
	}
	if len(n.sent) != 2 || n.sent[0].IDempotencyKey != n.sent[1].IDempotencyKey {
		t.Fatal("retry identity changed")
	}
	if len(n.sent[1].Data) != 1 {
		t.Fatal("arbitrary payload forwarded")
	}
	e.EventType = notifications.EventPackageDeliveryCodeReady
	if result, _ := d.Deliver(context.Background(), e); result.Outcome != marketplace.DeliveryRejected {
		t.Fatal("generic secret notification accepted")
	}
}
