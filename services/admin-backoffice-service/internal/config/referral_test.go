package config

import "testing"

func TestReferralCommandsDefaultDisabledAndNeedBothWriteFlags(t *testing.T) {
	t.Setenv("ADMIN_REFERRAL_WRITES_ENABLED", "")
	t.Setenv("ADMIN_REMOTE_WRITES_ENABLED", "")
	flags := loadDomainFlags()
	if flags.ReferralWrites || flags.WriteFamilyRequested("referral.admin.command.execute") {
		t.Fatal("Referral writes enabled by default")
	}
	t.Setenv("ADMIN_REFERRAL_WRITES_ENABLED", "true")
	if loadDomainFlags().WriteFamilyRequested("referral.admin.command.execute") {
		t.Fatal("Referral flag bypassed the global write gate")
	}
	t.Setenv("ADMIN_REMOTE_WRITES_ENABLED", "true")
	if !loadDomainFlags().WriteFamilyRequested("referral.admin.command.execute") {
		t.Fatal("explicit Referral and global write flags did not request the family")
	}
}
