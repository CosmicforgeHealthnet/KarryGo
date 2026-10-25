package adminresources

import (
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/adminowner"
)

func TestReferralOwnerProfileUsesFixedPrivacyMinimizedDictionary(t *testing.T) {
	profile := adminowner.ReferralProfile()
	want := map[string]string{
		"persons": "referral.persons.read", "accounts": "referral.accounts.read",
		"referrals": "referral.referrals.read", "tracks": "referral.tracks.read",
		"rewards": "referral.rewards.read", "fraud-flags": "referral.fraud_flags.read",
		"milestones": "referral.milestones.read", "reconciliation-mismatches": "referral.reconciliation.read",
		"admin-actions": "referral.admin_actions.read",
	}
	if profile.Owner != Owner || profile.Path != "/internal/referral/v1/admin-owner" || len(profile.Definitions) != len(want) {
		t.Fatalf("unexpected fixed Referral profile: owner=%q path=%q definitions=%d", profile.Owner, profile.Path, len(profile.Definitions))
	}
	for kind, permission := range want {
		definition, ok := profile.Definitions[kind]
		if !ok || definition.Kind != kind || definition.Capability != permission || definition.SourceContract != "referral.admin-resources.v1" {
			t.Fatalf("invalid definition for %q: %+v", kind, definition)
		}
		for _, field := range append(append([]string{}, definition.Dimensions...), definition.References...) {
			lower := strings.ToLower(field)
			for _, forbidden := range []string{"phone", "email", "fingerprint", "referral_code", "evidence", "note", "payload", "bank"} {
				if strings.Contains(lower, forbidden) {
					t.Fatalf("privacy-sensitive field %q exposed by %q", field, kind)
				}
			}
		}
	}
	if Source() == nil {
		t.Fatal("fixed Referral SQL source is unavailable")
	}
}
