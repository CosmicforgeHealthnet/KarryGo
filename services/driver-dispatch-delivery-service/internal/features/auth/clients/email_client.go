package authclients

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// EmailClient sends OTP codes via email.
type EmailClient interface {
	SendOTP(ctx context.Context, to, code string) error
}

// NoopEmailClient discards all sends. Used when SMTP is not configured.
type NoopEmailClient struct{}

func (NoopEmailClient) SendOTP(_ context.Context, _, _ string) error { return nil }

// smtpDeadline is the maximum wall-clock time for a single email delivery,
// covering both the TLS dial and all subsequent SMTP commands.
const smtpDeadline = 15 * time.Second

// CpanelEmailClient delivers OTP email via cPanel/Exim SMTP using implicit TLS
// (port 465). Uses Go standard library net/smtp — no external email packages.
type CpanelEmailClient struct {
	Host     string // SMTP hostname, e.g. "mail.example.com"
	Port     int    // 465 for implicit TLS
	User     string // SMTP login username
	Password string // SMTP login password
	From     string // Envelope + header From address
}

// SendOTP dials the SMTP server via implicit TLS (port 465) and sends the
// OTP code to the recipient. The total operation is bounded by smtpDeadline
// so a hung SMTP server cannot block the calling goroutine indefinitely.
// Returns an error if delivery fails; the caller is expected to log and
// continue — email failure is non-fatal.
func (c *CpanelEmailClient) SendOTP(_ context.Context, to, code string) error {
	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	deadline := time.Now().Add(smtpDeadline)

	// Dial with an absolute deadline shared by both the TLS handshake and all
	// subsequent SMTP commands (AUTH, MAIL FROM, RCPT TO, DATA, QUIT).
	conn, err := tls.DialWithDialer(
		&net.Dialer{Deadline: deadline},
		"tcp", addr,
		&tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12},
	)
	if err != nil {
		return fmt.Errorf("smtp tls dial %s: %w", addr, err)
	}
	// Apply the same deadline to all subsequent SMTP operations on the connection.
	_ = conn.SetDeadline(deadline)

	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp new client: %w", err)
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

	subject := "Your KarryGo Verification Code"
	body := fmt.Sprintf(
		"Your KarryGo OTP code is: %s\r\n\r\nThis code expires in 10 minutes. Do not share it with anyone.",
		code,
	)
	msg := strings.Join([]string{
		"From: " + c.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	if _, err := fmt.Fprint(w, msg); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close data writer: %w", err)
	}

	return client.Quit()
}
