package authclients

import (
	"context"
	"fmt"
	"time"

	"cosmicforge/logistics/shared/go/notifications"
)

type NotificationEmailOTPSender struct {
	client   notifications.Client
	fallback OTPSender
	now      func() time.Time
}

func NewNotificationEmailOTPSender(client notifications.Client, fallback OTPSender) *NotificationEmailOTPSender {
	return &NotificationEmailOTPSender{
		client:   client,
		fallback: fallback,
		now:      time.Now,
	}
}

func (s *NotificationEmailOTPSender) SendOTP(ctx context.Context, destination OTPDestination, otp string) error {
	if destination.Type != "email" {
		if s.fallback == nil {
			return nil
		}
		return s.fallback.SendOTP(ctx, destination, otp)
	}

	_, err := s.client.Send(ctx, notifications.Request{
		IDempotencyKey: fmt.Sprintf("customer-service:auth-otp:%s:%d", destination.Value, s.now().UnixNano()),
		SourceService:  "customer-service",
		EventType:      "customer.auth.otp",
		Recipient: notifications.Recipient{
			Type:  "customer",
			ID:    destination.Value,
			Email: destination.Value,
		},
		Channels: []string{notifications.ChannelEmail},
		Title:    "Your Cosmicforge Logistics verification code",
		Body:     fmt.Sprintf("Your verification code is %s. It expires shortly.", otp),
		Priority: notifications.PriorityHigh,
	})
	return err
}
