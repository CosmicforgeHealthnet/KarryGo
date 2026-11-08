package partneraccount

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cosmicforge/logistics/services/partner-web-service/internal/config"
)

func resendConfig(from, fromName string) config.PartnerSMTPConfig {
	return config.PartnerSMTPConfig{Host: "smtp.resend.com", Port: 465, Username: "resend", Password: "test-only", From: from, FromName: fromName, TLSMode: "tls"}
}

func TestPartnerSMTPFromAcceptsDisplayNameForm(t *testing.T) {
	cfg := NewSMTPPartnerEmailSender(resendConfig("Karryam <no-reply@karryam.com>", "KarryGo")).normalizedConfig()
	if cfg.From != "no-reply@karryam.com" {
		t.Fatalf("envelope sender = %q, want the bare address", cfg.From)
	}
	if cfg.FromName != "Karryam" {
		t.Fatalf("display name = %q, want the one written in PARTNER_SMTP_FROM", cfg.FromName)
	}
	raw, err := partnerOTPEmailMessage(cfg.FromName, cfg.From, "partner@example.com", "123456")
	if err != nil {
		t.Fatal(err)
	}
	header := string(raw)
	if !strings.Contains(header, "From: \"Karryam\" <no-reply@karryam.com>\r\n") {
		t.Fatalf("unexpected From header in %q", header)
	}
}

func TestPartnerSMTPBareFromUsesOfficialName(t *testing.T) {
	cfg := NewSMTPPartnerEmailSender(resendConfig("no-reply@karryam.com", "Karryam")).normalizedConfig()
	if cfg.From != "no-reply@karryam.com" || cfg.FromName != "Karryam" {
		t.Fatalf("got %q / %q", cfg.From, cfg.FromName)
	}
}

func TestPartnerSMTPOverridesLegacyNamesWithoutChangingAddress(t *testing.T) {
	for _, from := range []string{"no-reply@karryam.com", "KarryGo <no-reply@karryam.com>", "Cosmicforge Logistics <no-reply@karryam.com>"} {
		cfg := NewSMTPPartnerEmailSender(resendConfig(from, "KarryGo")).normalizedConfig()
		if cfg.From != "no-reply@karryam.com" || cfg.FromName != "Karryam" {
			t.Fatal("legacy sender survived normalization")
		}
	}
}

func TestPartnerSMTPNeverUsesANonAddressUsernameAsSender(t *testing.T) {
	sender := NewSMTPPartnerEmailSender(resendConfig("", "Karryam"))
	if got := sender.normalizedConfig().From; got != "" {
		t.Fatalf("From = %q, want empty (the Resend username is not a sender)", got)
	}
	// Fails closed before any network call.
	if err := sender.SendPartnerOTP(context.Background(), "partner@example.com", "123456"); !errors.Is(err, ErrPartnerEmailNotConfigured) {
		t.Fatalf("err = %v, want ErrPartnerEmailNotConfigured", err)
	}
}

func TestPartnerSMTPAddressUsernameStillWorksAsSender(t *testing.T) {
	cfg := config.PartnerSMTPConfig{Host: "mail.example.com", Port: 465, Username: "no-reply@example.com", Password: "x", FromName: "KarryGo", TLSMode: "tls"}
	if got := NewSMTPPartnerEmailSender(cfg).normalizedConfig().From; got != "no-reply@example.com" {
		t.Fatalf("From = %q", got)
	}
}

func TestPartnerSMTPFailureIsSafeAndNeverReportedAsDelivered(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewSMTPPartnerEmailSender(resendConfig("no-reply@karryam.com", "KarryGo")).SendPartnerOTP(ctx, "fixture@example.com", "012345")
	if !errors.Is(err, ErrPartnerEmailDeliveryFailed) || err.Error() != "partner email delivery failed" {
		t.Fatal("failed SMTP attempt exposed details or reported success")
	}
}
