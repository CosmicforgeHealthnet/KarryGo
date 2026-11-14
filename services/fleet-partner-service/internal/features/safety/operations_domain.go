package safety

import (
	"encoding/json"
	"time"
)

type OperationsFlags struct{ Reports, Incidents, Workers, KillSwitch bool }
type Report struct {
	ID                   string     `json:"id"`
	OrganizationID       string     `json:"organizationId"`
	BranchID             string     `json:"branchId"`
	ReporterType         string     `json:"reporterType"`
	ReporterReference    string     `json:"reporterReference"`
	Anonymous            bool       `json:"anonymous"`
	ReportType           string     `json:"reportType"`
	Severity             string     `json:"severity"`
	LocationReference    string     `json:"locationReference"`
	RouteID              string     `json:"routeId"`
	AssignmentID         string     `json:"assignmentId"`
	TripReference        string     `json:"tripReference"`
	ProviderMembershipID string     `json:"providerMembershipId"`
	AssetID              string     `json:"assetId"`
	Description          string     `json:"description"`
	ImmediateAction      string     `json:"immediateAction"`
	Status               string     `json:"status"`
	OwnerUserID          string     `json:"ownerUserId"`
	ResolutionAction     string     `json:"resolutionAction"`
	ResolutionReason     string     `json:"resolutionReason"`
	Version              int64      `json:"version"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	ClosedAt             *time.Time `json:"closedAt"`
}
type Incident struct {
	ID                 string          `json:"id"`
	OrganizationID     string          `json:"organizationId"`
	BranchID           string          `json:"branchId"`
	IncidentType       string          `json:"incidentType"`
	Severity           string          `json:"severity"`
	Status             string          `json:"status"`
	OccurredAt         time.Time       `json:"occurredAt"`
	LocationReference  string          `json:"locationReference"`
	RouteID            string          `json:"routeId"`
	AssignmentID       string          `json:"assignmentId"`
	TripReference      string          `json:"tripReference"`
	PeopleReferences   json.RawMessage `json:"peopleReferences"`
	AssetReferences    json.RawMessage `json:"assetReferences"`
	ShipmentReferences json.RawMessage `json:"shipmentReferences"`
	ExternalReference  string          `json:"externalReference"`
	Description        string          `json:"description"`
	ReportedBy         string          `json:"reportedBy"`
	OwnerUserID        string          `json:"ownerUserId"`
	ResolutionAction   string          `json:"resolutionAction"`
	ResolutionReason   string          `json:"resolutionReason"`
	Version            int64           `json:"version"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
	ClosedAt           *time.Time      `json:"closedAt"`
	// Lifecycle facts recorded by the spec 58.11 transitions.
	ImmediateActions    string     `json:"immediateActions"`
	TriagedBy           string     `json:"triagedBy"`
	TriagedAt           *time.Time `json:"triagedAt"`
	ContainmentSummary  string     `json:"containmentSummary"`
	ContainedBy         string     `json:"containedBy"`
	ContainedAt         *time.Time `json:"containedAt"`
	EscalationReason    string     `json:"escalationReason"`
	EscalationTrigger   string     `json:"escalationTrigger"`
	PreEscalationStatus string     `json:"preEscalationStatus"`
	EscalatedBy         string     `json:"escalatedBy"`
	EscalatedAt         *time.Time `json:"escalatedAt"`
	ClosureReason       string     `json:"closureReason"`
	ClosedBy            string     `json:"closedBy"`
}

// Incident statuses (spec 58.11). OPEN is "reported", INVESTIGATING is
// "under_investigation" and CORRECTIVE_ACTION_REQUIRED is "corrective_action".
const (
	IncidentOpen             = "OPEN"
	IncidentTriaged          = "TRIAGED"
	IncidentContained        = "CONTAINED"
	IncidentInvestigating    = "INVESTIGATING"
	IncidentCorrectiveAction = "CORRECTIVE_ACTION_REQUIRED"
	IncidentResolved         = "RESOLVED"
	IncidentClosed           = "CLOSED"
	IncidentEscalated        = "ESCALATED"
)

// Investigation is the organization's own investigation of one incident.
type Investigation struct {
	ID                   string          `json:"id"`
	OrganizationID       string          `json:"organizationId"`
	IncidentID           string          `json:"incidentId"`
	InvestigatorUserID   string          `json:"investigatorUserId"`
	Scope                string          `json:"scope"`
	EvidencePreservation string          `json:"evidencePreservation"`
	Status               string          `json:"status"`
	Findings             string          `json:"findings"`
	RootCause            string          `json:"rootCause"`
	ContributingFactors  json.RawMessage `json:"contributingFactors"`
	FindingsSubmittedAt  *time.Time      `json:"findingsSubmittedAt"`
	ApprovedBy           string          `json:"approvedBy"`
	ApprovedAt           *time.Time      `json:"approvedAt"`
	ClosedAt             *time.Time      `json:"closedAt"`
	OpenedBy             string          `json:"openedBy"`
	Version              int64           `json:"version"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
}
type Evidence struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	IncidentID     string          `json:"incidentId"`
	MediaFileID    string          `json:"mediaFileId"`
	HashMetadata   json.RawMessage `json:"hashMetadata"`
	CapturedBy     string          `json:"capturedBy"`
	CapturedAt     time.Time       `json:"capturedAt"`
	SourceTime     *time.Time      `json:"sourceTime"`
	SafeMetadata   json.RawMessage `json:"safeMetadata"`
	CreatedAt      time.Time       `json:"createdAt"`
}
type CorrectiveAction struct {
	ID                       string          `json:"id"`
	OrganizationID           string          `json:"organizationId"`
	IncidentID               string          `json:"incidentId"`
	ActionType               string          `json:"actionType"`
	OwnerUserID              string          `json:"ownerUserId"`
	DueAt                    time.Time       `json:"dueAt"`
	Status                   string          `json:"status"`
	VerificationRequirements json.RawMessage `json:"verificationRequirements"`
	CompletionNotes          string          `json:"completionNotes"`
	CompletedAt              *time.Time      `json:"completedAt"`
	VerifiedBy               string          `json:"verifiedBy"`
	VerifiedAt               *time.Time      `json:"verifiedAt"`
	Version                  int64           `json:"version"`
	CreatedAt                time.Time       `json:"createdAt"`
	UpdatedAt                time.Time       `json:"updatedAt"`
}
