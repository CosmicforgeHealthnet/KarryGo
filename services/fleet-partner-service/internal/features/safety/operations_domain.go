package safety

import (
	"encoding/json"
	"time"
)

type OperationsFlags struct{ Reports, Incidents, Workers, KillSwitch bool }
type Report struct {
	ID, OrganizationID, BranchID, ReporterType, ReporterReference                                                string
	Anonymous                                                                                                    bool
	ReportType, Severity, LocationReference, RouteID, AssignmentID, TripReference, ProviderMembershipID, AssetID string
	Description, ImmediateAction, Status, OwnerUserID, ResolutionAction, ResolutionReason                        string
	Version                                                                                                      int64
	CreatedAt, UpdatedAt                                                                                         time.Time
	ClosedAt                                                                                                     *time.Time
}
type Incident struct {
	ID, OrganizationID, BranchID, IncidentType, Severity, Status                                string
	OccurredAt                                                                                  time.Time
	LocationReference, RouteID, AssignmentID, TripReference                                     string
	PeopleReferences, AssetReferences, ShipmentReferences                                       json.RawMessage
	ExternalReference, Description, ReportedBy, OwnerUserID, ResolutionAction, ResolutionReason string
	Version                                                                                     int64
	CreatedAt, UpdatedAt                                                                        time.Time
	ClosedAt                                                                                    *time.Time
	// Lifecycle facts recorded by the spec 58.11 transitions.
	ImmediateActions, TriagedBy                              string
	TriagedAt                                                *time.Time
	ContainmentSummary, ContainedBy                          string
	ContainedAt                                              *time.Time
	EscalationReason, EscalationTrigger, PreEscalationStatus string
	EscalatedBy                                              string
	EscalatedAt                                              *time.Time
	ClosureReason, ClosedBy                                  string
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
	ID, OrganizationID, IncidentID, InvestigatorUserID, Scope, EvidencePreservation string
	Status, Findings, RootCause                                                     string
	ContributingFactors                                                             json.RawMessage
	FindingsSubmittedAt                                                             *time.Time
	ApprovedBy                                                                      string
	ApprovedAt, ClosedAt                                                            *time.Time
	OpenedBy                                                                        string
	Version                                                                         int64
	CreatedAt, UpdatedAt                                                            time.Time
}
type Evidence struct {
	ID, OrganizationID, IncidentID, MediaFileID string
	HashMetadata                                json.RawMessage
	CapturedBy                                  string
	CapturedAt                                  time.Time
	SourceTime                                  *time.Time
	SafeMetadata                                json.RawMessage
	CreatedAt                                   time.Time
}
type CorrectiveAction struct {
	ID, OrganizationID, IncidentID, ActionType, OwnerUserID string
	DueAt                                                   time.Time
	Status                                                  string
	VerificationRequirements                                json.RawMessage
	CompletionNotes                                         string
	CompletedAt                                             *time.Time
	VerifiedBy                                              string
	VerifiedAt                                              *time.Time
	Version                                                 int64
	CreatedAt, UpdatedAt                                    time.Time
}
