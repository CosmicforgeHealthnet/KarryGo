package admincommand

// VerificationSpec is the formal review dictionary (Section 18). Each action
// is one legal transition of Verification's own case state machine; there is
// no generic status setter. High-impact outcomes require a second reviewer
// bound to the exact intent. Automation never produces VERIFIED.
func VerificationSpec() Spec {
	cases := kinds("vendor-cases")
	// Company (organization) submissions from fleet-partner-service: an
	// operator reviews them and decides (Fleet ADR-006).
	orgs := kinds("organization-submissions")
	orgDocument := enum(false, "CERTIFICATE_OF_INCORPORATION", "TAX_CLEARANCE", "INSURANCE_POLICY", "OPERATING_LICENSE")
	reason := enum(true, "DOCUMENT_MISMATCH", "DOCUMENT_EXPIRED", "DOCUMENT_REVOKED", "IDENTITY_MISMATCH", "ADDRESS_CONFLICT", "INSUFFICIENT_EVIDENCE", "COMPLIANCE_LAPSE", "FRAUD_SIGNAL", "OTHER_REVIEWED")
	return Spec{Owner: "verification-compliance-service", Domain: "verification", Contract: "verification.admin-command.v1", Capability: "verification.admin.command.execute", ReadCapability: "verification.admin.command.read", ApproveCapability: "verification.admin.command.approve", Actions: map[string]ActionSpec{
		"START_REVIEW":        {SubjectKinds: cases, VersionOf: VersionSubject},
		"REQUEST_REMEDIATION": {SubjectKinds: cases, VersionOf: VersionSubject, Params: map[string]ParamSpec{"remediation": enum(true, "RESUBMIT_DOCUMENT", "PROVIDE_ADDITIONAL_DOCUMENT", "CORRECT_DETAILS")}},
		"APPROVE_CASE":        {SubjectKinds: cases, RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"expires_on": dateReq}},
		"REJECT_CASE":         {SubjectKinds: cases, RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"reason": reason}},
		"SUSPEND_CASE":        {SubjectKinds: cases, RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"reason": reason}},
		"REOPEN_CASE":         {SubjectKinds: cases, RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"appeal_ref": refReq}},

		"START_ORGANIZATION_REVIEW":    {SubjectKinds: orgs, VersionOf: VersionSubject},
		"REQUEST_ORGANIZATION_CHANGES": {SubjectKinds: orgs, VersionOf: VersionSubject, Params: map[string]ParamSpec{"issue": reason, "remediation": enum(true, "RESUBMIT_DOCUMENT", "PROVIDE_ADDITIONAL_DOCUMENT", "CORRECT_DETAILS"), "document": orgDocument}},
		"APPROVE_ORGANIZATION":         {SubjectKinds: orgs, RequiresApproval: true, VersionOf: VersionSubject},
		"REJECT_ORGANIZATION":          {SubjectKinds: orgs, RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"reason": reason}},
		"SUSPEND_ORGANIZATION":         {SubjectKinds: orgs, RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"reason": reason}},
	}}
}
