package request

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/shared/go/notifications"
)

type captureNotifier struct {
	requests []notifications.Request
	err      error
}

func (c *captureNotifier) Send(_ context.Context, request notifications.Request) (notifications.SendResponse, error) {
	c.requests = append(c.requests, request)
	return notifications.SendResponse{}, c.err
}

func TestNotificationServiceSender_SendsDeliveryMatched(t *testing.T) {
	capture := &captureNotifier{}
	sender := NewNotificationServiceSenderWith(capture)

	err := sender.SendRequestBroadcast(context.Background(), "rider-1", RequestPushPayload{
		InboxID: "inbox-1", BroadcastID: "bc-1", BookingID: "book-1",
		FareAmount: 120000, PickupAddress: "A", DropoffAddress: "B",
		ExpiresIn: 30, Type: "new_request",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capture.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(capture.requests))
	}
	got := capture.requests[0]
	if got.EventType != notifications.EventDeliveryMatched {
		t.Fatalf("event type = %q", got.EventType)
	}
	if got.Recipient.Type != notifications.RecipientProvider || got.Recipient.ID != "rider-1" {
		t.Fatalf("unexpected recipient %+v", got.Recipient)
	}
	// Inbox id keys the idempotency (unique per rider per broadcast attempt).
	want := notifications.IdempotencyKey(notificationSourceService, notifications.EventDeliveryMatched, "inbox-1")
	if got.IDempotencyKey != want {
		t.Fatalf("idempotency key = %q, want %q", got.IDempotencyKey, want)
	}
	if got.Priority != notifications.PriorityHigh {
		t.Fatalf("priority = %q", got.Priority)
	}
}

func TestNotificationServiceSender_ErrorPropagatesForRetry(t *testing.T) {
	capture := &captureNotifier{err: errors.New("boom")}
	sender := NewNotificationServiceSenderWith(capture)

	err := sender.SendRequestBroadcast(context.Background(), "rider-1", RequestPushPayload{InboxID: "inbox-1"})
	if err == nil {
		t.Fatal("expected error so the asynq task retries")
	}
}
