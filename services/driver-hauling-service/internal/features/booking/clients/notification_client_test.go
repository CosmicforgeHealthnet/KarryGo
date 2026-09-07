package bookingclients

import (
	"context"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/notifications"
)

type capturingNotifier struct{ requests []notifications.Request }

func (n *capturingNotifier) Send(_ context.Context, request notifications.Request) (notifications.SendResponse, error) {
	n.requests = append(n.requests, request)
	return notifications.SendResponse{Status: "queued"}, nil
}

func TestDeliveryCredentialVersionsUseSafeDistinctNotificationKeys(t *testing.T) {
	capture := &capturingNotifier{}
	notifier := NewBookingNotifierWith(capture)
	for _, ref := range []string{"challenge-a", "challenge-b"} {
		notifier.NotifyCustomerDeliveryCodeReady(context.Background(), "customer", "booking", map[string]interface{}{
			"booking_id": "booking", "challenge_ref": ref,
			"code": "123456", "qr_token": "opaque-secret-token",
		})
	}
	if len(capture.requests) != 2 || capture.requests[0].IDempotencyKey == capture.requests[1].IDempotencyKey {
		t.Fatal("credential versions must be independently deliverable")
	}
	for _, request := range capture.requests {
		if strings.Contains(request.IDempotencyKey, "123456") || strings.Contains(request.IDempotencyKey, "opaque-secret-token") {
			t.Fatal("notification idempotency key contains a bearer secret")
		}
	}
}
