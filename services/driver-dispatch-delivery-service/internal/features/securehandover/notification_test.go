package securehandover

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/notifications"
)

type notificationRecorder struct{ request notifications.Request }

func (r *notificationRecorder) Send(_ context.Context, request notifications.Request) (notifications.SendResponse, error) {
	r.request = request
	return notifications.SendResponse{MessageID: "message-1", Status: "accepted"}, nil
}

func TestNotificationTransportUsesExistingSecretTransportContract(t *testing.T) {
	recorder := &notificationRecorder{}
	transport := NewNotificationTransport(recorder)
	reference, state, channels, err := transport.Deliver(context.Background(), SecretDeliveryIntent{
		Challenge: Challenge{ChallengeRef: "handover-1", Version: 2, ExpiresAt: time.Now().Add(time.Minute), IntendedRecipientRef: "receiver-hash"},
		Context:   Context{CustomerID: "customer-1", ReceiverEmail: "receiver@example.test", ReceiverPhone: "+15551234567"},
		OTP:       "123456", QRToken: "opaque-qr-token", Channels: []string{notifications.ChannelEmail, notifications.ChannelInApp, notifications.ChannelPush},
	})
	if err != nil || reference != "message-1" || state != "ACCEPTED" || len(channels) != 3 {
		t.Fatalf("delivery = %q %q %v %v", reference, state, channels, err)
	}
	if recorder.request.TemplateKey != templateDeliveryCode || recorder.request.EventType != eventDeliveryChallenge {
		t.Fatalf("notification contract = %+v", recorder.request)
	}
	if recorder.request.Recipient.Email != "receiver@example.test" || recorder.request.Recipient.Phone != "" || recorder.request.Recipient.ID == "customer-1" {
		t.Fatalf("recipient = %+v", recorder.request.Recipient)
	}
	if recorder.request.TemplateData["challenge_ref"] != "handover-1" || recorder.request.TemplateData["code"] != nil || recorder.request.TemplateData["qr_token"] != nil {
		t.Fatal("notification must carry a challenge reference without a credential")
	}
	if err := recorder.request.Validate(); err != nil {
		t.Fatalf("safe notification rejected: %v", err)
	}
	for _, channel := range recorder.request.Channels {
		if channel == "sms" {
			t.Fatal("SMS was falsely activated")
		}
	}
}

func TestNotificationIdempotencyBindsChallengeVersion(t *testing.T) {
	recorder := &notificationRecorder{}
	transport := NewNotificationTransport(recorder)
	intent := SecretDeliveryIntent{Challenge: Challenge{ChallengeRef: "handover-1", Version: 1, IntendedRecipientRef: "receiver-hash"}, Context: Context{CustomerID: "customer-1"}, OTP: "123456", Channels: []string{notifications.ChannelInApp}}
	if _, _, _, err := transport.Deliver(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	first := recorder.request.IDempotencyKey
	if _, _, _, err := transport.Deliver(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	if recorder.request.IDempotencyKey != first {
		t.Fatal("same challenge version changed idempotency identity")
	}
	intent.Challenge.Version = 2
	if _, _, _, err := transport.Deliver(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	if recorder.request.IDempotencyKey == first {
		t.Fatal("new challenge version reused the prior notification identity")
	}
}

func TestNotificationRecoveryRejectsChangedContext(t *testing.T) {
	cipher, err := newCredentialCipher([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	s := &Service{cipher: cipher, now: func() time.Time { return now }}
	current := Context{TripID: "trip", MissionRef: "mission", ProviderID: "provider", ManifestRef: "manifest", ManifestVersion: 1, SourceVersion: 2, ReceiverPhone: "+2348011111111"}
	challenge := Challenge{Stage: StageDelivery, Status: StatusActive, ExpiresAt: now.Add(time.Minute), TripID: current.TripID, MissionRef: current.MissionRef, AssignedProviderID: current.ProviderID, ManifestRef: current.ManifestRef, ManifestVersion: 1, SourceVersion: 2, IntendedRecipientRef: s.deliveryRecipientRef(current)}
	if !s.notificationContextMatches(challenge, current) {
		t.Fatal("valid context rejected")
	}
	for _, change := range []func(*Context){func(c *Context) { c.ProviderID = "other" }, func(c *Context) { c.ManifestVersion++ }, func(c *Context) { c.ReceiverPhone = "+2348022222222" }, func(c *Context) { c.ReceiverEmail = "new@example.test" }, func(c *Context) { c.SourceVersion++ }} {
		other := current
		change(&other)
		if s.notificationContextMatches(challenge, other) {
			t.Fatal("stale context allowed a secret send")
		}
	}
	challenge.Status = StatusInvalidated
	if s.notificationContextMatches(challenge, current) {
		t.Fatal("invalidated challenge sent")
	}
}
