package messageclients

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"cosmicforge/logistics/services/notification-service/internal/config"
)

type SMTPEmailSender struct {
	cfg config.SMTPConfig
}

func NewSMTPEmailSender(cfg config.SMTPConfig) *SMTPEmailSender {
	return &SMTPEmailSender{cfg: cfg}
}

func (s *SMTPEmailSender) SendEmail(ctx context.Context, message EmailMessage) (ProviderResult, error) {
	if s.cfg.Host == "" || s.cfg.Username == "" || s.cfg.Password == "" || s.cfg.From == "" {
		return ProviderResult{Provider: "smtp", Retryable: false}, fmt.Errorf("smtp email is not configured")
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	client, err := s.connect(addr)
	if err != nil {
		return ProviderResult{Provider: "smtp", Retryable: true}, err
	}
	defer client.Close()

	host, _, _ := net.SplitHostPort(addr)
	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, host)
		if err := client.Auth(auth); err != nil {
			return ProviderResult{Provider: "smtp", Retryable: true}, err
		}
	}

	if err := client.Mail(s.cfg.From); err != nil {
		return ProviderResult{Provider: "smtp", Retryable: true}, err
	}
	if err := client.Rcpt(message.To); err != nil {
		return ProviderResult{Provider: "smtp", Retryable: false}, err
	}

	writer, err := client.Data()
	if err != nil {
		return ProviderResult{Provider: "smtp", Retryable: true}, err
	}
	_, err = writer.Write([]byte(formatEmail(s.cfg.From, message)))
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return ProviderResult{Provider: "smtp", Retryable: true}, err
	}

	providerID := "smtp"
	return ProviderResult{Provider: "smtp", ProviderMessageID: &providerID}, nil
}

func (s *SMTPEmailSender) connect(addr string) (*smtp.Client, error) {
	host, _, _ := net.SplitHostPort(addr)
	switch strings.ToLower(s.cfg.TLSMode) {
	case "tls":
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, host)
	case "none":
		return smtp.Dial(addr)
	default:
		client, err := smtp.Dial(addr)
		if err != nil {
			return nil, err
		}
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				_ = client.Close()
				return nil, err
			}
		}
		return client, nil
	}
}

func formatEmail(from string, message EmailMessage) string {
	return strings.Join([]string{
		"From: " + from,
		"To: " + message.To,
		"Subject: " + message.Subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		message.Body,
	}, "\r\n")
}
