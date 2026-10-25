package adminowner

// ReferralProfile exposes only privacy-minimized Referral owner records. Raw
// identity fingerprints, codes, free-form fraud evidence and wallet payloads
// are deliberately absent. Geography and tenant authority are supplied only
// by a separately signed owner context because Referral's person records do
// not canonically own those relationships.
func ReferralProfile() Profile {
	return Profile{Owner: "referral-service", Domain: "referral", Prefix: "REFERRAL", Path: "/internal/referral/v1/admin-owner", Definitions: map[string]Definition{
		"persons":                   {Kind: "persons", Capability: "referral.persons.read", SourceContract: "referral.admin-resources.v1", Dimensions: []string{"home_actor_type"}, References: []string{"home_actor"}},
		"accounts":                  {Kind: "accounts", Capability: "referral.accounts.read", SourceContract: "referral.admin-resources.v1", Dimensions: []string{"actor_type", "is_new_account", "attribution_outcome"}, References: []string{"person"}},
		"referrals":                 {Kind: "referrals", Capability: "referral.referrals.read", SourceContract: "referral.admin-resources.v1", Dimensions: []string{"code_source"}, References: []string{"referrer_person", "referred_person"}},
		"tracks":                    {Kind: "tracks", Capability: "referral.tracks.read", SourceContract: "referral.admin-resources.v1", Dimensions: []string{"model", "window_starts_at", "window_ends_at"}, Counters: []string{"jobs_required", "jobs_counted"}, References: []string{"referral"}},
		"rewards":                   {Kind: "rewards", Capability: "referral.rewards.read", SourceContract: "referral.admin-resources.v1", Dimensions: []string{"reward_type", "balance", "currency", "wallet_state"}, Counters: []string{"amount_minor"}, References: []string{"person", "referral", "beneficiary_actor", "reverses_reward", "wallet"}},
		"fraud-flags":               {Kind: "fraud-flags", Capability: "referral.fraud_flags.read", SourceContract: "referral.admin-resources.v1", Dimensions: []string{"flag_type", "severity"}, References: []string{"person", "referral"}},
		"milestones":                {Kind: "milestones", Capability: "referral.milestones.read", SourceContract: "referral.admin-resources.v1", Dimensions: []string{"reward_type"}, Counters: []string{"threshold"}, References: []string{"person", "reward"}},
		"reconciliation-mismatches": {Kind: "reconciliation-mismatches", Capability: "referral.reconciliation.read", SourceContract: "referral.admin-resources.v1", Dimensions: []string{"reason"}, Counters: []string{"referral_amount_minor", "wallet_amount_minor"}, References: []string{"person"}},
		"admin-actions":             {Kind: "admin-actions", Capability: "referral.admin_actions.read", SourceContract: "referral.admin-resources.v1", Dimensions: []string{"action", "subject_kind"}, References: []string{"subject", "operator"}},
	}}
}
