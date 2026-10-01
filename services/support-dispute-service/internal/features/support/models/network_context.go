package supportmodels

// Section 15: structured operational context on a complaint, what Support saw
// when it looked, and what it asked an owner to do.
//
// The line that runs through all three is ownership. Support keeps REFERENCES
// to jobs other services own, SNAPSHOTS of what it was shown, and REQUESTS it
// made. It never keeps a trip, mission or booking state of its own -- a second
// copy of operational truth is a copy that will eventually disagree with the
// first, and the one support agents read would be the stale one.

import (
	"time"

	"cosmicforge/logistics/shared/go/supportcase"
)

// Network complaint categories (Section 15). Added to ValidCategories below,
// following the existing in-code allow-list style so no migration is needed.
const (
	CategoryWrongVehicle           = "wrong_vehicle"
	CategoryCapabilityMismatch     = "capability_mismatch"
	CategoryVendorDelay            = "vendor_delay"
	CategoryProviderNoShow         = "provider_no_show"
	CategoryRecoveryFailure        = "recovery_failure"
	CategoryUnsafeVehicle          = "unsafe_vehicle"
	CategoryCargoFitFailure        = "cargo_fit_failure"
	CategoryTemperatureIssue       = "temperature_issue"
	CategoryCustodyHandoverFailure = "custody_handover_failure"
)

func init() {
	for _, category := range []string{
		CategoryWrongVehicle, CategoryCapabilityMismatch, CategoryVendorDelay,
		CategoryProviderNoShow, CategoryRecoveryFailure, CategoryUnsafeVehicle,
		CategoryCargoFitFailure, CategoryTemperatureIssue, CategoryCustodyHandoverFailure,
	} {
		ValidCategories[category] = true
	}
}

// ContextRefs are the references a complaint may carry. All optional.
//
// None of them is a state. A complaint about a mission knows WHICH mission;
// what that mission is doing now is the owner's to say, and is read fresh.
type ContextRefs struct {
	ContextType        string `json:"context_type,omitempty"`
	OrderRef           string `json:"order_ref,omitempty"`
	FulfilmentRef      string `json:"fulfilment_ref,omitempty"`
	MissionRef         string `json:"mission_ref,omitempty"`
	ProviderRef        string `json:"provider_ref,omitempty"`
	AssetRef           string `json:"asset_ref,omitempty"`
	GuaranteeBreachRef string `json:"guarantee_breach_ref,omitempty"`
	RecoveryCaseRef    string `json:"recovery_case_ref,omitempty"`
}

// PrimaryRef is the reference an owner is asked about for this context type.
func (r ContextRefs) PrimaryRef() string {
	switch r.ContextType {
	case supportcase.ContextVendorOrder:
		return r.OrderRef
	case supportcase.ContextFleetPrivateOrder:
		if r.OrderRef != "" {
			return r.OrderRef
		}
		return r.MissionRef
	default:
		return r.MissionRef
	}
}

// ContextSnapshot is what Support was shown, and when. Append-only.
type ContextSnapshot struct {
	ID            string                         `json:"id"`
	ComplaintID   string                         `json:"complaint_id"`
	SourceService string                         `json:"source_service"`
	SourceRef     string                         `json:"source_ref"`
	SourceVersion int64                          `json:"source_version"`
	Context       supportcase.OperationalContext `json:"context"`
	ObservedAt    time.Time                      `json:"observed_at"`
	CorrelationID string                         `json:"correlation_id,omitempty"`
}

// Action codes. Every code here is one an owner can actually be asked to do
// through a contract it ALREADY exposes; see the routing table in the use case
// for which owner, and why the others are not offered.
const (
	ActionRequestRecoveryEscalation = "REQUEST_RECOVERY_ESCALATION"
	ActionRequestCancellationReview = "REQUEST_CANCELLATION_REVIEW"
	ActionRequestReceiverReview     = "REQUEST_RECEIVER_CHANGE_REVIEW"
	ActionRequestRefundReview       = "REQUEST_REFUND_REVIEW"
	ActionRequestSafetyReview       = "REQUEST_PROVIDER_SAFETY_REVIEW"
	ActionRequestEvidenceRelease    = "REQUEST_EVIDENCE_RELEASE"
	ActionRequestGuaranteeReview    = "REQUEST_GUARANTEE_INVESTIGATION"
)

// Action request statuses. They describe the conversation with the owner,
// never the state of the thing being acted on.
const (
	ActionPending    = "PENDING"
	ActionDelivered  = "DELIVERED"
	ActionAccepted   = "ACCEPTED"
	ActionRefused    = "REFUSED"
	ActionUnknown    = "UNKNOWN"
	ActionDeadLetter = "DEAD_LETTER"
)

// ActionRequest is Support asking an owner to act.
type ActionRequest struct {
	ID                 string     `json:"id"`
	ComplaintID        string     `json:"complaint_id"`
	TargetService      string     `json:"target_service"`
	ActionCode         string     `json:"action_code"`
	TargetRef          string     `json:"target_ref"`
	RequestVersion     int64      `json:"request_version"`
	Status             string     `json:"status"`
	IdempotencyKey     string     `json:"-"`
	RequestFingerprint string     `json:"-"`
	ResultRef          string     `json:"result_ref,omitempty"`
	ResultCode         string     `json:"result_code,omitempty"`
	RequestedBy        string     `json:"requested_by"`
	Attempts           int        `json:"attempts"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	// Parameters are the owner-facing inputs, e.g. the trigger code for a
	// recovery escalation. Persisted, so a retried delivery sends exactly what
	// the first attempt sent; covered by the request fingerprint.
	Parameters map[string]string `json:"parameters"`
}
