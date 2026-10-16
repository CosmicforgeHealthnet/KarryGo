package messageclients

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"cosmicforge/logistics/services/notification-service/internal/config"
)

type SMTPEmailSender struct{ cfg config.SMTPConfig }

func NewSMTPEmailSender(cfg config.SMTPConfig) *SMTPEmailSender { return &SMTPEmailSender{cfg: cfg} }

func (s *SMTPEmailSender) Ready() error {
	if s.cfg.Host == "" || s.cfg.Username == "" || s.cfg.Password == "" || s.cfg.From == "" {
		return fmt.Errorf("smtp email is not configured")
	}
	if s.cfg.Port < 1 || s.cfg.Port > 65535 {
		return fmt.Errorf("smtp port is invalid")
	}
	if _, err := mail.ParseAddress(s.cfg.From); err != nil {
		return fmt.Errorf("smtp sender address is invalid")
	}
	switch strings.ToLower(s.cfg.TLSMode) {
	case "", "tls", "starttls", "none":
		return nil
	default:
		return fmt.Errorf("smtp TLS mode is invalid")
	}
}

func (s *SMTPEmailSender) SendEmail(ctx context.Context, message EmailMessage) (ProviderResult, error) {
	failure := func(err error) (ProviderResult, error) {
		retry := true
		var reply *textproto.Error
		if errors.As(err, &reply) && reply.Code >= 500 {
			retry = false
		}
		return ProviderResult{Provider: "smtp", Retryable: retry}, err
	}
	if err := s.Ready(); err != nil {
		return ProviderResult{Provider: "smtp", Retryable: false}, err
	}
	to, err := mail.ParseAddress(message.To)
	if err != nil {
		return ProviderResult{Provider: "smtp", Retryable: false}, fmt.Errorf("recipient email is invalid")
	}
	from, _ := mail.ParseAddress(s.cfg.From)
	timeout := s.cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client, err := s.connect(ctx)
	if err != nil {
		return failure(err)
	}
	defer client.Close()
	stop := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stop()
	if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
		return failure(err)
	}
	if err := client.Mail(from.Address); err != nil {
		return failure(err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return failure(err)
	}
	writer, err := client.Data()
	if err != nil {
		return failure(err)
	}
	_, err = writer.Write([]byte(formatEmail(from.String(), message)))
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return failure(err)
	}
	providerID := "smtp"
	return ProviderResult{Provider: "smtp", ProviderMessageID: &providerID}, nil
}

func (s *SMTPEmailSender) connect(ctx context.Context) (*smtp.Client, error) {
	host := s.cfg.Host
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(s.cfg.Port)))
	if err != nil {
		return nil, err
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	mode := strings.ToLower(s.cfg.TLSMode)
	if mode == "tls" {
		secure := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := secure.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, err
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if mode == "" || mode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close()
			return nil, fmt.Errorf("smtp server does not support required STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			_ = client.Close()
			return nil, err
		}
	}
	return client, nil
}

func formatEmail(from string, message EmailMessage) string {
	clean := func(value string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(value) }
	return strings.Join([]string{"From: " + clean(from), "To: " + clean(message.To),
		"Subject: " + clean(message.Subject), "MIME-Version: 1.0", "Content-Type: text/plain; charset=UTF-8", "", message.Body}, "\r\n")
}
