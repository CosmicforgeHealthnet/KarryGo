package messageclients

import (
	"context"
	"errors"
	"log"
)

type LoggingPushSender struct{}

func NewLoggingPushSender() *LoggingPushSender {
	return &LoggingPushSender{}
}

func (s *LoggingPushSender) SendPush(ctx context.Context, message PushMessage) (ProviderResult, error) {
	// Shape only: marketing copy and device tokens are never logged.
	log.Printf("notification_push title_len=%d token_count=%d", len(message.Title), len(message.Tokens))
	providerID := "logged-push"
	return ProviderResult{Provider: "logging_push", ProviderMessageID: &providerID}, nil
}

type LoggingEmailSender struct{}

func NewLoggingEmailSender() *LoggingEmailSender {
	return &LoggingEmailSender{}
}

func (s *LoggingEmailSender) SendEmail(ctx context.Context, message EmailMessage) (ProviderResult, error) {
	// The development fallback records delivery shape, never the address.
	log.Printf("notification_email recipient_configured=%t subject_len=%d", message.To != "", len(message.Subject))
	providerID := "logged-email"
	return ProviderResult{Provider: "logging_email", ProviderMessageID: &providerID}, nil
}

// UnavailableEmailSender never records an unsent email as a success.
type UnavailableEmailSender struct{}

func (UnavailableEmailSender) Ready() error { return errors.New("email delivery is not configured") }
func (s UnavailableEmailSender) SendEmail(context.Context, EmailMessage) (ProviderResult, error) {
	return ProviderResult{Provider: "unavailable_email", Retryable: false}, s.Ready()
}
