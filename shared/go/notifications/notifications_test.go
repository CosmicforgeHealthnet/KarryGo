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
