package authclients

import (
	"context"
	"fmt"
	"strings"

	"cosmicforge/logistics/shared/go/notifications"
	"github.com/google/uuid"
)

type NotificationEmailClient struct{ notifier notifications.Notifier }

func NewNotificationEmailClient(notifier notifications.Notifier) *NotificationEmailClient {
	return &NotificationEmailClient{notifier: notifier}
}

func (c *NotificationEmailClient) SendOTP(ctx context.Context, to, code string) error {
	return c.send(ctx, to, "dispatch.auth.otp", "Your Karryam verification code",
		fmt.Sprintf("Your Karryam verification code is %s. It expires shortly. Never share this code.", code))
}

func (c *NotificationEmailClient) SendEmail(ctx context.Context, to, subject, textBody, htmlBody string) error {
	if strings.TrimSpace(textBody) == "" {
		return fmt.Errorf("notification email requires a plain-text body")
	}
	return c.send(ctx, to, "dispatch.email", subject, textBody)
}

func (c *NotificationEmailClient) send(ctx context.Context, to, event, subject, body string) error {
	if c == nil || c.notifier == nil {
		return ErrEmailNotConfigured
	}
	_, err := c.notifier.Send(ctx, notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey("driver-dispatch-delivery-service", event, uuid.NewString()),
		SourceService:  "driver-dispatch-delivery-service", EventType: event,
		Recipient: notifications.Recipient{Type: "dispatch_auth", ID: to, Email: to},
		Channels:  []string{notifications.ChannelEmail}, Title: subject, Body: body,
		Priority: notifications.PriorityHigh,
	})
	return err
}
