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

func (s *LoggingOTPSender) SendOTP(ctx context.Context, phone string, otp string) error {
	if s.debug {
		log.Printf("customer_auth_otp phone=%s otp=%s", phone, otp)
		return nil
	}

	log.Printf("customer_auth_otp phone=%s generated=true", phone)
	return nil
}
