package main

import (
	"context"
	"cosmicforge/logistics/services/notification-service/internal/config"
	clients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	"testing"
)

func TestEmailLoggingRequiresExplicitDevelopmentMode(t *testing.T) {
	for _, env := range []string{"staging", "production"} {
		sender := buildEmailSender(config.Config{AppEnv: env, EmailProvider: "logging"})
		result, err := sender.SendEmail(context.Background(), clients.EmailMessage{})
		if err == nil || result.ProviderMessageID != nil {
			t.Fatal("non-delivery reported sent")
		}
	}
	if _, ok := buildEmailSender(config.Config{AppEnv: "development", EmailProvider: "logging"}).(*clients.LoggingEmailSender); !ok {
		t.Fatal("explicit development logging unavailable")
	}
	if _, ok := buildEmailSender(config.Config{AppEnv: "development", EmailProvider: "smtp"}).(*clients.SMTPEmailSender); !ok {
		t.Fatal("missing SMTP silently switched to logging")
	}
}
