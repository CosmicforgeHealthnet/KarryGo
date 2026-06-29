package authclients

import "context"

type NotificationClient interface {
	SendOTP(ctx context.Context, phoneNumber string, otp string) error
}

type LoggingNotificationClient struct {
	debugOTP bool
}

func NewLoggingNotificationClient(debugOTP bool) *LoggingNotificationClient {
	return &LoggingNotificationClient{debugOTP: debugOTP}
}

// SendOTP is intentionally a no-op in the local dev environment.
// OTP logging (with the purpose label "signup" / "login") is handled by the
// usecase layer so that both the phone and email channels are covered in a
// single place and the log lines include the correct purpose context.
// A production implementation would call the SMS gateway here.
func (c *LoggingNotificationClient) SendOTP(ctx context.Context, phoneNumber string, otp string) error {
	return nil
}
