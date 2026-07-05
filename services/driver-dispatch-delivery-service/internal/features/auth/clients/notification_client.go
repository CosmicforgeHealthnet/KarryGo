package authclients

import (
	"context"
	"errors"
)

type NotificationClient interface {
	SendOTP(ctx context.Context, phoneNumber string, otp string) error
}

var ErrSMSNotConfigured = errors.New("sms delivery is not configured")

type LoggingNotificationClient struct {
	debugOTP bool
}

func NewLoggingNotificationClient(debugOTP bool) *LoggingNotificationClient {
	return &LoggingNotificationClient{debugOTP: debugOTP}
}

// SendOTP does not deliver SMS. It returns ErrSMSNotConfigured so callers can
// log an explicit skip instead of treating local development logging as a real
// provider response.
func (c *LoggingNotificationClient) SendOTP(ctx context.Context, phoneNumber string, otp string) error {
	return ErrSMSNotConfigured
}
