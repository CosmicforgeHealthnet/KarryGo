package admincommand

// ReferralSpec contains only transitions already owned by Referral's native
// programme state machines. Paid reward reversal creates a Referral ledger
// take-back; Wallet remains the authority that posts the financial effect.
func ReferralSpec() Spec {
	return Spec{Owner: "referral-service", Domain: "referral", Segment: "referral", Contract: "referral.admin-command.v1", Capability: "referral.admin.command.execute", ReadCapability: "referral.admin.command.read", ApproveCapability: "referral.admin.command.approve", Actions: map[string]ActionSpec{
		"FREEZE_PERSON":       {SubjectKinds: kinds("persons"), VersionOf: VersionSubject},
		"CLEAR_PERSON":        {SubjectKinds: kinds("persons"), RequiresApproval: true, VersionOf: VersionSubject},
		"BLOCK_PERSON":        {SubjectKinds: kinds("persons"), RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"reverse_paid": enum(false, "false", "true")}},
		"FREEZE_REFERRAL":     {SubjectKinds: kinds("referrals"), VersionOf: VersionSubject},
		"CLEAR_REFERRAL":      {SubjectKinds: kinds("referrals"), RequiresApproval: true, VersionOf: VersionSubject},
		"DISQUALIFY_REFERRAL": {SubjectKinds: kinds("referrals"), RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"reverse_paid": enum(false, "false", "true")}},
		"RESOLVE_FRAUD_FLAG":  {SubjectKinds: kinds("fraud-flags"), RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"outcome": enum(true, "CLEARED", "UPHELD")}},
		"REVERSE_REWARD":      {SubjectKinds: kinds("rewards"), RequiresApproval: true, VersionOf: VersionSubject},
	}}
}
