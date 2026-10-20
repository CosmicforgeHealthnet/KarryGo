package config

import "testing"

func TestPartnerSMTPDefaultsToImplicitTLS(t *testing.T) {
	t.Setenv("PARTNER_SMTP_HOST", "mail.example.com")
	t.Setenv("PARTNER_SMTP_USERNAME", "no-reply@example.com")
	t.Setenv("PARTNER_SMTP_PASSWORD", "secret")
	t.Setenv("PARTNER_SMTP_PORT", "")
	t.Setenv("PARTNER_SMTP_FROM", "")
	t.Setenv("PARTNER_SMTP_FROM_NAME", "")
	t.Setenv("PARTNER_SMTP_TLS_MODE", "")

	cfg := Load().PartnerSMTP
	if cfg.Port != 465 || cfg.TLSMode != "tls" {
		t.Fatalf("smtp defaults = port %d tls %q, want 465/tls", cfg.Port, cfg.TLSMode)
	}
	if cfg.From != "no-reply@example.com" || cfg.FromName != "KarryGo" {
		t.Fatalf("from defaults = %q/%q", cfg.From, cfg.FromName)
	}
}

func TestPartnerSMTPPort587DefaultsToStartTLS(t *testing.T) {
	t.Setenv("PARTNER_SMTP_PORT", "587")
	t.Setenv("PARTNER_SMTP_TLS_MODE", "")

	cfg := Load().PartnerSMTP
	if cfg.Port != 587 || cfg.TLSMode != "starttls" {
		t.Fatalf("smtp defaults = port %d tls %q, want 587/starttls", cfg.Port, cfg.TLSMode)
	}
}
