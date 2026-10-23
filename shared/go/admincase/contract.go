// Package admincase is the finite source-owned case authorization contract.
// It discloses neither evidence bytes nor private URLs.
package admincase

import (
	"cosmicforge/logistics/shared/go/adminpolicy"
	"time"
)

const Caller = "media-file-service-case-authorizer"
const Contract = "karryam.source-case-authorization.v1"
const ContextPrefix = "media-case:"

type Spec struct {
	Owner, Domain, Kind, Permission, SourceContract, BindingType string
	Subjects                                                     map[string][]string
	States, Purposes                                             []string
}

func Specs() []Spec {
	recoverySubjects := map[string][]string{"CASE": {"$CASE"}, "PROVIDER": {"original_provider", "replacement_provider"}, "BOOKING": {"booking"}, "MISSION": {"mission", "goods_mission"}, "TRUCK": {"original_truck", "replacement_truck"}}
	taxiSubjects := map[string][]string{"CASE": {"$CASE"}, "PROVIDER": {"original_provider", "replacement_provider"}, "BOOKING": {"booking"}, "MISSION": {"goods_mission"}, "VEHICLE": {"original_vehicle", "replacement_vehicle"}}
	recoveryStates := []string{"DETECTED", "EVALUATING", "REPLACEMENT_SEARCH", "TRANSFER_REQUIRED", "IN_PROGRESS", "OPS_REQUIRED"}
	purposes := []string{"CARGO_CONDITION", "DAMAGE_EVIDENCE", "RECOVERY_TRANSFER", "SAFETY_INCIDENT", "INCIDENT_ATTACHMENT"}
	return []Spec{
		{"vendor-service", "vendor", "cases", "vendor.case.evidence.read", "vendor.admin-case-evidence.v1", "VENDOR_CASE", map[string][]string{"CASE": {"$CASE"}, "BUSINESS": {"business"}, "BRANCH": {"branch"}}, []string{"OPEN", "ESCALATED", "REVIEWED", "APPEALED", "APPEAL_DECIDED"}, []string{"INCIDENT_ATTACHMENT", "DAMAGE_EVIDENCE", "CARGO_CONDITION", "SAFETY_INCIDENT", "DISPUTE_ATTACHMENT"}},

		{"customer-service", "customer", "review-work-items", "customer.case.evidence.read", "customer.admin-case-evidence.v1", "VERIFICATION_CASE", map[string][]string{"CASE": {"$CASE"}, "CUSTOMER": {"customer"}}, []string{"REQUESTED", "ACKNOWLEDGED"}, []string{"COMPLIANCE_DOCUMENT", "PROOF_DOCUMENT", "DISPUTE_ATTACHMENT"}},
		{"driver-taxi-service", "taxi", "recoveries", "taxi.case.evidence.read", "taxi.admin-case-evidence.v1", "RECOVERY_CASE", taxiSubjects, recoveryStates, purposes},
		{"driver-taxi-service", "taxi", "incidents", "taxi.case.evidence.read", "taxi.admin-case-evidence.v1", "RECOVERY_CASE", map[string][]string{"CASE": {"$CASE"}, "PROVIDER": {"provider"}, "VEHICLE": {"vehicle"}, "BOOKING": {"booking"}}, []string{"OPEN", "REMEDIATION_IN_PROGRESS"}, purposes},
		{"driver-dispatch-delivery-service", "dispatch", "incidents", "dispatch.case.evidence.read", "dispatch.admin-case-evidence.v1", "RECOVERY_CASE", map[string][]string{"CASE": {"$CASE"}, "TRIP": {"trip"}, "PROVIDER": {"original_provider", "replacement_provider"}}, []string{"REPORTED", "REASSIGNING", "TRANSFER_PENDING", "EMERGENCY_RECOVERY"}, purposes},
		{"driver-dispatch-delivery-service", "dispatch", "delivery-recoveries", "dispatch.case.evidence.read", "dispatch.admin-case-evidence.v1", "RECOVERY_CASE", map[string][]string{"CASE": {"$CASE"}, "TRIP": {"trip"}}, []string{"REQUESTED", "SCHEDULED", "READY", "AWAITING_CUSTOMER_APPROVAL", "AWAITING_FINANCIAL_AUTHORIZATION", "RECONCILIATION_REQUIRED"}, purposes},
		{"driver-dispatch-delivery-service", "dispatch", "returns", "dispatch.case.evidence.read", "dispatch.admin-case-evidence.v1", "RECOVERY_CASE", map[string][]string{"CASE": {"$CASE"}, "TRIP": {"trip"}}, []string{"REQUIRED", "ROUTED", "RETURNING", "HANDOVER_REQUIRED", "SUPPORT_REQUIRED"}, purposes},
		{"driver-hauling-service", "hauling", "recoveries", "hauling.case.evidence.read", "hauling.admin-case-evidence.v1", "RECOVERY_CASE", recoverySubjects, recoveryStates, purposes},
		// Hauling-owned inspection cases. Evidence is reachable only through an
		// inspection the owner recorded and still binds to the truck's tenant.
		{"driver-hauling-service", "hauling", "inspections", "hauling.case.evidence.read", "hauling.admin-case-evidence.v1", "INSPECTION_CASE", map[string][]string{"CASE": {"$CASE"}, "TRUCK": {"truck"}}, []string{"RECORDED", "CONCLUDED"}, []string{"INSPECTION_EVIDENCE", "CARGO_CONDITION", "DAMAGE_EVIDENCE"}},
		// Fleet standalone investigations. Domain is the native Fleet route
		// segment so Path() matches the owner's private admin group.
		{"fleet-partner-service", "fleet-partner", "investigations", "fleet.case.evidence.read", "fleet.admin-case-evidence.v1", "INVESTIGATION_CASE", map[string][]string{"CASE": {"$CASE"}, "SHIPMENT": {"shipment"}, "ASSIGNMENT": {"private_assignment"}, "EXECUTION_LINK": {"execution_link"}}, []string{"OPEN", "FINDING_RECORDED"}, []string{"CARGO_CONDITION", "DAMAGE_EVIDENCE", "SAFETY_INCIDENT", "INCIDENT_ATTACHMENT"}},
	}
}
func Find(owner, kind string) (Spec, bool) {
	for _, s := range Specs() {
		if s.Owner == owner && s.Kind == kind {
			return s, true
		}
	}
	return Spec{}, false
}
func Contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (s Spec) Path() string { return "/internal/" + s.Domain + "/v1/admin/case-authority" }

type Proof struct {
	ContractVersion   string            `json:"contract_version"`
	Owner             string            `json:"owner"`
	CaseKind          string            `json:"case_kind"`
	CaseReference     string            `json:"case_reference"`
	SubjectType       string            `json:"subject_type"`
	SubjectReference  string            `json:"subject_reference"`
	Scope             adminpolicy.Scope `json:"scope"`
	SourceContract    string            `json:"source_contract"`
	NativeFingerprint string            `json:"native_fingerprint"`
	ExpiresAt         time.Time         `json:"expires_at"`
	PolicyID          string            `json:"policy_id"`
	PolicyVersion     int64             `json:"policy_version"`
	PolicyChecksum    string            `json:"policy_checksum"`
}
