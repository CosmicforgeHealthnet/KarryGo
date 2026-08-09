package bookingclients

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

func TestNotifier_RecipientAwareIdempotencyKey(t *testing.T) {
	capture := &captureNotifier{}
	notifier := NewBookingNotifierWith(capture)

	notifier.NotifyCustomerAccepted(context.Background(), "cust-1", "book-1", map[string]interface{}{"fare": 1})

	if len(capture.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(capture.requests))
	}
	got := capture.requests[0]
	want := notifications.IdempotencyKey(sourceService, notifications.EventDeliveryAccepted, "book-1:cust-1")
	if got.IDempotencyKey != want {
		t.Fatalf("idempotency key = %q, want %q", got.IDempotencyKey, want)
	}
	if got.Recipient.Type != notifications.RecipientCustomer || got.Recipient.ID != "cust-1" {
		t.Fatalf("unexpected recipient %+v", got.Recipient)
	}
	if got.TemplateKey != notifications.EventDeliveryAccepted {
		t.Fatalf("template key = %q", got.TemplateKey)
	}
}

func TestNotifier_EmptyRecipientShortCircuits(t *testing.T) {
	capture := &captureNotifier{}
	notifier := NewBookingNotifierWith(capture)

	notifier.NotifyCustomerCompleted(context.Background(), "", "book-1", nil)

	if len(capture.requests) != 0 {
		t.Fatalf("expected no requests for empty recipient, got %d", len(capture.requests))
	}
}

func TestNotifier_SendErrorSwallowed(t *testing.T) {
	capture := &captureNotifier{err: errors.New("notification-service down")}
	notifier := NewBookingNotifierWith(capture)

	// Must not panic or propagate: fire-and-forget contract.
	notifier.NotifyProviderCancelled(context.Background(), "rider-1", "book-1", nil)

	if len(capture.requests) != 1 {
		t.Fatalf("expected the request to have been attempted")
	}
}

func TestNewBookingNotifier_UnconfiguredIsNoop(t *testing.T) {
	notifier := NewBookingNotifier("", nil)
	// Must not panic and must not attempt network calls.
	notifier.NotifyCustomerDelivered(context.Background(), "cust-1", "book-1", nil)
}
