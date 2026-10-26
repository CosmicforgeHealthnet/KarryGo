package adminowner

// SupportProfile is the minimized support-dispute-service owner contract
// (Section 16). Chat bodies, identity snapshots (names/phones), incident
// coordinates and free-text resolution notes are deliberately absent. A case
// label is a bounded, credential-redacted subject preview only. Market/zone
// are not recorded natively on complaints; they come only from an
// independently signed owner context, never inferred.
func SupportProfile() Profile {
	contract := "support.admin-resources.v1"
	def := func(kind, capability string, dims, counters, refs []string, label bool) Definition {
		return Definition{Kind: kind, Capability: capability, SourceContract: contract, Dimensions: dims, Counters: counters, References: refs, Label: label}
	}
	caseDims := []string{"service_type", "category", "priority", "complainant_type", "context_type", "locale", "sla_state", "consent_classification"}
	caseCounters := []string{"unread_count", "message_count", "evidence_count"}
	caseRefs := []string{"organization", "mission", "order", "assignee", "dispute"}
	return Profile{Owner: "support-dispute-service", Domain: "support", Prefix: "SUPPORT_DISPUTE", Path: "/internal/support/v1/admin", Definitions: map[string]Definition{
		"cases":         def("cases", "support.cases.read", caseDims, caseCounters, caseRefs, true),
		"sos-cases":     def("sos-cases", "support.sos.read", []string{"service_type", "complainant_type", "escalation_level", "sla_state"}, []string{"unread_count", "message_count"}, []string{"organization", "assignee"}, false),
		"disputes":      def("disputes", "support.disputes.read", []string{"service_type", "respondent_type", "remedy_state"}, nil, []string{"case", "remedy_operation"}, false),
		"case-messages": def("case-messages", "support.messages.read", []string{"sender_type", "read"}, []string{"sequence"}, []string{"case"}, true),
		"case-evidence": def("case-evidence", "support.evidence.read", []string{"uploader_type"}, nil, []string{"case", "media_object"}, false),
	}}
}
