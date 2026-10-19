package providerauthclients

import (
	"context"
	"fmt"
	"time"

	"cosmicforge/logistics/shared/go/notifications"
)

// EmailOTPSender delivers a driver's login code by email through
// notification-service. Phone codes are not sent: the platform has no SMS
// provider yet, so a phone login only works with debug codes.
type EmailOTPSender struct {
	client notifications.Client
	now    func() time.Time
}

func NewEmailOTPSender(client notifications.Client) *EmailOTPSender {
	return &EmailOTPSender{client: client, now: time.Now}
}

// SendLoginCode sends code to an email identifier. Other identifier types
// are a no-op (there is no channel to deliver them on).
func (s *EmailOTPSender) SendLoginCode(ctx context.Context, identifierType, identifier, code string) error {
	if identifierType != "email" {
		return nil
	}
	_, err := s.client.Send(ctx, notifications.Request{
		IDempotencyKey: fmt.Sprintf("driver-taxi-service:auth-otp:%s:%d", identifier, s.now().UnixNano()),
		SourceService:  "driver-taxi-service",
		EventType:      "provider.auth.otp",
		Recipient: notifications.Recipient{
			Type:  notifications.RecipientProvider,
			ID:    identifier,
			Email: identifier,
		},
		Channels: []string{notifications.ChannelEmail},
		Title:    "Your KarryGo driver sign-in code",
		Body:     fmt.Sprintf("Your sign-in code is %s. It expires in a few minutes. If you did not ask for it, ignore this email.", code),
		Priority: notifications.PriorityHigh,
	})
	return err
}
