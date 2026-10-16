package authclients

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// EmailClient sends OTP codes and generic app notifications via email.
type EmailClient interface {
	SendOTP(ctx context.Context, to, code string) error
	SendEmail(ctx context.Context, to, subject, textBody, htmlBody string) error
}

var ErrEmailNotConfigured = errors.New("email delivery is not configured")

// NoopEmailClient is kept for tests/backward compatibility. It reports that
// email is unavailable instead of silently discarding an OTP.
type NoopEmailClient struct{}

func (NoopEmailClient) SendOTP(_ context.Context, _, _ string) error {
	return ErrEmailNotConfigured
}

func (NoopEmailClient) SendEmail(_ context.Context, _, _, _, _ string) error {
	return ErrEmailNotConfigured
}

// smtpDeadline is the maximum wall-clock time for a single email delivery,
// covering both the TLS dial and all subsequent SMTP commands.
const smtpDeadline = 15 * time.Second

// Deprecated: runtime email delivery uses NotificationEmailClient.
// CpanelEmailClient remains for compatibility with existing adapters and tests. It supports
// implicit TLS (usually port 465) and STARTTLS (usually port 587).
type CpanelEmailClient struct {
	Host     string // SMTP hostname, e.g. "mail.example.com"
	Port     int    // 465 for implicit TLS, 587 for STARTTLS
	User     string // SMTP login username
	Password string // SMTP login password
	From     string // Envelope + header From address
	TLSMode  string // "tls", "starttls", or "none"; empty auto-detects from Port
}

// SendOTP sends the OTP code using the generic cPanel SMTP sender.
func (c *CpanelEmailClient) SendOTP(ctx context.Context, to, code string) error {
	subject := "Your KarryGo Verification Code"
	body := fmt.Sprintf(
		"Your KarryGo OTP code is: %s\r\n\r\nThis code expires in 10 minutes. Do not share it with anyone.",
		code,
	)
	return c.SendEmail(ctx, to, subject, body, "")
}

// SendEmail sends a generic text or multipart text/html email through cPanel SMTP.
// The total operation is bounded by smtpDeadline so a hung SMTP server cannot
// block the request indefinitely.
func (c *CpanelEmailClient) SendEmail(ctx context.Context, to, subject, textBody, htmlBody string) error {
	if c.Host == "" || c.User == "" || c.Password == "" || c.From == "" {
		return ErrEmailNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.Now().Add(smtpDeadline)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	client, err := c.connect(ctx, deadline)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Auth(smtp.PlainAuth("", c.User, c.Password, c.Host)); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(c.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}

	msg := buildEmailMessage(c.From, to, subject, textBody, htmlBody)
	if _, err := fmt.Fprint(w, msg); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data writer: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit: %w", err)
	}
	return nil
}

func buildEmailMessage(from, to, subject, textBody, htmlBody string) string {
	headers := []string{
		"From: " + sanitizeHeader(from),
		"To: " + sanitizeHeader(to),
		"Subject: " + sanitizeHeader(subject),
		"MIME-Version: 1.0",
	}
	if strings.TrimSpace(htmlBody) == "" {
		return strings.Join(append(headers,
			"Content-Type: text/plain; charset=UTF-8",
			"",
			textBody,
		), "\r\n")
	}

	boundary := "karrygo-email-boundary"
	lines := append(headers,
		`Content-Type: multipart/alternative; boundary="`+boundary+`"`,
		"",
		"--"+boundary,
		"Content-Type: text/plain; charset=UTF-8",
		"",
		textBody,
		"--"+boundary,
		"Content-Type: text/html; charset=UTF-8",
		"",
		htmlBody,
		"--"+boundary+"--",
	)
	return strings.Join(lines, "\r\n")
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func (c *CpanelEmailClient) connect(ctx context.Context, deadline time.Time) (*smtp.Client, error) {
	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	mode := strings.ToLower(strings.TrimSpace(c.TLSMode))
	if mode == "" {
		if c.Port == 465 {
			mode = "tls"
		} else {
			mode = "starttls"
		}
	}

	switch mode {
	case "tls", "implicit_tls":
		conn, err := tls.DialWithDialer(
			&net.Dialer{Deadline: deadline},
			"tcp", addr,
			&tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12},
		)
		if err != nil {
			return nil, fmt.Errorf("smtp tls dial %s: %w", addr, err)
		}
		_ = conn.SetDeadline(deadline)
		client, err := smtp.NewClient(conn, c.Host)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("smtp new client: %w", err)
		}
		return client, nil
	case "none":
		conn, err := (&net.Dialer{Deadline: deadline}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("smtp dial %s: %w", addr, err)
		}
		_ = conn.SetDeadline(deadline)
		client, err := smtp.NewClient(conn, c.Host)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("smtp new client: %w", err)
		}
		return client, nil
	case "starttls":
		conn, err := (&net.Dialer{Deadline: deadline}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("smtp dial %s: %w", addr, err)
		}
		_ = conn.SetDeadline(deadline)
		client, err := smtp.NewClient(conn, c.Host)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("smtp new client: %w", err)
		}
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close()
			return nil, fmt.Errorf("smtp starttls unsupported by %s", addr)
		}
		if err := client.StartTLS(&tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("smtp starttls: %w", err)
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported smtp tls mode %q", c.TLSMode)
	}
}
