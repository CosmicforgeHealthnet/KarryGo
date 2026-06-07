package authclients

import (
	"context"
	"log"
)

type NotificationClient interface {
	SendOTP(ctx context.Context, phoneNumber string, otp string) error
}

type LoggingNotificationClient struct {
	debugOTP bool
}

func NewLoggingNotificationClient(debugOTP bool) *LoggingNotificationClient {
	return &LoggingNotificationClient{debugOTP: debugOTP}
}

func (c *LoggingNotificationClient) SendOTP(ctx context.Context, phoneNumber string, otp string) error {
	if c.debugOTP {
		log.Printf("development dispatch rider otp phone_number=%s otp=%s", phoneNumber, otp)
	}

	return nil
}
