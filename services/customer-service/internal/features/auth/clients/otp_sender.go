package authclients

import "context"

type OTPSender interface {
	SendOTP(ctx context.Context, phone string, otp string) error
}
