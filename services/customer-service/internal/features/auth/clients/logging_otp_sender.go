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
		log.Printf("customer_auth_otp type=%s generated=true debug=true", destination.Type)
		return nil
	}

	log.Printf("customer_auth_otp type=%s generated=true", destination.Type)
	return nil
}
