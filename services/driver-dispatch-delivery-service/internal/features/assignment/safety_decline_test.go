package assignment

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/notifications"
)

type sentNotifications struct{ requests []notifications.Request }

func (s *sentNotifications) Send(_ context.Context, request notifications.Request) (notifications.SendResponse, error) {
	s.requests = append(s.requests, request)
	return notifications.SendResponse{MessageID: "m1", Status: "queued"}, nil
}

type passThrough struct{ seen []string }

func (p *passThrough) Configured() bool { return true }
func (p *passThrough) Deliver(_ context.Context, e marketplace.Envelope) (marketplace.DeliveryResult, error) {
	p.seen = append(p.seen, e.EventType)
	return marketplace.DeliveryResult{Outcome: marketplace.DeliveryAccepted}, nil
}

func TestASafetyDeclineReachesOperations(t *testing.T) {
	sent, next := &sentNotifications{}, &passThrough{}
	d := SafetyDeclineNotifier{Notifier: sent, Next: next}
	result, err := d.Deliver(context.Background(), marketplace.Envelope{EventID: "e1",
		EventType: notifications.EventNetworkAdminSafetyDecline, AggregateType: SafetyDeclineAggregate,
		AggregateID: "offer-1", Payload: map[string]any{"mission_id": "m1", "provider_id": "r1", "reason_code": "UNSAFE_VEHICLE"}})
	if err != nil || result.Outcome != marketplace.DeliveryAccepted || len(sent.requests) != 1 {
		t.Fatalf("result=%+v err=%v sent=%d", result, err, len(sent.requests))
	}
	request := sent.requests[0]
	if request.Recipient.Type != notifications.RecipientAdminOperations || request.EventType != notifications.EventNetworkAdminSafetyDecline {
		t.Fatalf("request %+v", request)
	}
	if err = request.Validate(); err != nil {
		t.Fatalf("the operations notification is invalid: %v", err)
	}
	if len(next.seen) != 0 {
		t.Fatal("a safety row fell through to the lifecycle deliverer")
	}
}

func TestOtherNotificationsFallThroughUnchanged(t *testing.T) {
	sent, next := &sentNotifications{}, &passThrough{}
	d := SafetyDeclineNotifier{Notifier: sent, Next: next}
	if _, err := d.Deliver(context.Background(), marketplace.Envelope{EventType: "delivery.accepted",
		AggregateType: "booking_notification"}); err != nil {
		t.Fatal(err)
	}
	if len(next.seen) != 1 || len(sent.requests) != 0 {
		t.Fatalf("next=%v sent=%d", next.seen, len(sent.requests))
	}
}
