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

func TestLoggingNotificationClientLogsOTPOnlyWhenDebugEnabled(t *testing.T) {
	var buf bytes.Buffer
	originalOutput := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(originalOutput) })

	client := NewLoggingNotificationClient(true)
	if err := client.SendOTP(context.Background(), "+2348012345678", "123456"); err != nil {
		t.Fatalf("SendOTP() error = %v", err)
	}
	if !strings.Contains(buf.String(), "otp=123456") {
		t.Fatalf("debug logging should include OTP, got %q", buf.String())
	}
}
