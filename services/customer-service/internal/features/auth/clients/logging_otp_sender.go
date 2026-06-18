package authclients

import (
	"context"
	"log"
)

type LoggingOTPSender struct {
	debug bool
}

func NewLoggingOTPSender(debug bool) *LoggingOTPSender {
	return &LoggingOTPSender{debug: debug}
}

func (s *LoggingOTPSender) SendOTP(ctx context.Context, destination OTPDestination, otp string) error {
	if s.debug {
		log.Printf("customer_auth_otp type=%s value=%s otp=%s", destination.Type, destination.Value, otp)
		return nil
	}

	log.Printf("customer_auth_otp type=%s value=%s generated=true", destination.Type, destination.Value)
	return nil
}
