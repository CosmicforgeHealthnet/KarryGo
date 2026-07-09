package authclients

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestLoggingNotificationClientDoesNotLogOTPWhenDebugDisabled(t *testing.T) {
	var buf bytes.Buffer
	originalOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(originalOutput) })

	client := NewLoggingNotificationClient(false)
	if err := client.SendOTP(context.Background(), "+2348012345678", "123456"); !errors.Is(err, ErrSMSNotConfigured) {
		t.Fatalf("SendOTP() error = %v, want ErrSMSNotConfigured", err)
	}
	if strings.Contains(buf.String(), "123456") || strings.Contains(buf.String(), "otp=") {
		t.Fatalf("production logging must not include OTP, got %q", buf.String())
	}
}

// TestLoggingNotificationClientSendOTPReportsNotConfigured verifies that the
// local client does not pretend that SMS delivery happened. OTP debug logging
// remains the responsibility of the usecase layer.
func TestLoggingNotificationClientSendOTPReportsNotConfigured(t *testing.T) {
	var buf bytes.Buffer
	originalOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(originalOutput) })

	client := NewLoggingNotificationClient(true)
	if err := client.SendOTP(context.Background(), "+2348012345678", "123456"); !errors.Is(err, ErrSMSNotConfigured) {
		t.Fatalf("SendOTP() error = %v, want ErrSMSNotConfigured", err)
	}
	if strings.Contains(buf.String(), "otp=") {
		t.Fatalf("SendOTP must not log OTP (logging moved to usecase layer), got %q", buf.String())
	}
}
