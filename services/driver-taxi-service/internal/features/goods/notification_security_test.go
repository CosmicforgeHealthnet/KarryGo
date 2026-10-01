package goods

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/notifications"
)

type custodyNotificationRecorder struct{ requests []notifications.Request }

func (r *custodyNotificationRecorder) Send(_ context.Context, request notifications.Request) (notifications.SendResponse, error) {
	r.requests = append(r.requests, request)
	return notifications.SendResponse{MessageID: "message", Status: "accepted"}, nil
}

func TestTaxiGoodsNotificationsCarryOnlyRetrievalReferences(t *testing.T) {
	recorder := &custodyNotificationRecorder{}
	adapter := NewNotificationAdapter(recorder)
	credential := Credential{ChallengeRef: "challenge-1", OTP: "123456", QRToken: "opaque-secret", ExpiresAt: time.Now().Add(time.Minute)}
	if err := adapter.NotifyDeliveryCredential(context.Background(), "receiver-account", "mission-1", credential); err != nil {
		t.Fatal(err)
	}
	if err := adapter.NotifyPickupCredential(context.Background(), "provider-1", "mission-1", credential); err != nil {
		t.Fatal(err)
	}
	if len(recorder.requests) != 2 {
		t.Fatalf("notifications=%d", len(recorder.requests))
	}
	for _, request := range recorder.requests {
		if request.Data["challenge_ref"] != credential.ChallengeRef || request.Data["otp"] != nil || request.Data["qr_token"] != nil ||
			request.TemplateData["otp"] != nil || request.TemplateData["qr_token"] != nil {
			t.Fatalf("unsafe notification: %+v", request)
		}
		if err := request.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
