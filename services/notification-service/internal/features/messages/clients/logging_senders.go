package messageclients

import (
	"context"
	"log"
)

type LoggingPushSender struct{}

func NewLoggingPushSender() *LoggingPushSender {
	return &LoggingPushSender{}
}

func (s *LoggingPushSender) SendPush(ctx context.Context, message PushMessage) (ProviderResult, error) {
	log.Printf("notification_push title=%q token_count=%d", message.Title, len(message.Tokens))
	providerID := "logged-push"
	return ProviderResult{Provider: "logging_push", ProviderMessageID: &providerID}, nil
}

type LoggingEmailSender struct{}

func NewLoggingEmailSender() *LoggingEmailSender {
	return &LoggingEmailSender{}
}

func (s *LoggingEmailSender) SendEmail(ctx context.Context, message EmailMessage) (ProviderResult, error) {
	log.Printf("notification_email to=%s subject=%q", message.To, message.Subject)
	providerID := "logged-email"
	return ProviderResult{Provider: "logging_email", ProviderMessageID: &providerID}, nil
}
