package authclients

import "context"

type OTPDestination struct {
	Type  string
	Value string
}

type OTPSender interface {
	SendOTP(ctx context.Context, destination OTPDestination, otp string) error
}
