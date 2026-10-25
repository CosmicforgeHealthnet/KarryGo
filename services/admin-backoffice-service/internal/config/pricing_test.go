package config

import "testing"

func TestPricingCapabilitiesDefaultDisabledAndNeedBothWriteFlags(t *testing.T) {
	t.Setenv("ADMIN_PRICING_READ_ENABLED", "")
	t.Setenv("ADMIN_PRICING_WRITES_ENABLED", "")
	t.Setenv("ADMIN_REMOTE_WRITES_ENABLED", "")
	flags := loadDomainFlags()
	if flags.PricingWrites || flags.RemoteWrites || flags.WriteFamilyRequested("pricing.policy.activate") {
		t.Fatal("Pricing writes were enabled without explicit independent flags")
	}
	t.Setenv("ADMIN_PRICING_WRITES_ENABLED", "true")
	flags = loadDomainFlags()
	if flags.WriteFamilyRequested("pricing.policy.activate") {
		t.Fatal("domain flag bypassed the global remote-write gate")
	}
	t.Setenv("ADMIN_REMOTE_WRITES_ENABLED", "true")
	flags = loadDomainFlags()
	if !flags.WriteFamilyRequested("pricing.policy.activate") {
		t.Fatal("explicit Pricing and remote-write flags did not request the family")
	}
}
