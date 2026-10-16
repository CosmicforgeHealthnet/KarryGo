package vendorauthclients

import (
	"context"
	"cosmicforge/logistics/shared/go/notifications"
	"strings"
	"testing"
)

type recordingOTPNotifier struct{ requests []notifications.Request }

func (n *recordingOTPNotifier) Send(_ context.Context, r notifications.Request) (notifications.SendResponse, error) {
	n.requests = append(n.requests, r)
	return notifications.SendResponse{Status: "QUEUED"}, r.Validate()
}
func TestOTPResendDoesNotReusePreviousNotification(t *testing.T) {
	recorder := &recordingOTPNotifier{}
	notifier := NewOTPNotifier(recorder)
	for _, code := range []string{"123456", "654321"} {
		if err := notifier.SendOTP(context.Background(), "email", "owner@example.com", "login", code); err != nil {
			t.Fatal(err)
		}
	}
	if len(recorder.requests) != 2 || recorder.requests[0].IDempotencyKey == recorder.requests[1].IDempotencyKey {
		t.Fatal("new code deduplicated against previous code")
	}
	if !strings.Contains(recorder.requests[1].Body, "654321") || strings.Contains(recorder.requests[1].IDempotencyKey, "owner@example.com") {
		t.Fatal("wrong code or identifier in delivery key")
	}
	if err := notifier.SendOTP(context.Background(), "phone", "+2348000000000", "login", "123456"); err == nil {
		t.Fatal("email transport claimed SMS support")
	}
}
