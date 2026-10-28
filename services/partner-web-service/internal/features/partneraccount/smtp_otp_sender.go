package partneraccount

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/services/partner-web-service/internal/config"
)

const partnerOTPSubject = "Your KarryGo verification code"

var (
	ErrPartnerEmailNotConfigured  = errors.New("partner email not configured")
	ErrPartnerEmailDeliveryFailed = errors.New("partner email delivery failed")
)

// SMTPPartnerEmailSender delivers partner signup OTPs directly through the
// configured cPanel SMTP mailbox. It never logs or exposes SMTP credentials or
// plaintext OTPs.
type SMTPPartnerEmailSender struct {
	cfg config.PartnerSMTPConfig
}

func NewSMTPPartnerEmailSender(cfg config.PartnerSMTPConfig) *SMTPPartnerEmailSender {
	return &SMTPPartnerEmailSender{cfg: cfg}
}

func (s *SMTPPartnerEmailSender) SendPartnerOTP(ctx context.Context, toEmail string, otpCode string) error {
	cfg := s.normalizedConfig()
	if cfg.Host == "" || cfg.Port <= 0 || cfg.Username == "" || cfg.Password == "" || cfg.From == "" {
		return ErrPartnerEmailNotConfigured
	}
	if cfg.TLSMode != "tls" && cfg.TLSMode != "starttls" {
		return ErrPartnerEmailNotConfigured
	}

	message := partnerOTPEmailMessage(cfg.FromName, cfg.From, toEmail, otpCode)
	if err := sendSMTPMessage(ctx, cfg, toEmail, message); err != nil {
		return fmt.Errorf("%w: %v", ErrPartnerEmailDeliveryFailed, err)
	}
	return nil
}

func (s *SMTPPartnerEmailSender) normalizedConfig() config.PartnerSMTPConfig {
	cfg := s.cfg
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Username = strings.TrimSpace(cfg.Username)
	// PARTNER_SMTP_FROM may be a bare address or "Name <address>". Only a
	// username that is itself an address can stand in for it: providers such
	// as Resend authenticate with a fixed username ("resend") that is not a
	// sender, and using it would put an invalid MAIL FROM on the wire.
	from := strings.TrimSpace(cfg.From)
	if from == "" {
		from = cfg.Username
	}
	cfg.From = ""
	if parsed, err := mail.ParseAddress(from); err == nil {
		cfg.From = parsed.Address
		if parsed.Name != "" {
			cfg.FromName = parsed.Name
		}
	}
	if strings.TrimSpace(cfg.FromName) == "" {
		cfg.FromName = "KarryGo"
	}
	cfg.TLSMode = strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	if cfg.TLSMode == "" {
		if cfg.Port == 587 {
			cfg.TLSMode = "starttls"
		} else {
			cfg.TLSMode = "tls"
		}
	}
	return cfg
}

func sendSMTPMessage(ctx context.Context, cfg config.PartnerSMTPConfig, toEmail string, message []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if deadline, ok := ctx.Deadline(); ok {
		dialer.Deadline = deadline
	}

	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	var err error
	if cfg.TLSMode == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()

	if cfg.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			return err
		}
	}

	if err := client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
		return err
	}
	if err := client.Mail(cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(toEmail); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func partnerOTPEmailMessage(fromName string, fromEmail string, toEmail string, otpCode string) []byte {
	return partnerOTPEmailMessageAt(fromName, fromEmail, toEmail, otpCode, time.Now())
}

// partnerOTPEmailMessageAt builds the RFC 5322 message. Date and Message-ID
// are set here so receiving providers (and Resend SMTP) never see a message
// without them; missing Date headers raise spam scores.
func partnerOTPEmailMessageAt(fromName string, fromEmail string, toEmail string, otpCode string, now time.Time) []byte {
	from := (&mail.Address{Name: fromName, Address: fromEmail}).String()
	to := (&mail.Address{Address: toEmail}).String()
	body := fmt.Sprintf("Your KarryGo verification code is %s. It expires in 10 minutes.", otpCode)

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", from)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Date: %s\r\n", now.UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "Message-ID: %s\r\n", partnerMessageID(fromEmail))
	fmt.Fprintf(&msg, "Subject: %s\r\n", partnerOTPSubject)
	fmt.Fprint(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprint(&msg, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprint(&msg, "\r\n")
	fmt.Fprintf(&msg, "%s\r\n", body)
	return msg.Bytes()
}

// partnerMessageID returns <random@sender-domain>; the sender domain keeps the
// identifier aligned with the From address.
func partnerMessageID(fromEmail string) string {
	domain := "karryam.invalid"
	if at := strings.LastIndex(fromEmail, "@"); at >= 0 && at < len(fromEmail)-1 {
		domain = strings.ToLower(strings.TrimSpace(fromEmail[at+1:]))
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("<%d.partner-otp@%s>", time.Now().UnixNano(), domain)
	}
	return fmt.Sprintf("<%s.partner-otp@%s>", hex.EncodeToString(buf), domain)
}
