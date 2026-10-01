package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultsMatchTheApprovedProgramme(t *testing.T) {
	t.Setenv("REFERRAL_DATABASE_URL", "postgres://x/y")
	cfg := Load()
	p := cfg.Programme
	checks := map[string]bool{
		"₦500 customer referral bonus":      p.CustomerReferralBonusKobo == 50_000,
		"₦200 customer welcome credit":      p.CustomerWelcomeCreditKobo == 20_000,
		"₦500 driver welcome credit":        p.ProviderWelcomeCreditKobo == 50_000,
		"10% commission":                    p.ProviderCommissionBPS == 1_000,
		"5 activation jobs":                 p.ProviderActivationJobs == 5,
		"90-day window":                     p.ProviderWindow == 90*24*time.Hour,
		"5 bookings for customer referrers": p.CustomerActivityThreshold == 5,
		"10 jobs for driver referrers":      p.ProviderActivityThreshold == 10,
		"30-day activity window":            p.ActivityWindow == 30*24*time.Hour,
		"₦7,500 ten-referral milestone":     p.Milestone10BonusKobo == 750_000,
		"review after 3 reversed jobs":      p.ReversalReviewThreshold == 3,
		"welcome credit is spend-only":      !p.ProviderWelcomeWithdrawable,
		"port 8114":                         cfg.HTTPAddr == ":8114",
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("default for %s is wrong", name)
		}
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
}

func TestValidateRejectsHalfConfiguredWallet(t *testing.T) {
	t.Setenv("REFERRAL_WALLET_URL", "http://localhost:8105")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "REFERRAL_WALLET_SECRET") {
		t.Fatalf("expected the missing wallet secret to be reported, got %v", err)
	}
}

func TestValidateRejectsNonsenseNumbers(t *testing.T) {
	t.Setenv("REFERRAL_PROVIDER_COMMISSION_BPS", "20000")
	t.Setenv("REFERRAL_CUSTOMER_BONUS_KOBO", "-1")
	t.Setenv("REFERRAL_PROVIDER_TOKEN_SECRETS", "vendor=x")
	err := Load().Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{"COMMISSION_BPS", "CUSTOMER_BONUS_KOBO", "unknown actor type"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}
