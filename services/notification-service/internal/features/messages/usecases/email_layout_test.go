package messageusecases

import (
	"context"
	"testing"

	"cosmicforge/logistics/shared/go/emailbrand"
	"cosmicforge/logistics/shared/go/notifications"
)

// The producer's structured layout is stored with the message and handed to
// the email sender at delivery time, so a retry renders the same email.
func TestEmailDeliveryCarriesStructuredLayout(t *testing.T) {
	repo := newFakeRepository()
	queue := &fakeQueue{}
	service := newTestService(repo, queue)
	layout := emailbrand.Layout{Product: "Karryam Fleet", Heading: "Join Cosmicforge",
		Action: &emailbrand.Action{Label: "Accept invitation", URL: "https://fleet.example.test/sign-up?token=t"}}
	_, err := service.Send(context.Background(), notifications.Request{
		IDempotencyKey: "fleet-layout", SourceService: "fleet-partner-service",
		EventType: "corporate.staff.invitation", Channels: []string{notifications.ChannelEmail},
		Recipient: notifications.Recipient{Type: "corporate_user", ID: "ada@example.com", Email: "ada@example.com"},
		Title:     "You have been invited", Body: "You have been invited to join Cosmicforge.",
		Data: map[string]interface{}{emailbrand.LayoutDataKey: layout},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessDelivery(context.Background(), queue.enqueued[0]); err != nil {
		t.Fatal(err)
	}
	messages := service.email.(*fakeEmailSender).messages
	if len(messages) != 1 || messages[0].Layout == nil || messages[0].Layout.Heading != "Join Cosmicforge" ||
		messages[0].Layout.Action == nil || messages[0].Layout.Action.URL != layout.Action.URL {
		t.Fatalf("structured layout lost at delivery: %+v", messages)
	}
}

// A malformed layout never drops the email: it is sent from subject and body.
func TestMalformedEmailLayoutFallsBackToBody(t *testing.T) {
	repo := newFakeRepository()
	queue := &fakeQueue{}
	service := newTestService(repo, queue)
	_, err := service.Send(context.Background(), notifications.Request{
		IDempotencyKey: "fleet-bad-layout", SourceService: "fleet-partner-service",
		EventType: "corporate.staff.invitation", Channels: []string{notifications.ChannelEmail},
		Recipient: notifications.Recipient{Type: "corporate_user", ID: "ada@example.com", Email: "ada@example.com"},
		Title:     "You have been invited", Body: "You have been invited to join Cosmicforge.",
		Data: map[string]interface{}{emailbrand.LayoutDataKey: map[string]interface{}{"intro": []string{"no heading"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessDelivery(context.Background(), queue.enqueued[0]); err != nil {
		t.Fatal(err)
	}
	messages := service.email.(*fakeEmailSender).messages
	if len(messages) != 1 || messages[0].Layout != nil || messages[0].Body == "" {
		t.Fatalf("malformed layout must fall back to the body: %+v", messages)
	}
}
