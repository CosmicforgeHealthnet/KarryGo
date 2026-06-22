package notifications

import (
	"errors"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
)

func TestRequestValidateAllowsOptionalChannels(t *testing.T) {
	request := Request{
		IDempotencyKey: "customer-service:test:1",
		SourceService:  "customer-service",
		EventType:      "test.created",
		Recipient: Recipient{
			Type: "customer",
			ID:   "customer-1",
		},
		Title: "Hello",
		Body:  "World",
	}

	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRequestValidateRejectsUnknownChannel(t *testing.T) {
	request := Request{
		IDempotencyKey: "customer-service:test:1",
		SourceService:  "customer-service",
		EventType:      "test.created",
		Recipient: Recipient{
			Type: "customer",
			ID:   "customer-1",
		},
		Title:    "Hello",
		Channels: []string{"fax"},
	}

	err := request.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeValidationFailed {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestIdempotencyKeyIsDeterministic(t *testing.T) {
	first := IdempotencyKey("driver-hauling-service", EventBookingMatched, "booking-1")
	second := IdempotencyKey("driver-hauling-service", EventBookingMatched, "booking-1")

	if first != second {
		t.Fatalf("expected deterministic key, got %q and %q", first, second)
	}
	if want := "driver-hauling-service:booking.matched:booking-1"; first != want {
		t.Fatalf("IdempotencyKey() = %q, want %q", first, want)
	}
}

func TestClientImplementsNotifier(t *testing.T) {
	var _ Notifier = Client{}
}
