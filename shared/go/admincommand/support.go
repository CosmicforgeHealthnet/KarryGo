package admincommand

// SupportSpec is Support's closed command dictionary (Section 16). The owner
// stores the canonical message/decision; the Bridge never chooses a state.
// RECORD_DISPUTE_REMEDY only links a Wallet operation that the Bridge has
// already observed CONFIRMED by original-identity readback; Support never
// moves money and never trusts an unconfirmed reference.
func SupportSpec() Spec {
	cases := kinds("cases", "sos-cases")
	return Spec{Owner: "support-dispute-service", Domain: "support", Contract: "support.admin-command.v1", Capability: "support.admin.command.execute", ReadCapability: "support.admin.command.read", ApproveCapability: "support.admin.command.approve", Actions: map[string]ActionSpec{
		"ASSIGN_CASE":           {SubjectKinds: cases, VersionOf: VersionSubject, Params: map[string]ParamSpec{"assignee_ref": refReq}},
		"REPLY_TO_CASE":         {SubjectKinds: cases, VersionOf: VersionSubject, Params: map[string]ParamSpec{"message": {Kind: ParamText, Max: 512, Required: true}, "client_message_id": {Kind: ParamUUID, Required: true}}},
		"TRANSITION_CASE":       {SubjectKinds: cases, VersionOf: VersionSubject, Params: map[string]ParamSpec{"status": enum(true, "UNDER_REVIEW", "AWAITING_EVIDENCE", "OPEN")}},
		"ESCALATE_CASE":         {SubjectKinds: kinds("cases"), VersionOf: VersionSubject, Params: map[string]ParamSpec{"escalation": enum(true, "OPERATIONS_REVIEW", "SAFETY_REVIEW", "DISPUTE_REVIEW")}},
		"CLOSE_CASE":            {SubjectKinds: cases, VersionOf: VersionSubject, Params: map[string]ParamSpec{"closure": enum(true, "RESOLVED", "NO_FURTHER_ACTION", "DUPLICATE")}},
		"ATTACH_EVIDENCE":       {SubjectKinds: cases, VersionOf: VersionSubject, Params: map[string]ParamSpec{"media_object_id": {Kind: ParamUUID, Required: true}}},
		"ESCALATE_SOS":          {SubjectKinds: kinds("sos-cases"), VersionOf: VersionSubject, Params: map[string]ParamSpec{"level": enum(true, "SAFETY_LEAD", "EMERGENCY_LIAISON")}},
		"RESOLVE_DISPUTE":       {SubjectKinds: kinds("disputes"), RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"outcome": enum(true, "FAVOUR_COMPLAINANT", "FAVOUR_RESPONDENT", "SPLIT", "DISMISSED")}},
		"RECORD_DISPUTE_REMEDY": {SubjectKinds: kinds("disputes"), RequiresApproval: true, VersionOf: VersionSubject, Params: map[string]ParamSpec{"wallet_operation_id": {Kind: ParamUUID, Required: true}, "amount_minor": {Kind: ParamUint, Required: true}}},
	}}
}
