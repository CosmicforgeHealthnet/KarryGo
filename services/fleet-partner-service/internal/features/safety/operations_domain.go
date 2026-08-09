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
