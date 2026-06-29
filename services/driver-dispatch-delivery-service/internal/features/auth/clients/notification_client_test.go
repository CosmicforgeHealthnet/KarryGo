package authclients

import (
	"bytes"
	"context"
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
	if err := client.SendOTP(context.Background(), "+2348012345678", "123456"); err != nil {
		t.Fatalf("SendOTP() error = %v", err)
	}
	if strings.Contains(buf.String(), "123456") || strings.Contains(buf.String(), "otp=") {
		t.Fatalf("production logging must not include OTP, got %q", buf.String())
	}
}

// TestLoggingNotificationClientSendOTPIsNoOp verifies that SendOTP does not
// emit OTP log lines even when debug mode is enabled.  OTP logging (with the
// purpose label "signup" / "login") is now the responsibility of the usecase
// layer, which covers both the phone and email channels in one place.
func TestLoggingNotificationClientSendOTPIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	originalOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(originalOutput) })

	client := NewLoggingNotificationClient(true)
	if err := client.SendOTP(context.Background(), "+2348012345678", "123456"); err != nil {
		t.Fatalf("SendOTP() error = %v", err)
	}
	if strings.Contains(buf.String(), "otp=") {
		t.Fatalf("SendOTP must not log OTP (logging moved to usecase layer), got %q", buf.String())
	}
}
