package admincommand

import "cosmicforge/logistics/shared/go/adminowner"

// Parameter kinds. Each one is a bounded, closed shape; there is no generic
// "value" or JSON parameter that could carry an arbitrary state.
const (
	ParamEnum = "ENUM"
	ParamUUID = "UUID"
	ParamRef  = "REF"
	ParamDate = "DATE"
	ParamUint = "UINT"
	ParamText = "TEXT"
)

// VersionOf names the entity whose version ExpectedVersion fences.
const (
	VersionSubject = "SUBJECT" // the subject's administrative standing
	VersionCase    = "CASE"    // the case named by case_ref (0 = create)
)

type ParamSpec struct {
	Kind     string
	Enum     []string
	Required bool
	Max      int
}
type ActionSpec struct {
	SubjectKinds     []string
	Params           map[string]ParamSpec
	RequiresCase     bool
	RequiresApproval bool
	VersionOf        string
}

// Spec is one owner's fixed command dictionary. Subject kinds are always
// existing enrolled resource kinds, so command authority is resolved from the
// same independently signed owner context as reads; the owner's own binding
// contract for that kind must match before the command contract applies.
type Spec struct {
	Owner, Domain, Contract    string
	Capability, ReadCapability string
	ApproveCapability          string
	Actions                    map[string]ActionSpec
}

func (s Spec) Path() string { return "/internal/" + s.Domain + "/v1/admin/commands" }

// SubjectContract is the owner's existing resource contract for a kind.
func (s Spec) SubjectContract(kind string) (string, bool) {
	profile, ok := adminowner.ForOwner(s.Owner)
	if !ok {
		return "", false
	}
	d, ok := profile.Definitions[kind]
	return d.SourceContract, ok
}

// SubjectKinds is every kind the dictionary can address.
func (s Spec) SubjectKinds() []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range s.Actions {
		for _, k := range a.SubjectKinds {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

func enum(required bool, values ...string) ParamSpec {
	return ParamSpec{Kind: ParamEnum, Enum: values, Required: required}
}
func kinds(values ...string) []string { return values }

var dateReq = ParamSpec{Kind: ParamDate, Required: true}
var dateOpt = ParamSpec{Kind: ParamDate}
var refOpt = ParamSpec{Kind: ParamRef}
var refReq = ParamSpec{Kind: ParamRef, Required: true}
var noteOpt = ParamSpec{Kind: ParamText, Max: 280}

func Specs() []Spec {
	severity := enum(true, "LOW", "MEDIUM", "HIGH", "CRITICAL")
	return []Spec{
		{
			Owner: "customer-service", Domain: "customer", Contract: "customer.admin-command.v1",
			Capability: "customer.admin.command.execute", ReadCapability: "customer.admin.command.read", ApproveCapability: "customer.admin.command.approve",
			Actions: map[string]ActionSpec{
				"RESTRICT_ACCOUNT":        {SubjectKinds: kinds("customers"), Params: map[string]ParamSpec{"restriction": enum(true, "LOGIN_BLOCKED", "BOOKING_HOLD")}, RequiresApproval: true, VersionOf: VersionSubject},
				"LIFT_RESTRICTION":        {SubjectKinds: kinds("customers"), RequiresApproval: true, VersionOf: VersionSubject},
				"REQUIRE_REMEDIATION":     {SubjectKinds: kinds("customers"), Params: map[string]ParamSpec{"remediation": enum(true, "REVERIFY_CONTACT", "RESET_SESSIONS", "PROFILE_CORRECTION")}, VersionOf: VersionSubject},
				"COMPLETE_REMEDIATION":    {SubjectKinds: kinds("customers"), Params: map[string]ParamSpec{"evidence_ref": refOpt}, VersionOf: VersionSubject},
				"OPEN_PRIVACY_REQUEST":    {SubjectKinds: kinds("customers"), RequiresCase: true, Params: map[string]ParamSpec{"request_type": enum(true, "ACCESS_EXPORT", "ERASURE", "RECTIFICATION", "CONSENT_WITHDRAWAL", "RESTRICT_PROCESSING"), "channel": enum(true, "IN_APP", "EMAIL", "SUPPORT", "REGULATOR"), "received_on": dateReq}, VersionOf: VersionCase},
				"ACCEPT_PRIVACY_REQUEST":  {SubjectKinds: kinds("customers"), RequiresCase: true, Params: map[string]ParamSpec{"due_on": dateReq}, VersionOf: VersionCase},
				"FULFIL_PRIVACY_REQUEST":  {SubjectKinds: kinds("customers"), RequiresCase: true, Params: map[string]ParamSpec{"evidence_ref": refReq}, RequiresApproval: true, VersionOf: VersionCase},
				"REJECT_PRIVACY_REQUEST":  {SubjectKinds: kinds("customers"), RequiresCase: true, Params: map[string]ParamSpec{"rejection": enum(true, "IDENTITY_UNVERIFIED", "LEGAL_HOLD", "DUPLICATE", "OUT_OF_SCOPE")}, RequiresApproval: true, VersionOf: VersionCase},
				"RECORD_SUPPORT_DECISION": {SubjectKinds: kinds("customers"), RequiresCase: true, Params: map[string]ParamSpec{"decision": enum(true, "ACCOUNT_STANDING_CONFIRMED", "GOODWILL_ACKNOWLEDGED", "ESCALATED_TO_RESTRICTION_REVIEW", "NO_ACCOUNT_ACTION"), "note": noteOpt}, VersionOf: VersionCase},
			},
		},
		{
			Owner: "driver-taxi-service", Domain: "taxi", Contract: "taxi.admin-command.v1",
			Capability: "taxi.admin.command.execute", ReadCapability: "taxi.admin.command.read", ApproveCapability: "taxi.admin.command.approve",
			Actions: map[string]ActionSpec{
				"OPEN_INCIDENT":             {SubjectKinds: kinds("providers", "vehicles", "independent-rides"), RequiresCase: true, Params: map[string]ParamSpec{"category": enum(true, "SAFETY", "CONDUCT", "VEHICLE_CONDITION", "DOCUMENT", "FRAUD_SIGNAL", "SERVICE_QUALITY"), "severity": severity, "occurred_on": dateReq}, VersionOf: VersionCase},
				"START_REMEDIATION":         {SubjectKinds: kinds("providers", "vehicles", "independent-rides"), RequiresCase: true, Params: map[string]ParamSpec{"remediation": enum(true, "RETRAINING", "DOCUMENT_RESUBMISSION", "VEHICLE_REPAIR", "WARNING_ISSUED"), "due_on": dateReq}, VersionOf: VersionCase},
				"RESOLVE_INCIDENT":          {SubjectKinds: kinds("providers", "vehicles", "independent-rides"), RequiresCase: true, Params: map[string]ParamSpec{"resolution": enum(true, "REMEDIATED", "NO_FINDING", "ESCALATED_TO_RESTRICTION")}, VersionOf: VersionCase},
				"CLOSE_INCIDENT":            {SubjectKinds: kinds("providers", "vehicles", "independent-rides"), RequiresCase: true, VersionOf: VersionCase},
				"RECORD_DOCUMENT_DECISION":  {SubjectKinds: kinds("providers", "vehicles"), Params: map[string]ParamSpec{"document": enum(true, "DRIVER_LICENSE", "VEHICLE_REGISTRATION", "INSURANCE", "ROADWORTHINESS"), "decision": enum(true, "VERIFIED", "REJECTED", "EXPIRED"), "expires_on": dateOpt, "evidence_ref": refOpt}, VersionOf: VersionSubject},
				"REQUEST_REVERIFICATION":    {SubjectKinds: kinds("providers", "vehicles"), Params: map[string]ParamSpec{"document": enum(true, "DRIVER_LICENSE", "VEHICLE_REGISTRATION", "INSURANCE", "ROADWORTHINESS")}, VersionOf: VersionSubject},
				"RESTRICT_PROVIDER":         {SubjectKinds: kinds("providers"), Params: map[string]ParamSpec{"restriction": enum(true, "SUSPEND_ONLINE", "NO_NEW_ASSIGNMENTS")}, RequiresApproval: true, VersionOf: VersionSubject},
				"LIFT_PROVIDER_RESTRICTION": {SubjectKinds: kinds("providers"), RequiresApproval: true, VersionOf: VersionSubject},
				"RESTRICT_VEHICLE":          {SubjectKinds: kinds("vehicles"), Params: map[string]ParamSpec{"restriction": enum(true, "OUT_OF_SERVICE")}, RequiresApproval: true, VersionOf: VersionSubject},
				"LIFT_VEHICLE_RESTRICTION":  {SubjectKinds: kinds("vehicles"), RequiresApproval: true, VersionOf: VersionSubject},
			},
		},
		{
			Owner: "driver-dispatch-delivery-service", Domain: "dispatch", Contract: "dispatch.admin-command.v1",
			Capability: "dispatch.admin.command.execute", ReadCapability: "dispatch.admin.command.read", ApproveCapability: "dispatch.admin.command.approve",
			Actions: map[string]ActionSpec{
				"RETRY_INCIDENT_RECOVERY":    {SubjectKinds: kinds("incidents"), VersionOf: VersionSubject},
				"ESCALATE_INCIDENT_RECOVERY": {SubjectKinds: kinds("incidents"), Params: map[string]ParamSpec{"escalation": enum(true, "OPERATIONS_REVIEW", "SAFETY_REVIEW")}, VersionOf: VersionSubject},
				"REQUEST_REASSIGNMENT":       {SubjectKinds: kinds("missions"), Params: map[string]ParamSpec{"reason": enum(true, "PROVIDER_UNRESPONSIVE", "PROVIDER_UNSAFE", "VEHICLE_FAILURE", "OPERATIONS_DIRECTED")}, RequiresApproval: true, VersionOf: VersionSubject},
				"RETRY_RETURN_ASSIGNMENT":    {SubjectKinds: kinds("returns"), VersionOf: VersionSubject},
				"REQUEST_DELIVERY_REPLAN":    {SubjectKinds: kinds("delivery-recoveries"), Params: map[string]ParamSpec{"replan": enum(true, "RESCHEDULE_ATTEMPT", "RETURN_TO_SENDER")}, RequiresApproval: true, VersionOf: VersionSubject},
			},
		},
		{
			Owner: "driver-hauling-service", Domain: "hauling", Contract: "hauling.admin-command.v1",
			Capability: "hauling.admin.command.execute", ReadCapability: "hauling.admin.command.read", ApproveCapability: "hauling.admin.command.approve",
			Actions: map[string]ActionSpec{
				"RECORD_INSPECTION":            {SubjectKinds: kinds("trucks"), RequiresCase: true, Params: map[string]ParamSpec{"inspection_kind": enum(true, "ROADWORTHINESS", "CARGO_BAY", "REFRIGERATION", "LIFTING_EQUIPMENT", "SAFETY_EQUIPMENT"), "performed_on": dateReq, "media_object_id": ParamSpec{Kind: ParamUUID}}, VersionOf: VersionCase},
				"ATTACH_INSPECTION_EVIDENCE":   {SubjectKinds: kinds("trucks"), RequiresCase: true, Params: map[string]ParamSpec{"media_object_id": ParamSpec{Kind: ParamUUID, Required: true}}, VersionOf: VersionCase},
				"CONCLUDE_INSPECTION":          {SubjectKinds: kinds("trucks"), RequiresCase: true, Params: map[string]ParamSpec{"conclusion": enum(true, "PASS", "FAIL", "CONDITIONAL"), "valid_until": dateOpt}, RequiresApproval: true, VersionOf: VersionCase},
				"RECORD_CARGO_SUITABILITY":     {SubjectKinds: kinds("booking-cargo-inputs"), Params: map[string]ParamSpec{"decision": enum(true, "SUITABLE", "UNSUITABLE", "SUITABLE_WITH_REMEDIATION"), "truck_id": ParamSpec{Kind: ParamUUID, Required: true}}, VersionOf: VersionSubject},
				"COMPLETE_CARGO_REMEDIATION":   {SubjectKinds: kinds("booking-cargo-inputs"), Params: map[string]ParamSpec{"evidence_ref": refReq}, VersionOf: VersionSubject},
				"RESTRICT_TRUCK":               {SubjectKinds: kinds("trucks"), Params: map[string]ParamSpec{"reason": enum(true, "INSPECTION_FAILED", "SAFETY_CONCERN", "DOCUMENT_LAPSE")}, RequiresApproval: true, VersionOf: VersionSubject},
				"LIFT_TRUCK_RESTRICTION":       {SubjectKinds: kinds("trucks"), RequiresApproval: true, VersionOf: VersionSubject},
				"RESTRICT_PROVIDER":            {SubjectKinds: kinds("providers"), Params: map[string]ParamSpec{"reason": enum(true, "SAFETY_CONCERN", "DOCUMENT_LAPSE", "CONDUCT")}, RequiresApproval: true, VersionOf: VersionSubject},
				"LIFT_PROVIDER_RESTRICTION":    {SubjectKinds: kinds("providers"), RequiresApproval: true, VersionOf: VersionSubject},
				"REQUEST_RECOVERY_REPLACEMENT": {SubjectKinds: kinds("recoveries"), Params: map[string]ParamSpec{"replacement_provider_id": ParamSpec{Kind: ParamUUID, Required: true}, "replacement_truck_id": ParamSpec{Kind: ParamUUID, Required: true}}, RequiresApproval: true, VersionOf: VersionSubject},
			},
		},
		{
			Owner: "fleet-partner-service", Domain: "fleet", Contract: "fleet.admin-command.v1",
			Capability: "fleet.admin.command.execute", ReadCapability: "fleet.admin.command.read", ApproveCapability: "fleet.admin.command.approve",
			Actions: map[string]ActionSpec{
				"OPEN_INVESTIGATION":  {SubjectKinds: kinds("shipments", "private-assignments", "execution-links"), RequiresCase: true, Params: map[string]ParamSpec{"category": enum(true, "CARGO_DAMAGE", "CARGO_LOSS", "SAFETY", "CONDUCT", "SERVICE_FAILURE"), "severity": severity, "occurred_on": dateReq}, VersionOf: VersionCase},
				"RECORD_FINDING":      {SubjectKinds: kinds("shipments", "private-assignments", "execution-links"), RequiresCase: true, Params: map[string]ParamSpec{"finding": enum(true, "SUBSTANTIATED", "UNSUBSTANTIATED", "INCONCLUSIVE"), "note": noteOpt}, VersionOf: VersionCase},
				"CLOSE_INVESTIGATION": {SubjectKinds: kinds("shipments", "private-assignments", "execution-links"), RequiresCase: true, Params: map[string]ParamSpec{"closure": enum(true, "RESOLVED", "REFERRED_TO_OWNER", "NO_FURTHER_ACTION")}, RequiresApproval: true, VersionOf: VersionCase},
			},
		},
	}
}

func ForOwner(owner string) (Spec, bool) {
	for _, s := range Specs() {
		if s.Owner == owner {
			return s, true
		}
	}
	return Spec{}, false
}
func ForDomain(domain string) (Spec, bool) {
	for _, s := range Specs() {
		if s.Domain == domain {
			return s, true
		}
	}
	return Spec{}, false
}
