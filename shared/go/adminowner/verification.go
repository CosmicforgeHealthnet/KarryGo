package adminowner

// VerificationProfile is the minimized verification-compliance-service owner
// contract (Section 18). Document references, identity numbers, bank data and
// reviewer free text are deliberately absent; automation output is exposed
// only as bounded finding codes. Verification remains the sole authority for
// formal status; HQ and Vendor only observe it.
func VerificationProfile() Profile {
	contract := "verification.admin-resources.v1"
	def := func(kind, capability string, dims, counters, refs []string) Definition {
		return Definition{Kind: kind, Capability: capability, SourceContract: contract, Dimensions: dims, Counters: counters, References: refs}
	}
	return Profile{Owner: "verification-compliance-service", Domain: "verification", Prefix: "VERIFICATION_COMPLIANCE", Path: "/internal/verification/v1/admin", Definitions: map[string]Definition{
		"vendor-cases":             def("vendor-cases", "verification.vendor_cases.read", []string{"dimension", "subject_type", "automation_recommendation", "expires_at", "reason_code"}, []string{"document_count", "finding_count", "appeal_count"}, []string{"business", "subject"}),
		"vendor-case-findings":     def("vendor-case-findings", "verification.findings.read", []string{"check_code", "outcome", "severity"}, []string{"case_version"}, []string{"case"}),
		"asset-capability-cases":   def("asset-capability-cases", "verification.asset_cases.read", []string{"authority", "capability_code", "product_code", "expires_at"}, []string{"policy_version"}, []string{"asset", "provider"}),
		"organization-submissions": def("organization-submissions", "verification.organization_submissions.read", []string{"restriction_status"}, nil, []string{"organization"}),
	}}
}
